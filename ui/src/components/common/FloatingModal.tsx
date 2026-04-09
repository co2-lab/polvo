import { useRef, useState, useEffect, useCallback, type ReactNode } from 'react'
import { motion, AnimatePresence } from 'framer-motion'

interface FloatingModalProps {
  open: boolean
  onClose: () => void
  children: (titleBarProps: TitleBarProps) => ReactNode
  initialWidth?: number
  initialHeight?: number
  minWidth?: number
  minHeight?: number
}

export interface TitleBarProps {
  /** Spread onto the titlebar element to enable drag */
  dragHandleProps: React.HTMLAttributes<HTMLElement>
}

type ResizeDir = 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'

function getCursor(dir: ResizeDir): string {
  switch (dir) {
    case 'n': case 's': return 'ns-resize'
    case 'e': case 'w': return 'ew-resize'
    case 'ne': case 'sw': return 'nesw-resize'
    case 'nw': case 'se': return 'nwse-resize'
  }
}

// Edge handles: each is an absolutely-positioned div covering a border edge.
// HIT is the clickable thickness (large for easy grab).
// The visual border is 1px via the parent's border — these handles are transparent.
const HIT = 8   // px — hit area on each edge
const CORNER = 16 // px — corner square size

const HANDLES: { dir: ResizeDir; style: React.CSSProperties }[] = [
  // edges
  { dir: 'n',  style: { top: 0,    left: CORNER,   right: CORNER,  height: HIT, cursor: 'ns-resize' } },
  { dir: 's',  style: { bottom: 0, left: CORNER,   right: CORNER,  height: HIT, cursor: 'ns-resize' } },
  { dir: 'w',  style: { left: 0,   top: CORNER,    bottom: CORNER, width:  HIT, cursor: 'ew-resize' } },
  { dir: 'e',  style: { right: 0,  top: CORNER,    bottom: CORNER, width:  HIT, cursor: 'ew-resize' } },
  // corners
  { dir: 'nw', style: { top: 0,    left: 0,   width: CORNER, height: CORNER, cursor: 'nwse-resize' } },
  { dir: 'ne', style: { top: 0,    right: 0,  width: CORNER, height: CORNER, cursor: 'nesw-resize' } },
  { dir: 'sw', style: { bottom: 0, left: 0,   width: CORNER, height: CORNER, cursor: 'nesw-resize' } },
  { dir: 'se', style: { bottom: 0, right: 0,  width: CORNER, height: CORNER, cursor: 'nwse-resize' } },
]

export function FloatingModal({
  open,
  onClose,
  children,
  initialWidth = 520,
  initialHeight = 560,
  minWidth = 300,
  minHeight = 200,
}: FloatingModalProps) {
  const [pos,  setPos]  = useState({ x: 0, y: 0 })
  const [size, setSize] = useState({ w: initialWidth, h: initialHeight })
  const [initialized, setInitialized] = useState(false)

  // Centre on first open
  useEffect(() => {
    if (open && !initialized) {
      setPos({
        x: Math.max(0, (window.innerWidth  - initialWidth)  / 2),
        y: Math.max(0, (window.innerHeight - initialHeight) / 2),
      })
      setSize({ w: initialWidth, h: initialHeight })
      setInitialized(true)
    }
    if (!open) setInitialized(false)
  }, [open, initialized, initialWidth, initialHeight])

  // --- drag ---
  const posRef = useRef(pos)
  useEffect(() => { posRef.current = pos }, [pos])

  const onDragMouseDown = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    const start = { mx: e.clientX, my: e.clientY, px: posRef.current.x, py: posRef.current.y }
    const onMove = (ev: MouseEvent) => {
      setPos({
        x: Math.max(0, start.px + ev.clientX - start.mx),
        y: Math.max(0, start.py + ev.clientY - start.my),
      })
    }
    const onUp = () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp) }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [])

  // --- resize ---
  const sizeRef = useRef(size)
  useEffect(() => { sizeRef.current = size }, [size])

  const startResize = useCallback((dir: ResizeDir) => (e: React.MouseEvent) => {
    e.preventDefault()
    e.stopPropagation()
    const start = { mx: e.clientX, my: e.clientY, px: posRef.current.x, py: posRef.current.y, w: sizeRef.current.w, h: sizeRef.current.h }

    const onMove = (ev: MouseEvent) => {
      const dx = ev.clientX - start.mx
      const dy = ev.clientY - start.my
      let { px, py, w, h } = start

      if (dir.includes('e')) w  = Math.max(minWidth,  w + dx)
      if (dir.includes('s')) h  = Math.max(minHeight, h + dy)
      if (dir.includes('w')) { const nw = Math.max(minWidth,  w - dx); px += w - nw; w = nw }
      if (dir.includes('n')) { const nh = Math.max(minHeight, h - dy); py += h - nh; h = nh }

      setPos({ x: px, y: py })
      setSize({ w, h })
    }
    const onUp = () => { window.removeEventListener('mousemove', onMove); window.removeEventListener('mouseup', onUp) }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [minWidth, minHeight])

  return (
    <AnimatePresence>
      {open && (
        <>
          {/* backdrop — click to close */}
          <div className="fixed inset-0 z-[100]" onClick={onClose} />

          <motion.div
            initial={{ opacity: 0, scale: 0.96 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.96 }}
            transition={{ duration: 0.12 }}
            className="fixed z-[101] bg-[#111111]/95 backdrop-blur-2xl border border-white/10 rounded-2xl shadow-2xl flex flex-col overflow-hidden"
            style={{ left: pos.x, top: pos.y, width: size.w, height: size.h }}
          >
            {/* Resize handles — rendered on top of content, pointer-events only on the handles */}
            {HANDLES.map(({ dir, style }) => (
              <div
                key={dir}
                onMouseDown={startResize(dir)}
                style={{ position: 'absolute', zIndex: 10, ...style }}
              />
            ))}

            {children({ dragHandleProps: { onMouseDown: onDragMouseDown, style: { cursor: 'grab' } } })}
          </motion.div>
        </>
      )}
    </AnimatePresence>
  )
}
