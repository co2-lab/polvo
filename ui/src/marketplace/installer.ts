import type { Theme } from '../themes/types'
import { themeRegistry } from './themeRegistry'

const INSTALLED_THEMES_KEY = 'polvo.installed-themes'

export interface ExtensionManifest {
  name: string
  version: string
  description: string
  category: 'ai-model' | 'tools' | 'theme' | 'panel'
  dock?: boolean
  panelType?: string
  provides?: {
    theme?: {
      label: string
      variables: Record<string, string>
    }
  }[]
}

export function installTheme(theme: Theme): void {
  try {
    const stored = localStorage.getItem(INSTALLED_THEMES_KEY)
    const installed: Theme[] = stored ? JSON.parse(stored) : []
    if (!installed.find((t) => t.id === theme.id)) {
      installed.push(theme)
      localStorage.setItem(INSTALLED_THEMES_KEY, JSON.stringify(installed))
    }
    themeRegistry.add(theme)
  } catch {
    // ignore
  }
}

export function uninstallTheme(id: string): void {
  try {
    const stored = localStorage.getItem(INSTALLED_THEMES_KEY)
    const installed: Theme[] = stored ? JSON.parse(stored) : []
    const filtered = installed.filter((t) => t.id !== id)
    localStorage.setItem(INSTALLED_THEMES_KEY, JSON.stringify(filtered))
    themeRegistry.remove(id)
  } catch {
    // ignore
  }
}

export function loadInstalledThemes(): void {
  try {
    const stored = localStorage.getItem(INSTALLED_THEMES_KEY)
    if (!stored) return
    const installed: Theme[] = JSON.parse(stored)
    for (const theme of installed) {
      themeRegistry.add(theme)
    }
  } catch {
    // ignore
  }
}

export function installExtension(manifest: ExtensionManifest): void {
  if (manifest.category === 'theme' && manifest.provides) {
    for (const provide of manifest.provides) {
      if (provide.theme) {
        const theme: Theme = {
          id: manifest.name,
          label: provide.theme.label,
          variables: provide.theme.variables,
        }
        installTheme(theme)
      }
    }
  }
}
