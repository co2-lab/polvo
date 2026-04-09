package server

// Binary protocol bytes (must match ui/src/components/terminal/TerminalPanel.tsx)
const (
	msgInput   = 0x00 // client → server: raw stdin bytes
	msgOutput  = 0x01 // server → client: raw PTY output bytes
	msgResize  = 0x02 // client → server: JSON {"cols":N,"rows":N}
	msgNew     = 0x03 // server → client: session is brand new
	msgResumed = 0x04 // server → client: session resumed (scrollback replayed)
)
