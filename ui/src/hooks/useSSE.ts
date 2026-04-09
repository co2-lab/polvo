import { useEffect, useRef, useCallback } from 'react'
import type { SSEEvent, EventKind } from '../types/api'

type EventHandler = (payload: unknown) => void

export interface SSEHandlers {
  onSnapshot?: EventHandler
  onAgentStarted?: EventHandler
  onAgentDone?: EventHandler
  onWatchStarted?: EventHandler
  onWatchStopped?: EventHandler
  onFileChanged?: EventHandler
  onLog?: EventHandler
  onPRCreated?: EventHandler
  onReportSaved?: EventHandler
}

const API_BASE = 'http://127.0.0.1:7373'

export function useSSE(handlers: SSEHandlers) {
  const handlersRef = useRef(handlers)
  handlersRef.current = handlers

  const dispatch = useCallback((event: SSEEvent) => {
    const h = handlersRef.current
    const kindMap: Record<EventKind, EventHandler | undefined> = {
      snapshot: h.onSnapshot,
      agent_started: h.onAgentStarted,
      agent_done: h.onAgentDone,
      watch_started: h.onWatchStarted,
      watch_stopped: h.onWatchStopped,
      file_changed: h.onFileChanged,
      log: h.onLog,
      pr_created: h.onPRCreated,
      report_saved: h.onReportSaved,
      // Chat events are handled locally per-request, not via the global SSE bus.
      // These entries exist to satisfy the exhaustive Record type.
      chat_token: undefined,
      chat_done: undefined,
      chat_error: undefined,
    }
    kindMap[event.kind]?.(event.payload)
  }, [])

  useEffect(() => {
    let es: EventSource
    let reconnectTimer: ReturnType<typeof setTimeout>

    function connect() {
      es = new EventSource(`${API_BASE}/events`)

      es.onmessage = (e) => {
        try {
          const event = JSON.parse(e.data) as SSEEvent
          dispatch(event)
        } catch {
          // ignore malformed events
        }
      }

      es.onerror = () => {
        es.close()
        reconnectTimer = setTimeout(connect, 3000)
      }
    }

    connect()

    return () => {
      clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [dispatch])
}

export { API_BASE }
