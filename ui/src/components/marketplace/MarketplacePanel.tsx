import { useState, useEffect } from 'react'
import { Search } from 'lucide-react'
import { MARKETPLACE_EXTENSIONS } from '../../marketplace/registry'
import { installExtension, uninstallTheme } from '../../marketplace/installer'
import { themeRegistry } from '../../marketplace/themeRegistry'
import { ExtensionCard } from './ExtensionCard'
import type { Theme } from '../../themes/types'

type Category = 'all' | 'ai-model' | 'tools' | 'theme' | 'panel'

const CATEGORY_PILLS: { id: Category; label: string }[] = [
  { id: 'all', label: 'Todos' },
  { id: 'theme', label: 'Temas' },
  { id: 'ai-model', label: 'Modelos de IA' },
  { id: 'tools', label: 'Ferramentas' },
  { id: 'panel', label: 'Paineis' },
]

const NATIVE_IDS = new Set(['mint-dark', 'mint-light', 'monochrome', 'ocean', 'sunset'])

export function MarketplacePanel() {
  const [search, setSearch] = useState('')
  const [category, setCategory] = useState<Category>('all')
  const [installedThemes, setInstalledThemes] = useState<Theme[]>(() => themeRegistry.getInstalled())

  useEffect(() => {
    const unsub = themeRegistry.subscribe((themes) => {
      setInstalledThemes(themes.filter((t) => !NATIVE_IDS.has(t.id)))
    })
    return unsub
  }, [])

  const installedIds = new Set(installedThemes.map((t) => t.id))

  const extensions = MARKETPLACE_EXTENSIONS
    .filter((ext) => category === 'all' || ext.category === category)
    .filter((ext) =>
      !search ||
      ext.name.toLowerCase().includes(search.toLowerCase()) ||
      ext.description.toLowerCase().includes(search.toLowerCase())
    )
    .map((ext) => ({ ...ext, installed: installedIds.has(ext.id) }))

  function handleInstall(id: string) {
    const ext = MARKETPLACE_EXTENSIONS.find((e) => e.id === id)
    if (!ext) return
    installExtension(ext)
  }

  function handleUninstall(id: string) {
    uninstallTheme(id)
  }

  return (
    <div className="marketplace-panel">
      <div className="marketplace-header">
        <div className="marketplace-search-wrapper">
          <Search size={13} className="marketplace-search-icon" />
          <input
            className="marketplace-search"
            placeholder="Buscar extensões..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>
        <div className="marketplace-pills">
          {CATEGORY_PILLS.map((pill) => (
            <button
              key={pill.id}
              className={`marketplace-pill${category === pill.id ? ' marketplace-pill--active' : ''}`}
              onClick={() => setCategory(pill.id)}
            >
              {pill.label}
            </button>
          ))}
        </div>
      </div>
      <div className="marketplace-grid">
        {extensions.length === 0 ? (
          <div className="marketplace-empty">Nenhuma extensão encontrada.</div>
        ) : (
          extensions.map((ext) => (
            <ExtensionCard
              key={ext.id}
              extension={ext}
              onInstall={handleInstall}
              onUninstall={handleUninstall}
            />
          ))
        )}
      </div>
    </div>
  )
}
