import { useEffect, useRef, useCallback } from 'react'
import * as Sentry from '@sentry/react'
import type { SSEEvent, EventKind, SnapshotPayload } from '../types/api'

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

const API_BASE = 'http://localhost:7373'

export function useSSE(handlers: SSEHandlers) {
  const handlersRef = useRef(handlers)
  handlersRef.current = handlers

  const dispatch = useCallback((event: SSEEvent) => {
    // Respect telemetry opt-out from server config on first snapshot.
    if (event.kind === 'snapshot') {
      const snap = event.payload as SnapshotPayload
      if (snap?.status?.telemetry_disabled) {
        Sentry.close()
      }
    }

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
    let es: EventSource | null = null
    let reconnectTimer: ReturnType<typeof setTimeout>
    let cancelled = false
    let firstConnect = true

    function connect() {
      if (cancelled) return

      // On first load, delay slightly so the sidecar has time to bind the port.
      // This avoids a guaranteed-to-fail EventSource attempt (which produces
      // an uncancellable DevTools error) before the server is ready.
      const delay = firstConnect ? 800 : 3000
      firstConnect = false

      reconnectTimer = setTimeout(() => {
        if (cancelled) return

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
          es?.close()
          es = null
          connect()
        }
      }, delay)
    }

    connect()

    return () => {
      cancelled = true
      clearTimeout(reconnectTimer)
      es?.close()
    }
  }, [dispatch])
}

export { API_BASE }
