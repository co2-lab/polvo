import type { Theme } from './types'

export function applyTheme(theme: Theme): void {
  const root = document.documentElement
  for (const [key, value] of Object.entries(theme.variables)) {
    root.style.setProperty(key, value)
  }
}
