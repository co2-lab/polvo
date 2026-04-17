import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import * as Sentry from '@sentry/react'
import './index.css'
import App from './App.tsx'

// Detect macOS inside Tauri and add class to <html> for CSS targeting.
// Tauri 2 exposes __TAURI_INTERNALS__ (Tauri 1 used __TAURI__).
const isTauri = '__TAURI_INTERNALS__' in window || '__TAURI__' in window
if (isTauri && navigator.userAgent.includes('Macintosh')) {
  document.documentElement.classList.add('tauri-macos')
}

// Error reporting — enabled by default when VITE_SENTRY_DSN is set at build time.
// Users can opt out via polvo.yaml: telemetry: { disabled: true }
// The snapshot event from useSSE carries the disabled flag from the server config.
const sentryDSN = import.meta.env.VITE_SENTRY_DSN as string | undefined
if (sentryDSN) {
  Sentry.init({
    dsn: sentryDSN,
    environment: import.meta.env.MODE,
    release: import.meta.env.VITE_APP_VERSION as string | undefined,
    tracesSampleRate: 0, // errors only, no performance tracing
    beforeSend(event) {
      if (event.request) {
        event.request.data = undefined
        event.request.cookies = undefined
        event.request.headers = undefined
      }
      return event
    },
  })
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
