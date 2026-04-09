import { motion, type PanInfo, useDragControls, useMotionValue, animate } from 'framer-motion'
import * as Icons from 'lucide-react'
import { useIDEStore } from '../../store/useIDEStore'
import { AI_ICON_MAP } from '../icons/AIIcons'
import { clsx } from 'clsx'
import type { DockEdge, DockAlignment, DockItem } from '../../types/ide'

function DockIcon({ item, size = 20 }: { item: DockItem; size?: number }) {
  if (item.icon.startsWith('ai:')) {
    const cliId = item.icon.slice(3)
    const AIIcon = AI_ICON_MAP[cliId]
    if (AIIcon) return <AIIcon size={size} />
  }
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  const Icon = (Icons as any)[item.icon] || Icons.Box
  return <Icon className={`text-white/80`} style={{ width: size, height: size }} />
}

export function FloatingDock() {
  const {
    dockItems,
    addPanel,
    setDraggedDockItem,
    dockPosition,
    setDockPosition,
    setSettingsOpen,
    setDockPinned,
    setDockManagerOpen,
  } = useIDEStore()

  const dragControls = useDragControls()
  const x = useMotionValue(0)
  const y = useMotionValue(0)

  const handleDragEnd = (_event: MouseEvent | TouchEvent | PointerEvent, info: PanInfo) => {
    const { point } = info
    const { innerWidth, innerHeight } = window

    const distTop = point.y
    const distBottom = innerHeight - point.y
    const distLeft = point.x
    const distRight = innerWidth - point.x

    const minDist = Math.min(distTop, distBottom, distLeft, distRight)

    let newEdge: DockEdge = dockPosition.edge
    if (minDist === distTop) newEdge = 'top'
    else if (minDist === distBottom) newEdge = 'bottom'
    else if (minDist === distLeft) newEdge = 'left'
    else if (minDist === distRight) newEdge = 'right'

    let newAlignment: DockAlignment = 'center'
    if (newEdge === 'top' || newEdge === 'bottom') {
      if (point.x < innerWidth / 3) newAlignment = 'start'
      else if (point.x > (innerWidth * 2) / 3) newAlignment = 'end'
      else newAlignment = 'center'
    } else {
      if (point.y < innerHeight / 3) newAlignment = 'start'
      else if (point.y > (innerHeight * 2) / 3) newAlignment = 'end'
      else newAlignment = 'center'
    }

    setDockPosition({ edge: newEdge, alignment: newAlignment })
    animate(x, 0, { type: 'spring', stiffness: 300, damping: 30 })
    animate(y, 0, { type: 'spring', stiffness: 300, damping: 30 })
  }

  const isVertical = dockPosition.edge === 'left' || dockPosition.edge === 'right'

  const positionClasses = clsx('fixed z-50', {
    'top-4': dockPosition.edge === 'top' || (isVertical && dockPosition.alignment === 'start'),
    'bottom-4': dockPosition.edge === 'bottom' || (isVertical && dockPosition.alignment === 'end'),
    'left-4': dockPosition.edge === 'left' || (!isVertical && dockPosition.alignment === 'start'),
    'right-4': dockPosition.edge === 'right' || (!isVertical && dockPosition.alignment === 'end'),
    'left-1/2 -translate-x-1/2': !isVertical && dockPosition.alignment === 'center',
    'top-1/2 -translate-y-1/2': isVertical && dockPosition.alignment === 'center',
  })

  const visibleItems = dockItems.filter((i) => !i.hidden)

  return (
    <motion.div
      layout
      className={positionClasses}
      style={{ x, y }}
      drag
      dragControls={dragControls}
      dragListener={false}
      dragMomentum={false}
      onDragEnd={handleDragEnd}
    >
      <div
        className={clsx(
          'flex items-center gap-1 px-2 py-1.5 bg-black/10 border border-white/[0.05] rounded-2xl transition-all duration-300 hover:bg-black/40 hover:backdrop-blur-md hover:border-white/10 hover:shadow-2xl',
          isVertical ? 'flex-col' : 'flex-row'
        )}
      >
        <div
          className={clsx(
            'flex items-center justify-center text-white/30 cursor-grab active:cursor-grabbing hover:text-white/70 transition-colors',
            isVertical ? 'w-full h-6' : 'w-6 h-full'
          )}
          onPointerDown={(e) => dragControls.start(e)}
        >
          <Icons.GripHorizontal className={clsx('w-5 h-5', isVertical ? '' : 'rotate-90')} />
        </div>

        {visibleItems.map((item) => {
          return (
            <div
              key={item.id}
              className="relative group cursor-pointer"
              draggable={item.type !== 'action'}
              onDragStart={(e: React.DragEvent) => {
                if (item.type === 'action') {
                  e.preventDefault()
                  return
                }
                e.dataTransfer.setData('application/x-dock-item', item.id)
                setDraggedDockItem(item.id)
              }}
              onDragEnd={() => setDraggedDockItem(null)}
            >
            <motion.div
              className="relative"
              whileHover={{
                scale: 1.15,
                x: isVertical ? (dockPosition.edge === 'left' ? 8 : -8) : 0,
                y: !isVertical ? (dockPosition.edge === 'top' ? 8 : -8) : 0,
              }}
              whileTap={{ scale: 0.95 }}
              onClick={() => {
                if (item.type === 'action' && item.id === 'settings') {
                  setSettingsOpen(true)
                } else {
                  addPanel(item.id)
                }
              }}
            >
              <div className="w-10 h-10 flex items-center justify-center rounded-xl hover:bg-white/10 transition-all">
                <DockIcon item={item} size={20} />
              </div>
            </motion.div>
              <div
                className={clsx(
                  'absolute px-2.5 py-1.5 bg-black/80 backdrop-blur-md border border-white/10 text-xs font-medium text-white rounded-md opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-50 shadow-xl',
                  {
                    '-top-12 left-1/2 -translate-x-1/2': dockPosition.edge === 'bottom',
                    '-bottom-12 left-1/2 -translate-x-1/2': dockPosition.edge === 'top',
                    'top-1/2 -translate-y-1/2 -right-3 translate-x-full': dockPosition.edge === 'left',
                    'top-1/2 -translate-y-1/2 -left-3 -translate-x-full': dockPosition.edge === 'right',
                  }
                )}
              >
                {item.name}
              </div>
            </div>
          )
        })}

        <div className={clsx('bg-white/10 rounded-full', isVertical ? 'w-8 h-px my-1' : 'h-8 w-px mx-1')} />

        <button
          onClick={() => setDockManagerOpen(true)}
          className="p-2 hover:bg-white/10 rounded-xl text-white/50 hover:text-white/90 transition-colors"
          title="Manage Dock"
        >
          <Icons.Plus className="w-5 h-5" />
        </button>

        <button
          onClick={() => setDockPinned(true)}
          className="p-2 hover:bg-white/10 rounded-xl text-white/50 hover:text-white/90 transition-colors"
          title="Pin Dock"
        >
          <Icons.Pin className="w-5 h-5" />
        </button>
      </div>
    </motion.div>
  )
}

export function PinnedDock() {
  const { dockItems, addPanel, dockPosition, setDockPinned, setDockManagerOpen, setSettingsOpen } = useIDEStore()
  const isVertical = dockPosition.edge === 'left' || dockPosition.edge === 'right'
  const visibleItems = dockItems.filter((i) => !i.hidden)

  return (
    <div
      className={clsx(
        'bg-black/10 flex items-center justify-center gap-2 z-40 shrink-0 transition-all duration-300 hover:bg-black/40 hover:backdrop-blur-md',
        isVertical ? 'flex-col w-16 py-4' : 'flex-row h-16 px-4',
        dockPosition.edge === 'top' ? 'border-b border-white/10' : '',
        dockPosition.edge === 'bottom' ? 'border-t border-white/10' : '',
        dockPosition.edge === 'left' ? 'border-r border-white/10' : '',
        dockPosition.edge === 'right' ? 'border-l border-white/10' : ''
      )}
    >
      {visibleItems.map((item) => {
        return (
          <div key={item.id} className="relative group">
            <button
              onClick={() => {
                if (item.type === 'action' && item.id === 'settings') {
                  setSettingsOpen(true)
                } else {
                  addPanel(item.id)
                }
              }}
              className="p-2.5 hover:bg-white/10 rounded-xl text-white/60 hover:text-white transition-colors"
            >
              <DockIcon item={item} size={20} />
            </button>
            <div
              className={clsx(
                'absolute px-2.5 py-1.5 bg-black/80 backdrop-blur-md border border-white/10 text-xs font-medium text-white rounded-md opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-50 shadow-xl',
                {
                  '-top-10 left-1/2 -translate-x-1/2': dockPosition.edge === 'bottom',
                  '-bottom-10 left-1/2 -translate-x-1/2': dockPosition.edge === 'top',
                  'top-1/2 -translate-y-1/2 -right-3 translate-x-full': dockPosition.edge === 'left',
                  'top-1/2 -translate-y-1/2 -left-3 -translate-x-full': dockPosition.edge === 'right',
                }
              )}
            >
              {item.name}
            </div>
          </div>
        )
      })}

      <div className={clsx('bg-white/10', isVertical ? 'w-8 h-px my-1' : 'h-8 w-px mx-1')} />

      <div className="relative group">
        <button
          onClick={() => setDockManagerOpen(true)}
          className="p-2.5 hover:bg-white/10 rounded-xl text-white/40 hover:text-white/90 transition-colors"
        >
          <Icons.Plus className="w-5 h-5" />
        </button>
        <div className={clsx('absolute px-2.5 py-1.5 bg-black/80 backdrop-blur-md border border-white/10 text-xs font-medium text-white rounded-md opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-50 shadow-xl', { '-top-10 left-1/2 -translate-x-1/2': dockPosition.edge === 'bottom', '-bottom-10 left-1/2 -translate-x-1/2': dockPosition.edge === 'top', 'top-1/2 -translate-y-1/2 -right-3 translate-x-full': dockPosition.edge === 'left', 'top-1/2 -translate-y-1/2 -left-3 -translate-x-full': dockPosition.edge === 'right' })}>
          Manage Dock
        </div>
      </div>

      <div className="relative group">
        <button
          onClick={() => setDockPinned(false)}
          className="p-2.5 hover:bg-white/10 rounded-xl text-white/40 hover:text-white/90 transition-colors"
        >
          <Icons.PinOff className="w-5 h-5" />
        </button>
        <div className={clsx('absolute px-2.5 py-1.5 bg-black/80 backdrop-blur-md border border-white/10 text-xs font-medium text-white rounded-md opacity-0 group-hover:opacity-100 transition-opacity whitespace-nowrap pointer-events-none z-50 shadow-xl', { '-top-10 left-1/2 -translate-x-1/2': dockPosition.edge === 'bottom', '-bottom-10 left-1/2 -translate-x-1/2': dockPosition.edge === 'top', 'top-1/2 -translate-y-1/2 -right-3 translate-x-full': dockPosition.edge === 'left', 'top-1/2 -translate-y-1/2 -left-3 -translate-x-full': dockPosition.edge === 'right' })}>
          Unpin Dock
        </div>
      </div>
    </div>
  )
}
