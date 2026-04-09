import type { ExtensionManifest } from '../../marketplace/installer'

interface ExtensionCardProps {
  extension: ExtensionManifest & { id: string; installed?: boolean }
  onInstall: (id: string) => void
  onUninstall: (id: string) => void
}

const CATEGORY_LABELS: Record<string, string> = {
  'ai-model': 'Modelo de IA',
  tools: 'Ferramenta',
  theme: 'Tema',
  panel: 'Painel',
}

export function ExtensionCard({ extension, onInstall, onUninstall }: ExtensionCardProps) {
  return (
    <div className="extension-card">
      <div className="extension-card-header">
        <div className="extension-card-meta">
          <span className="extension-card-name">{extension.name}</span>
          <span className="extension-card-version">v{extension.version}</span>
        </div>
        <span className="extension-card-category">
          {CATEGORY_LABELS[extension.category] ?? extension.category}
        </span>
      </div>
      <p className="extension-card-desc">{extension.description}</p>
      <div className="extension-card-footer">
        {extension.installed ? (
          <button
            className="extension-btn extension-btn--installed"
            onClick={() => onUninstall(extension.id)}
          >
            Instalado ✓
          </button>
        ) : (
          <button
            className="extension-btn extension-btn--install"
            onClick={() => onInstall(extension.id)}
          >
            Instalar
          </button>
        )}
      </div>
    </div>
  )
}
