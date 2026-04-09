import { useEffect, useRef } from 'react'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import { WebLinksAddon } from '@xterm/addon-web-links'
import '@xterm/xterm/css/xterm.css'
import { useIDEStore } from '../../store/useIDEStore'

// Protocol: first byte of every binary WebSocket frame
const MSG_INPUT   = 0x00 // client → server: raw stdin bytes
const MSG_OUTPUT  = 0x01 // server → client: raw PTY output bytes
const MSG_RESIZE  = 0x02 // client → server: JSON {cols, rows}
const MSG_NEW     = 0x03 // server → client: session is brand new
const MSG_RESUMED = 0x04 // server → client: session resumed (scrollback replayed)

interface TerminalPanelProps {
  sessionId?: string
  /** If set, the PTY starts this executable directly instead of a shell. */
  executable?: string
}

export function TerminalPanel({ sessionId = 'default', executable }: TerminalPanelProps) {
  const containerRef = useRef<HTMLDivElement>(null)
  const termRef = useRef<Terminal | null>(null)
  const fitAddonRef = useRef<FitAddon | null>(null)
  const { editorFontFamily, editorFontSize } = useIDEStore(s => s.generalSettings)

  // Apply font changes to xterm without recreating the terminal
  useEffect(() => {
    if (!termRef.current) return
    termRef.current.options.fontFamily = editorFontFamily
    termRef.current.options.fontSize = editorFontSize
    fitAddonRef.current?.fit()
  }, [editorFontFamily, editorFontSize])

  useEffect(() => {
    if (!containerRef.current) return

    const sid = encodeURIComponent(sessionId)
    const wsBase = window.location.origin.replace(/^http/, 'ws')
    const wsUrl = executable
      ? `${wsBase}/terminal/ws?id=${sid}&cmd=${encodeURIComponent(executable)}`
      : `${wsBase}/terminal/ws?id=${sid}`

    let ws: WebSocket
    let destroyed = false
    let retryTimer: ReturnType<typeof setTimeout> | null = null

    const { editorFontFamily: fontFamily, editorFontSize: fontSize } = useIDEStore.getState().generalSettings
    const term = new Terminal({
      cursorBlink: true,
      fontSize,
      fontFamily,
      theme: {
        background: '#000000',
        foreground: '#e0e0e0',
        cursor: '#e0e0e0',
        selectionBackground: 'rgba(255,255,255,0.15)',
        black: '#1a1a1a',
        red: '#f87171',
        green: '#4ade80',
        yellow: '#facc15',
        blue: '#60a5fa',
        magenta: '#c084fc',
        cyan: '#22d3ee',
        white: '#e0e0e0',
        brightBlack: '#4a4a4a',
        brightRed: '#fca5a5',
        brightGreen: '#86efac',
        brightYellow: '#fde047',
        brightBlue: '#93c5fd',
        brightMagenta: '#d8b4fe',
        brightCyan: '#67e8f9',
        brightWhite: '#ffffff',
      },
      allowProposedApi: false,
      scrollback: 5000,
    })

    const fitAddon = new FitAddon()
    term.loadAddon(fitAddon)
    term.loadAddon(new WebLinksAddon())
    termRef.current = term
    fitAddonRef.current = fitAddon

    term.open(containerRef.current)

    const sendResize = (cols: number, rows: number) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      const payload = JSON.stringify({ cols, rows })
      const bytes = new TextEncoder().encode(payload)
      const msg = new Uint8Array(1 + bytes.length)
      msg[0] = MSG_RESIZE
      msg.set(bytes, 1)
      ws.send(msg)
    }

    const connect = (attempt: number) => {
      if (destroyed) return
      ws = new WebSocket(wsUrl)
      ws.binaryType = 'arraybuffer'

      ws.onopen = () => {
        fitAddon.fit()
      }

      ws.onmessage = (e) => {
        const buf = new Uint8Array(e.data as ArrayBuffer)
        if (buf.length === 0) return
        switch (buf[0]) {
          case MSG_OUTPUT:
            term.write(buf.slice(1))
            break
          case MSG_NEW:
            // new session: send dimensions so PTY knows terminal size
            sendResize(term.cols, term.rows)
            break
          case MSG_RESUMED:
            // scrollback already written to xterm — now trigger a resize so
            // TUI apps (claude, etc.) redraw their interface cleanly
            sendResize(term.cols, term.rows)
            break
        }
      }

      ws.onerror = () => {
        // handled by onclose — avoid double retry
      }

      ws.onclose = (e) => {
        if (destroyed) return
        // e.code 1000 = normal close (we called ws.close()), don't retry
        if (e.code === 1000) return
        // retry indefinitely with capped backoff — backend may be restarting
        const delay = Math.min(1000 * Math.pow(2, Math.min(attempt, 4)), 10000)
        retryTimer = setTimeout(() => connect(attempt + 1), delay)
      }
    }

    connect(0)

    term.onData((data) => {
      if (!ws || ws.readyState !== WebSocket.OPEN) return
      const bytes = new TextEncoder().encode(data)
      const msg = new Uint8Array(1 + bytes.length)
      msg[0] = MSG_INPUT
      msg.set(bytes, 1)
      ws.send(msg)
    })

    term.onResize(({ cols, rows }) => sendResize(cols, rows))

    const ro = new ResizeObserver(() => fitAddon.fit())
    ro.observe(containerRef.current)

    return () => {
      destroyed = true
      if (retryTimer) clearTimeout(retryTimer)
      if (ws) ws.close()
      ro.disconnect()
      term.dispose()
      termRef.current = null
      fitAddonRef.current = null
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  return (
    <div
      ref={containerRef}
      className="w-full h-full"
      style={{ padding: '4px 8px', boxSizing: 'border-box' }}
    />
  )
}
