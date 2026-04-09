import type { Theme } from '../themes/types'
import { NATIVE_THEMES } from '../themes'

type RegistryListener = (themes: Theme[]) => void

class ThemeRegistry {
  private themes: Theme[] = [...NATIVE_THEMES]
  private listeners: RegistryListener[] = []

  getAll(): Theme[] {
    return [...this.themes]
  }

  getNative(): Theme[] {
    return NATIVE_THEMES
  }

  getInstalled(): Theme[] {
    return this.themes.filter((t) => !NATIVE_THEMES.find((n) => n.id === t.id))
  }

  add(theme: Theme): void {
    if (this.themes.find((t) => t.id === theme.id)) return
    this.themes = [...this.themes, theme]
    this.notify()
  }

  remove(id: string): void {
    if (NATIVE_THEMES.find((t) => t.id === id)) return // não remove nativos
    this.themes = this.themes.filter((t) => t.id !== id)
    this.notify()
  }

  subscribe(listener: RegistryListener): () => void {
    this.listeners.push(listener)
    return () => {
      this.listeners = this.listeners.filter((l) => l !== listener)
    }
  }

  private notify(): void {
    this.listeners.forEach((l) => l(this.themes))
  }
}

export const themeRegistry = new ThemeRegistry()
