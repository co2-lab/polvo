import { X, Eye, EyeOff } from 'lucide-react'
import { useIDEStore } from '../../store/useIDEStore'
import * as Icons from 'lucide-react'
import { FloatingModal } from '../common/FloatingModal'
import { useT } from '../../lib/i18n'

export function DockManagerModal() {
  const t = useT()
  const { isDockManagerOpen, setDockManagerOpen, dockItems, toggleDockItemVisibility, themes, activeThemeId } = useIDEStore()
  const accent = themes.find(th => th.id === activeThemeId)?.colors.accent ?? 'rgba(255,255,255,0.2)'

  return (
    <FloatingModal
      open={isDockManagerOpen}
      onClose={() => setDockManagerOpen(false)}
      initialWidth={480}
      initialHeight={400}
      minHeight={200}
    >
      {({ dragHandleProps }) => (
        <>
          <div
            className="h-12 border-b border-white/10 flex items-center justify-between px-4 shrink-0"
            {...dragHandleProps}
          >
            <h3 className="text-sm font-medium text-white">{t('dock.manager.title')}</h3>
            <button
              onMouseDown={e => e.stopPropagation()}
              onClick={() => setDockManagerOpen(false)}
              className="p-1.5 text-white/50 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
            >
              <X className="w-4 h-4" />
            </button>
          </div>

          <div className="overflow-y-auto p-3 grid gap-2" style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(120px, 1fr))' }}>
            {dockItems.map((item) => {
              // eslint-disable-next-line @typescript-eslint/no-explicit-any
              const Icon = (Icons as any)[item.icon] || Icons.Box
              return (
                <div
                  key={item.id}
                  onClick={() => toggleDockItemVisibility(item.id)}
                  className="relative flex flex-col items-center gap-2 p-3 rounded-xl border transition-all cursor-pointer"
                  style={item.hidden
                    ? { background: 'rgba(255,255,255,0.02)', borderColor: 'rgba(255,255,255,0.05)', opacity: 0.4 }
                    : { background: `color-mix(in srgb, ${accent} 8%, rgba(255,255,255,0.04))`, borderColor: `color-mix(in srgb, ${accent} 50%, rgba(255,255,255,0.1))` }
                  }
                >
                  <div className="w-10 h-10 flex items-center justify-center bg-black/20 rounded-lg border border-white/5">
                    <Icon className="w-5 h-5 text-white/80" />
                  </div>
                  <div className="text-center">
                    <div className="text-xs font-medium text-white/90 truncate w-full">{item.name}</div>
                    <div className="text-xs text-white/30 capitalize">{item.type === 'ai' ? 'AI' : item.type}</div>
                  </div>
                  <div className="absolute top-2 right-2 p-1 text-white/30">
                    {item.hidden ? <EyeOff className="w-3.5 h-3.5" /> : <Eye className="w-3.5 h-3.5" />}
                  </div>
                </div>
              )
            })}
          </div>
        </>
      )}
    </FloatingModal>
  )
}
