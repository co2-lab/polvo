package mcp

import (
	"context"
	"log/slog"
	"time"

	"github.com/fsnotify/fsnotify"
)

const hotReloadDebounce = 500 * time.Millisecond

// WatchConfig monitors the MCP config file at path and hot-reloads servers
// when changes are detected. Changes are debounced by 500 ms.
//
// The function blocks until ctx is cancelled.
func (h *MCPHub) WatchConfig(ctx context.Context, path string) error {
	fsw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fsw.Close()

	// Watch the file directly (it may not exist yet; we re-try on create).
	_ = fsw.Add(path)

	var timer *time.Timer
	scheduleReload := func() {
		if timer != nil {
			timer.Stop()
		}
		timer = time.AfterFunc(hotReloadDebounce, func() {
			h.reload(ctx, path)
		})
	}

	for {
		select {
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			return ctx.Err()

		case event, ok := <-fsw.Events:
			if !ok {
				return nil
			}
			if event.Op.Has(fsnotify.Write) || event.Op.Has(fsnotify.Create) {
				slog.Debug("mcp: config file changed, scheduling reload", "path", event.Name)
				scheduleReload()
				// If a new file was created, make sure we watch it.
				if event.Op.Has(fsnotify.Create) {
					_ = fsw.Add(path)
				}
			}

		case err, ok := <-fsw.Errors:
			if !ok {
				return nil
			}
			slog.Warn("mcp: watcher error", "error", err)
		}
	}
}

// reload parses the updated config and reconciles the running connections:
// stops removed/modified servers and starts added/modified ones.
func (h *MCPHub) reload(ctx context.Context, path string) {
	newCfg, err := LoadMCPConfig(path)
	if err != nil {
		slog.Error("mcp: hot-reload: failed to parse config", "path", path, "error", err)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	oldCfg := h.cfg

	// Determine servers to stop (removed or command/args changed).
	for name, rc := range h.connections {
		newSrv, stillExists := newCfg.MCPServers[name]
		if !stillExists || newSrv.Disabled || serverChanged(oldCfg.MCPServers[name], newSrv) {
			slog.Info("mcp: hot-reload: stopping server", "server", name)
			if err := rc.Disconnect(); err != nil {
				slog.Warn("mcp: hot-reload: disconnect error", "server", name, "error", err)
			}
			delete(h.connections, name)
		}
	}

	// Start added or modified servers.
	for name, srv := range newCfg.MCPServers {
		if srv.Disabled {
			continue
		}
		if _, running := h.connections[name]; running {
			continue // already running and unchanged
		}
		slog.Info("mcp: hot-reload: starting server", "server", name)

		srv := srv // capture for closure
		factory := func() (*MCPConnection, error) {
			conn := NewMCPConnection(srv)
			if err := conn.Connect(ctx); err != nil {
				return nil, err
			}
			return conn, nil
		}

		rc, err := NewResilientClient(factory)
		if err != nil {
			slog.Warn("mcp: hot-reload: failed to connect", "server", name, "error", err)
			continue
		}
		h.connections[name] = rc
	}

	// Update stored config and permission engine.
	h.cfg = newCfg
	h.permissions = NewPermissionEngine(newCfg.Permissions)
	slog.Info("mcp: hot-reload complete")
}

// serverChanged returns true if the server config has changed in a way that
// requires reconnection (command, args, env, transport, url, or headers changed).
func serverChanged(old, new MCPServerConfig) bool {
	if old.Command != new.Command {
		return true
	}
	if len(old.Args) != len(new.Args) {
		return true
	}
	for i := range old.Args {
		if old.Args[i] != new.Args[i] {
			return true
		}
	}
	if old.Transport != new.Transport || old.URL != new.URL {
		return true
	}
	if len(old.Env) != len(new.Env) {
		return true
	}
	for k, v := range old.Env {
		if new.Env[k] != v {
			return true
		}
	}
	return false
}
