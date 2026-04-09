import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

// Detect macOS inside Tauri and add class to <html> for CSS targeting.
// Tauri 2 exposes __TAURI_INTERNALS__ (Tauri 1 used __TAURI__).
const isTauri = '__TAURI_INTERNALS__' in window || '__TAURI__' in window
if (isTauri && navigator.userAgent.includes('Macintosh')) {
  document.documentElement.classList.add('tauri-macos')
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
