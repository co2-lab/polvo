import { useState, useCallback, useEffect, useRef } from 'react'
import { useSSE } from './hooks/useSSE'
import { useIDEStore } from './store/useIDEStore'
import { WorkspaceTabs } from './components/layout/WorkspaceTabs'
import { SidePanel } from './components/layout/SidePanel'
import { WorkspaceArea } from './components/workspace/WorkspaceArea'
import { FloatingDock, PinnedDock } from './components/layout/Dock'
import { SettingsModal } from './components/settings/SettingsModal'
import { DockManagerModal } from './components/layout/DockManagerModal'
import { ProjectConfigModal } from './components/settings/ProjectConfigModal'
import { WelcomeScreen } from './components/welcome/WelcomeScreen'
import type { AgentStatus, SnapshotPayload, LogPayload } from './types/api'

function CloseConfirmDialog({ onConfirm, onCancel }: { onConfirm: () => void; onCancel: () => void }) {
  const lang = useIDEStore.getState().generalSettings.language
  const isPtBR = lang === 'pt-BR'
  return (
    <div className="fixed inset-0 z-[9999] flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="bg-[#111] border border-white/10 rounded-xl shadow-2xl p-6 flex flex-col gap-4 min-w-[280px]">
        <p className="text-sm text-white/80">{isPtBR ? 'Fechar o aplicativo?' : 'Close the application?'}</p>
        <div className="flex gap-2 justify-end">
          <button
            onClick={onCancel}
            className="px-4 py-1.5 rounded-lg text-sm text-white/50 hover:text-white/80 hover:bg-white/5 transition-colors"
          >
            {isPtBR ? 'Cancelar' : 'Cancel'}
          </button>
          <button
            onClick={onConfirm}
            className="px-4 py-1.5 rounded-lg text-sm bg-red-500/20 text-red-400 hover:bg-red-500/30 transition-colors"
          >
            {isPtBR ? 'Fechar' : 'Close'}
          </button>
        </div>
      </div>
    </div>
  )
}

const LOG_RING = 200

export default function App() {
  const {
    detectCLIs,
    loadProjects,
    activeThemeId,
    themes,
    isSidePanelOpen,
    sidePanelPosition,
    isDockPinned,
    isDockOpen,
    dockPosition,
    projects,
    projectConfigId,
    closeProjectConfig,
    generalSettings,
  } = useIDEStore()

  const [showCloseConfirm, setShowCloseConfirm] = useState(false)
  const confirmOnCloseRef = useRef(generalSettings.confirmOnClose)
  useEffect(() => { confirmOnCloseRef.current = generalSettings.confirmOnClose }, [generalSettings.confirmOnClose])

  useEffect(() => {
    const isTauri = '__TAURI_INTERNALS__' in window || '__TAURI__' in window
    if (!isTauri) return

    let unlisten: (() => void) | undefined

    import('@tauri-apps/api/window').then(({ getCurrentWindow }) => {
      const win = getCurrentWindow()
      win.listen('tauri://close-requested', () => {
        if (!confirmOnCloseRef.current) {
          win.destroy()
          return
        }
        setShowCloseConfirm(true)
      }).then(fn => { unlisten = fn })
    })

    return () => { unlisten?.() }
  }, [])

  const [agents, setAgents] = useState<AgentStatus[]>([])
  const [logLines, setLogLines] = useState<string[]>([])
  const [watching, setWatching] = useState(false)
  const [version, setVersion] = useState('0.0.0')
  const [ready, setReady] = useState(false)
  const [cwd, setCwd] = useState('')
  const [unconfigured, setUnconfigured] = useState(false)

  const appendLog = useCallback((text: string) => {
    setLogLines((prev) => {
      const next = [...prev, text]
      return next.length > LOG_RING ? next.slice(-LOG_RING) : next
    })
  }, [])

  useSSE({
    onSnapshot: useCallback((payload: unknown) => {
      const snap = payload as SnapshotPayload
      if (snap.status) {
        setWatching(snap.status.watching)
        setVersion(snap.status.version || '0.0.0')
        setCwd(snap.status.cwd ?? '')
        setUnconfigured(!snap.status.project)
      }
      if (snap.agents) setAgents(snap.agents)
      if (snap.recent_log) setLogLines(snap.recent_log)
      setReady(true)
    }, []),

    onAgentStarted: useCallback((payload: unknown) => {
      const a = payload as AgentStatus
      setAgents((prev) => {
        const existing = prev.findIndex((x) => x.file === a.file && x.name === a.name)
        if (existing !== -1) {
          const next = [...prev]
          next[existing] = a
          return next
        }
        return [a, ...prev]
      })
    }, []),

    onAgentDone: useCallback((payload: unknown) => {
      const a = payload as AgentStatus
      setAgents((prev) => prev.map((x) => (x.file === a.file && x.name === a.name ? a : x)))
    }, []),

    onWatchStarted: useCallback(() => setWatching(true), []),
    onWatchStopped: useCallback(() => setWatching(false), []),

    onLog: useCallback((payload: unknown) => {
      const p = payload as LogPayload
      if (p?.text) appendLog(p.text)
    }, [appendLog]),
  })

  // After the SSE connection delivers its first snapshot (ready=true), the server
  // is confirmed up — safe to load projects and CLIs. The initial calls on mount
  // may fail with ECONNREFUSED if the sidecar hasn't started yet.
  useEffect(() => {
    if (!ready) return
    detectCLIs()
    loadProjects()
  }, [ready, detectCLIs, loadProjects])

  const activeTheme = themes.find((t) => t.id === activeThemeId) || themes[0]

  if (!ready) {
    return (
      <div
        className="w-full h-screen flex items-center justify-center font-sans"
        style={{ backgroundColor: activeTheme.colors.bg, color: activeTheme.colors.text }}
      >
        <div className="flex flex-col items-center gap-4">
          <div className="w-8 h-8 border-2 border-white/20 border-t-white/80 rounded-full animate-spin" />
          <span className="text-sm text-white/40">Connecting…</span>
        </div>
      </div>
    )
  }

  if (unconfigured) {
    return (
      <>
        <WelcomeScreen version={version} cwd={cwd} colors={activeTheme.colors} />
        <SettingsModal />
      </>
    )
  }

  return (
    <div
      className="w-full h-screen flex flex-col overflow-hidden font-sans transition-colors duration-300"
      style={{ backgroundColor: activeTheme.colors.bg, color: activeTheme.colors.text }}
    >
      {isDockPinned && dockPosition.edge === 'top' && isDockOpen && <PinnedDock />}

      <WorkspaceTabs />

      <div className="flex-1 flex overflow-hidden">
        {isDockPinned && dockPosition.edge === 'left' && isDockOpen && <PinnedDock />}
        {isSidePanelOpen && sidePanelPosition === 'left' && <SidePanel />}

        <WorkspaceArea agents={agents} logLines={logLines} watching={watching} version={version} />

        {isSidePanelOpen && sidePanelPosition === 'right' && <SidePanel />}
        {isDockPinned && dockPosition.edge === 'right' && isDockOpen && <PinnedDock />}
      </div>

      {isDockPinned && dockPosition.edge === 'bottom' && isDockOpen && <PinnedDock />}

      {!isDockPinned && <FloatingDock />}

      <SettingsModal />
      <DockManagerModal />
      {projectConfigId && (
        <ProjectConfigModal
          projectName={projects.find(p => p.id === projectConfigId)?.name ?? projectConfigId}
          onClose={closeProjectConfig}
        />
      )}
      {showCloseConfirm && (
        <CloseConfirmDialog
          onConfirm={() => import('@tauri-apps/api/window').then(({ getCurrentWindow }) => getCurrentWindow().destroy())}
          onCancel={() => setShowCloseConfirm(false)}
        />
      )}
    </div>
  )
}
