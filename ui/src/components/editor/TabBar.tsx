import { X } from 'lucide-react'
import type { OpenFile } from '../../hooks/useFiles'

interface TabBarProps {
  tabs: OpenFile[]
  activeTab: string | null
  onSelect: (path: string) => void
  onClose: (path: string) => void
}

export function TabBar({ tabs, activeTab, onSelect, onClose }: TabBarProps) {
  if (tabs.length === 0) return null

  return (
    <div className="tab-bar">
      {tabs.map((f) => {
        const label = f.path.split('/').pop() ?? f.path
        const isActive = f.path === activeTab
        return (
          <div
            key={f.path}
            className={`tab-item${isActive ? ' active' : ''}`}
            onClick={() => onSelect(f.path)}
            title={f.path}
          >
            <span className="tab-label">
              {f.dirty && <span className="tab-dirty" title="Unsaved changes">●</span>}
              {label}
            </span>
            <button
              className="tab-close"
              onClick={(e) => {
                e.stopPropagation()
                onClose(f.path)
              }}
              title="Close tab"
            >
              <X size={12} />
            </button>
          </div>
        )
      })}
    </div>
  )
}
