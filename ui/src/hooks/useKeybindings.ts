import { useEffect, useCallback } from 'react'
import { useIDEStore } from '../store/useIDEStore'

export interface Keybinding {
  id: string
  action: string
  context: 'global' | 'editor' | 'terminal' | 'panels'
  defaultKey: string
  key: string
}

const DEFAULT_KEYBINDINGS: Keybinding[] = [
  { id: 'nav-panels', action: 'Navegar entre paineis', context: 'global', defaultKey: 'Ctrl+Tab', key: 'Ctrl+Tab' },
  { id: 'focus-dock', action: 'Focar Dock', context: 'global', defaultKey: 'Ctrl+D', key: 'Ctrl+D' },
  { id: 'open-marketplace', action: 'Abrir Marketplace', context: 'global', defaultKey: 'Ctrl+Shift+M', key: 'Ctrl+Shift+M' },
  { id: 'open-settings', action: 'Abrir Configurações', context: 'global', defaultKey: 'Ctrl+,', key: 'Ctrl+,' },
  { id: 'new-workspace', action: 'Criar novo workspace', context: 'global', defaultKey: 'Ctrl+Shift+N', key: 'Ctrl+Shift+N' },
  { id: 'close-panel', action: 'Fechar painel ativo', context: 'global', defaultKey: 'Ctrl+W', key: 'Ctrl+W' },
  { id: 'save-file', action: 'Salvar arquivo', context: 'editor', defaultKey: 'Ctrl+S', key: 'Ctrl+S' },
  { id: 'format-file', action: 'Formatar arquivo', context: 'editor', defaultKey: 'Shift+Alt+F', key: 'Shift+Alt+F' },
  { id: 'clear-terminal', action: 'Limpar terminal', context: 'terminal', defaultKey: 'Ctrl+L', key: 'Ctrl+L' },
]

const STORAGE_KEY = 'polvo.keybindings'

export function loadKeybindings(): Keybinding[] {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    if (!stored) return DEFAULT_KEYBINDINGS
    const overrides: Record<string, string> = JSON.parse(stored)
    return DEFAULT_KEYBINDINGS.map((kb) => ({
      ...kb,
      key: overrides[kb.id] ?? kb.key,
    }))
  } catch {
    return DEFAULT_KEYBINDINGS
  }
}

export function saveKeybinding(id: string, key: string): void {
  try {
    const stored = localStorage.getItem(STORAGE_KEY)
    const overrides: Record<string, string> = stored ? JSON.parse(stored) : {}
    overrides[id] = key
    localStorage.setItem(STORAGE_KEY, JSON.stringify(overrides))
  } catch {
    // ignore
  }
}

export function useKeybindings() {
  const addWorkspace = useIDEStore((s) => s.addWorkspace)

  const handleKeyDown = useCallback((e: KeyboardEvent) => {
    const keybindings = loadKeybindings()
    const key = formatKey(e)

    for (const kb of keybindings) {
      if (kb.key === key) {
        switch (kb.id) {
          case 'open-settings':
            // handled by SettingsModal directly
            break
          case 'new-workspace':
            if (e.ctrlKey && e.shiftKey && e.key === 'N') {
              e.preventDefault()
              addWorkspace()
            }
            break
          default:
            break
        }
      }
    }
  }, [addWorkspace])

  useEffect(() => {
    document.addEventListener('keydown', handleKeyDown)
    return () => document.removeEventListener('keydown', handleKeyDown)
  }, [handleKeyDown])
}

export function formatKey(e: KeyboardEvent): string {
  const parts: string[] = []
  if (e.ctrlKey || e.metaKey) parts.push('Ctrl')
  if (e.shiftKey) parts.push('Shift')
  if (e.altKey) parts.push('Alt')
  const key = e.key === ',' ? ',' : e.key
  if (!['Control', 'Shift', 'Alt', 'Meta'].includes(key)) parts.push(key)
  return parts.join('+')
}
