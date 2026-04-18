import { Panel, Group, Separator } from 'react-resizable-panels'
import { useIDEStore } from '../../store/useIDEStore'
import type { LayoutNode, PanelNode, SplitNode, PanelKind } from '../../types/ide'
import React, { useState, useEffect, useLayoutEffect, useRef, forwardRef, useContext, createContext, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { X, Layers, Square } from 'lucide-react'
import { clsx } from 'clsx'
import type { AgentStatus } from '../../types/api'
import { getFileIconUrl } from '../../lib/fileIcons'
import { getPanelIcon } from '../icons/panelIcon'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  useDroppable,
  useDraggable,
  type DragEndEvent,
  type DragStartEvent,
} from '@dnd-kit/core'
import { CSS } from '@dnd-kit/utilities'
import { useShallow } from 'zustand/react/shallow'

// Panel content components
import { AgentPanel } from '../agents/AgentPanel'
import { LogPanel } from '../log/LogPanel'
import { ChatPanel } from '../chat/ChatPanel'
import { DiffPanel } from '../diff/DiffPanel'
import { TerminalPanel } from '../terminal/TerminalPanel'
import { ExplorerPanel } from '../panels/ExplorerPanel'
import { FileEditorPanel } from '../panels/FileEditorPanel'
import { getIconColor } from '../../lib/fileIconColor'
import { getBaseTitle, computeTitleIndices, resolveTitle } from '../../lib/titleUtils'

interface WorkspaceAreaProps {
  agents: AgentStatus[]
  logLines: string[]
  watching: boolean
  version: string
}

// Context to pass dragging panel id down without prop drilling
const DraggingPanelContext = createContext<string | null>(null)

// ─── Stable Terminal Layer ─────────────────────────────────────────────────────
// Terminals are mounted once and never unmounted when the layout changes.
// A placeholder div in the layout tree tracks where each terminal should appear.
// A ResizeObserver + getBoundingClientRect keeps the portal-rendered terminal
// positioned over its placeholder.

interface TerminalEntry {
  sessionId: string
  executable?: string
}

// Global registry: sessionId → placeholder DOM element
const terminalPlaceholders = new Map<string, HTMLDivElement>()
// Listeners called when any placeholder is registered/unregistered
const placeholderListeners = new Set<() => void>()

function notifyPlaceholderChange() {
  placeholderListeners.forEach(fn => fn())
}

function registerPlaceholder(sessionId: string, el: HTMLDivElement) {
  terminalPlaceholders.set(sessionId, el)
  notifyPlaceholderChange()
}

function unregisterPlaceholder(sessionId: string) {
  terminalPlaceholders.delete(sessionId)
  notifyPlaceholderChange()
}

interface TerminalRect { top: number; left: number; width: number; height: number }

function PositionedTerminal({ sessionId, executable, hidden }: TerminalEntry & { hidden?: boolean }) {
  const [rect, setRect] = useState<TerminalRect | null>(null)
  // Keep the last known valid rect so the terminal is never sized 1×1
  const lastValidRect = useRef<TerminalRect | null>(null)

  const update = useCallback(() => {
    const el = terminalPlaceholders.get(sessionId)
    if (!el) { setRect(null); return }
    const r = el.getBoundingClientRect()
    if (r.width > 0 && r.height > 0) {
      lastValidRect.current = { top: r.top, left: r.left, width: r.width, height: r.height }
    }
    setRect(prev => {
      if (prev && prev.top === r.top && prev.left === r.left && prev.width === r.width && prev.height === r.height) return prev
      return { top: r.top, left: r.left, width: r.width, height: r.height }
    })
  }, [sessionId])

  useEffect(() => {
    update()

    // Watch the placeholder element for size changes
    const attach = () => {
      const el = terminalPlaceholders.get(sessionId)
      if (!el) return null
      const ro = new ResizeObserver(update)
      ro.observe(el)
      return ro
    }
    let ro = attach()

    // When placeholder is re-registered (e.g. after a layout change), re-attach and re-read position
    const onPlaceholderChange = () => {
      ro?.disconnect()
      ro = attach()
      update()
    }
    placeholderListeners.add(onPlaceholderChange)

    // Window resize can change the placeholder's bounding rect
    window.addEventListener('resize', update)

    return () => {
      placeholderListeners.delete(onPlaceholderChange)
      window.removeEventListener('resize', update)
      ro?.disconnect()
    }
  }, [sessionId, update])

  // Always render the portal (keeps TerminalPanel mounted and WS alive).
  // When placeholder has no rect, position off-screen using the last known size
  // so xterm always has valid dimensions (avoids 1×1 PTY resize).
  const noRect = !rect || rect.width === 0 || rect.height === 0
  const invisible = hidden || noRect
  const displayRect = noRect ? lastValidRect.current : rect

  return createPortal(
    <div
      style={!displayRect ? {
        // No rect ever known yet — truly hide off-screen with a reasonable size
        position: 'fixed',
        top: -9999,
        left: -9999,
        width: 800,
        height: 600,
        zIndex: -1,
        pointerEvents: 'none',
        visibility: 'hidden',
      } : {
        position: 'fixed',
        top: invisible ? -9999 : displayRect.top,
        left: invisible ? -9999 : displayRect.left,
        width: displayRect.width,
        height: displayRect.height,
        zIndex: invisible ? -1 : 10,
        pointerEvents: invisible ? 'none' : 'auto',
        visibility: invisible ? 'hidden' : 'visible',
      }}
    >
      <TerminalPanel sessionId={sessionId} executable={executable} />
    </div>,
    document.body
  )
}

// Collects terminals including their executable (needs dockItems)
function collectTerminalEntries(
  node: LayoutNode | null,
  dockItems: ReturnType<typeof useIDEStore.getState>['dockItems'],
  acc: TerminalEntry[] = [],
): TerminalEntry[] {
  if (!node) return acc
  if (node.type === 'panel') {
    if (node.kind === 'terminal' || node.kind === 'ai') {
      // Primary session
      const dockItem = dockItems.find(i => i.id === (node.tabContentIds?.[node.contentId] ?? node.contentId))
      acc.push({ sessionId: node.sessionId ?? node.id, executable: dockItem?.command })
      // Additional sessions from grouped tabs (tabSessions maps tabId → sessionId)
      if (node.tabSessions) {
        for (const [tabId, sid] of Object.entries(node.tabSessions)) {
          if (sid !== (node.sessionId ?? node.id)) {
            const contentId = node.tabContentIds?.[tabId] ?? tabId
            const tabDockItem = dockItems.find(i => i.id === contentId)
            acc.push({ sessionId: sid, executable: tabDockItem?.command })
          }
        }
      }
    }
    return acc
  }
  for (const child of node.children) collectTerminalEntries(child, dockItems, acc)
  return acc
}

export function TerminalLayer() {
  const { workspaces, dockItems, draggedPanelId } = useIDEStore(useShallow(s => ({
    workspaces: s.workspaces,
    dockItems: s.dockItems,
    draggedPanelId: s.draggedPanelId,
  })))
  const entries: TerminalEntry[] = []
  for (const ws of workspaces) {
    collectTerminalEntries(ws.layout, dockItems, entries)
  }
  // Deduplicate and sort by sessionId for stable React key ordering.
  // Without stable order, moving a panel changes DFS traversal order → React
  // unmounts/remounts PositionedTerminal components even though keys match.
  const seen = new Set<string>()
  const unique = entries
    .filter(e => { if (seen.has(e.sessionId)) return false; seen.add(e.sessionId); return true })
    .sort((a, b) => a.sessionId.localeCompare(b.sessionId))

  // Find the sessionId of the panel being dragged so we can hide only that one
  let draggedSessionId: string | null = null
  if (draggedPanelId) {
    for (const ws of workspaces) {
      if (!ws.layout) continue
      const found = ws.layout.type === 'panel' && ws.layout.id === draggedPanelId
        ? ws.layout
        : (function find(n: LayoutNode): PanelNode | null {
            if (n.type === 'panel') return n.id === draggedPanelId ? n : null
            for (const c of n.children) { const r = find(c); if (r) return r }
            return null
          })(ws.layout)
      if (found) { draggedSessionId = found.sessionId ?? found.id; break }
    }
  }

  return <>{unique.map(e => (
    <PositionedTerminal
      key={e.sessionId}
      {...e}
      hidden={e.sessionId === draggedSessionId}
    />
  ))}</>
}

// Placeholder rendered inside the layout where a terminal panel would be.
// It registers itself so PositionedTerminal can find its position.
function TerminalPlaceholder({ sessionId }: { sessionId: string }) {
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!ref.current) return
    registerPlaceholder(sessionId, ref.current)
    return () => unregisterPlaceholder(sessionId)
  }, [sessionId])

  // After every commit (e.g. layout restructuring), notify so PositionedTerminal re-reads position.
  // useLayoutEffect fires synchronously after DOM mutations, before paint.
  useLayoutEffect(() => {
    notifyPlaceholderChange()
  })

  return <div ref={ref} className="w-full h-full" />
}

// ─── Drop Zone ────────────────────────────────────────────────────────────────

type DropPosition = 'top' | 'bottom' | 'left' | 'right' | 'center'

function getDropPosition(
  rect: DOMRect,
  x: number,
  y: number,
  canCenter: boolean,
): DropPosition {
  const relX = x - rect.left
  const relY = y - rect.top
  const w = rect.width
  const h = rect.height
  const edgeSize = 0.3 // 30% from each edge

  if (canCenter) {
    const inCenterX = relX > w * edgeSize && relX < w * (1 - edgeSize)
    const inCenterY = relY > h * edgeSize && relY < h * (1 - edgeSize)
    if (inCenterX && inCenterY) return 'center'
  }

  // Which edge is closest?
  const dLeft   = relX
  const dRight  = w - relX
  const dTop    = relY
  const dBottom = h - relY
  const min = Math.min(dLeft, dRight, dTop, dBottom)
  if (min === dTop)    return 'top'
  if (min === dBottom) return 'bottom'
  if (min === dLeft)   return 'left'
  return 'right'
}

// Global map: panelId → last hovered drop position (read by dragEnd handler)
const panelHoverPos = new Map<string, DropPosition>()

// Single droppable overlay per panel — position computed from cursor location.
// The droppable hit-area stays in the normal DOM so dnd-kit can detect it,
// but the visual highlight is portalled to document.body so it renders above
// everything (terminals, etc.) without being clipped by overflow:hidden parents.
function PanelDropOverlay({
  panelId,
  canCenter,
}: {
  panelId: string
  canCenter: boolean
}) {
  const { isOver, setNodeRef } = useDroppable({ id: panelId, data: { panelId } })
  const [hoverPos, setHoverPos] = useState<DropPosition | null>(null)
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!isOver) {
      setHoverPos(null)
      panelHoverPos.delete(panelId)
      return
    }
    const handler = (e: PointerEvent) => {
      const rect = ref.current?.getBoundingClientRect()
      if (!rect) return
      const pos = getDropPosition(rect, e.clientX, e.clientY, canCenter)
      setHoverPos(pos)
      panelHoverPos.set(panelId, pos)
    }
    window.addEventListener('pointermove', handler)
    return () => window.removeEventListener('pointermove', handler)
  }, [isOver, canCenter, panelId])

  // Compute absolute pixel rect for the portal highlight
  const domRect = ref.current?.getBoundingClientRect()

  const highlight = isOver && hoverPos && domRect ? (() => {
    const { top, left, width, height } = domRect
    const pct = 1/3
    const styles: Record<DropPosition, React.CSSProperties> = {
      top:    { top, left, width, height: height * pct },
      bottom: { top: top + height * (1 - pct), left, width, height: height * pct },
      left:   { top, left, width: width * pct, height },
      right:  { top, left: left + width * (1 - pct), width: width * pct, height },
      center: { top: top + height * 0.25, left: left + width * 0.25, width: width * 0.5, height: height * 0.5 },
    }
    return styles[hoverPos]
  })() : null

  return (
    <>
      {/* Invisible hit-area in normal DOM flow — dnd-kit uses this for collision detection */}
      <div
        ref={(node) => { setNodeRef(node); (ref as React.MutableRefObject<HTMLDivElement | null>).current = node }}
        className="absolute inset-0"
        style={{ zIndex: 0 }}
      />
      {/* Visual highlight portalled to body so it's above everything */}
      {highlight && createPortal(
        <div
          className="pointer-events-none border-2 border-white/50 bg-white/20 transition-all duration-100"
          style={{ position: 'fixed', zIndex: 9999, borderRadius: hoverPos === 'center' ? 8 : 0, ...highlight }}
        />,
        document.body
      )}
    </>
  )
}

// ─── Kind Context Menu ─────────────────────────────────────────────────────────

const KindContextMenu = forwardRef<HTMLDivElement, {
  kind: PanelKind
  x: number
  y: number
  onClose: () => void
}>(function KindContextMenu({ kind, x, y, onClose }, ref) {
  const { kindBehaviors, setKindBehavior } = useIDEStore()
  const current = kindBehaviors[kind]

  return createPortal(
    <div
      ref={ref}
      className="fixed z-50 min-w-[180px] rounded-lg overflow-hidden shadow-xl border border-white/10 bg-[#111] text-xs"
      style={{ top: y, left: x }}
    >
      <div className="px-3 py-2 border-b border-white/10 text-white/30 uppercase tracking-wider font-medium">
        {kind} behavior
      </div>
      {current === 'grouped' ? (
        <>
          <div className="px-3 py-2 text-white/60 flex items-center gap-2">
            <Layers className="w-3.5 h-3.5 shrink-0 text-white/40" />
            New {kind}s open as tabs
          </div>
          <button
            className="w-full flex items-center gap-2.5 px-3 py-2 text-white/40 hover:bg-white/5 hover:text-white/70 transition-colors border-t border-white/5"
            onClick={() => { setKindBehavior(kind, 'new-panel'); onClose() }}
          >
            <Square className="w-3.5 h-3.5 shrink-0" />
            Open each in its own panel
          </button>
        </>
      ) : (
        <>
          <div className="px-3 py-2 text-white/60 flex items-center gap-2">
            <Square className="w-3.5 h-3.5 shrink-0 text-white/40" />
            New {kind}s open in new panels
          </div>
          <button
            className="w-full flex items-center gap-2.5 px-3 py-2 text-white/40 hover:bg-white/5 hover:text-white/70 transition-colors border-t border-white/5"
            onClick={() => { setKindBehavior(kind, 'grouped'); onClose() }}
          >
            <Layers className="w-3.5 h-3.5 shrink-0" />
            Group together as tabs
          </button>
        </>
      )}
    </div>,
    document.body
  )
})

// ─── Title index system ────────────────────────────────────────────────────────
// Context carrying the computed indices for the current render
const TitleIndicesContext = createContext<Map<string, number | null>>(new Map())

// ─── Helpers ───────────────────────────────────────────────────────────────────

function tabLabel(tabId: string, panelId: string, dockItems: { id: string; name: string }[], titleIndices: Map<string, number | null>, node?: PanelNode): string {
  const resolvedId = node?.tabContentIds?.[tabId] ?? tabId
  const baseName = getBaseTitle(resolvedId, dockItems)
  const key = node ? `${panelId}:${tabId}` : panelId
  return resolveTitle(baseName, key, titleIndices)
}

// ─── Drag Handle ───────────────────────────────────────────────────────────────

function usePanelDrag(panelId: string) {
  const { attributes, listeners, setNodeRef } = useDraggable({ id: panelId })
  return { dragRef: setNodeRef, dragListeners: listeners, dragAttributes: attributes }
}

// ─── Panel Content ─────────────────────────────────────────────────────────────

function PanelContent({ node, agents, logLines }: { node: PanelNode; agents: AgentStatus[]; logLines: string[] }) {
  const { removePanel, addPanel, movePanel, switchTab, closeTab, draggedDockItem, projects, activeProjectId, dockItems, openFile, dirtyFiles, flashPanelId, kindBehaviors } = useIDEStore()
  const { dragRef, dragListeners, dragAttributes } = usePanelDrag(node.id)
  const draggingPanelId = useContext(DraggingPanelContext)
  const titleIndices = useContext(TitleIndicesContext)
  const isFlashing = flashPanelId === node.id
  const isDraggingThis = draggingPanelId === node.id
  const showPanelDropZones = draggingPanelId !== null && !isDraggingThis
  const canGroupCenter = showPanelDropZones

  // For dock drops (HTML5 drag from Dock): track hovered position locally
  const [dockHoverPos, setDockHoverPos] = useState<'top'|'bottom'|'left'|'right'|null>(null)
  const panelRef = useRef<HTMLDivElement>(null)

  const getDockDropPosition = (e: React.DragEvent): 'top'|'bottom'|'left'|'right' => {
    const rect = panelRef.current?.getBoundingClientRect()
    if (!rect) return 'right'
    const x = e.clientX - rect.left
    const y = e.clientY - rect.top
    const w = rect.width, h = rect.height
    if (x < w / 3) return 'left'
    if (x > (w * 2) / 3) return 'right'
    if (y < h / 2) return 'top'
    return 'bottom'
  }

  const project = projects.find((p) => p.id === node.projectId)
    ?? projects.find((p) => p.id === activeProjectId)
  const dockItem = dockItems.find((i) => i.id === node.contentId)

  const isFilePanelId = node.contentId.startsWith('file:')
  const isNewFilePanelId = node.contentId.startsWith('newfile:')

  const effectiveProjectId = node.pinnedProjectId ?? activeProjectId
  const effectiveProject = projects.find((p) => p.id === effectiveProjectId)

  const [accentColor, setAccentColor] = useState<string | null>(null)
  const contextMenuRef = useRef<HTMLDivElement>(null)
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null)

  useEffect(() => {
    if (!isFilePanelId) {
      setAccentColor(effectiveProject?.color ?? null)
      return
    }
    const fp = node.contentId.slice(5)
    const url = getFileIconUrl(fp.split('/').pop() ?? fp, false, false)
    getIconColor(url).then(setAccentColor)
  }, [isFilePanelId, node.contentId, effectiveProject?.color])

  useEffect(() => {
    if (!contextMenu) return
    const handler = (e: MouseEvent) => {
      if (contextMenuRef.current && !contextMenuRef.current.contains(e.target as Node)) {
        setContextMenu(null)
      }
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [contextMenu])

  if (project?.hidden) return null

  const hasTabs = node.tabs.length > 1

  const baseDisplayName = isFilePanelId
    ? (node.contentId.slice(5).split('/').pop() ?? node.contentId.slice(5))
    : isNewFilePanelId
    ? `Untitled-${node.contentId.slice(8)}`
    : dockItem?.name || node.contentId
  // For standalone panels (no tabs), apply global duplicate index
  const displayName = hasTabs ? baseDisplayName : resolveTitle(baseDisplayName, node.id, titleIndices)

  // Resolve the sessionId for the active tab: tabSessions maps tabId → sessionId
  // for panels where multiple terminal instances are grouped as tabs.
  const activeTabSessionId = (node.kind === 'terminal' || node.kind === 'ai')
    ? (node.tabSessions?.[node.contentId] ?? node.sessionId ?? node.id)
    : null

  const renderContent = () => {
    if (isFilePanelId) return <FileEditorPanel path={node.contentId.slice(5)} panelId={node.id} />
    if (isNewFilePanelId) return <FileEditorPanel path={node.contentId} panelId={node.id} />
    // Terminals use a placeholder — the actual xterm is rendered in a portal above the layout
    if (activeTabSessionId) return <TerminalPlaceholder sessionId={activeTabSessionId} />
    switch (node.contentId) {
      case 'explorer': return <ExplorerPanel onOpenFile={openFile} panelId={node.id} pinnedProjectId={node.pinnedProjectId} accentColor={accentColor ?? undefined} />
      case 'agents':   return <AgentPanel agents={agents} />
      case 'log':      return <LogPanel lines={logLines} />
      case 'chat':     return <ChatPanel openFiles={[]} activeTab={null} />
      case 'diff':     return <DiffPanel />
      case 'editor':   return <div className="flex-1 flex items-center justify-center text-white/30 text-sm">Open a file from the explorer</div>
      default:
        return <div className="flex-1 flex items-center justify-center text-white/30 text-sm">{node.contentId}</div>
    }
  }

  const borderColor = accentColor
    ? `color-mix(in srgb, ${accentColor} 25%, rgba(255,255,255,0.08))`
    : 'rgba(255,255,255,0.10)'
  const headerBg = accentColor
    ? `color-mix(in srgb, ${accentColor} 8%, rgba(0,0,0,0.15))`
    : 'rgba(0,0,0,0.10)'
  const headerBorder = accentColor
    ? `color-mix(in srgb, ${accentColor} 20%, rgba(255,255,255,0.04))`
    : 'rgba(255,255,255,0.05)'
  const titleColor = accentColor
    ? `color-mix(in srgb, ${accentColor} 80%, white)`
    : 'rgba(255,255,255,0.60)'

  const headerContent = hasTabs ? (
    <div className="flex items-stretch flex-1 overflow-x-auto overflow-y-hidden min-w-0 scrollbar-none">
      {node.tabs.map((tabId) => {
        const isActive = tabId === node.contentId
        const label = tabLabel(tabId, node.id, dockItems, titleIndices, node)
        const iconUrl = tabId.startsWith('file:') ? getFileIconUrl(label, false, false) : null
        return (
          <div
            key={tabId}
            className={clsx(
              'group/tab flex items-center gap-1.5 px-3 h-8 shrink-0 border-r transition-colors select-none',
              isActive
                ? 'text-white/80 bg-white/5 border-r-white/10'
                : 'text-white/35 hover:text-white/60 border-r-white/5 hover:bg-white/[0.03]',
            )}
            onClick={() => switchTab(node.id, tabId)}
          >
            {iconUrl
              ? <img src={iconUrl} alt="" className="w-3.5 h-3.5 shrink-0" draggable={false} />
              : <span className="flex items-center shrink-0 opacity-50">{getPanelIcon(node.kind, dockItems.find(i => i.id === tabId)?.icon)}</span>
            }
            <span className="text-xs font-medium truncate max-w-[120px]">{dirtyFiles.has(tabId) ? `${label} ●` : label}</span>
            <button
              className={clsx(
                'shrink-0 rounded transition-colors p-0.5',
                isActive
                  ? 'text-white/40 hover:text-white/80 hover:bg-white/10'
                  : 'text-transparent group-hover/tab:text-white/30 hover:!text-white/70',
              )}
              onClick={(e) => { e.stopPropagation(); closeTab(node.id, tabId) }}
            >
              <X className="w-3 h-3" />
            </button>
          </div>
        )
      })}
    </div>
  ) : (
    <div className="h-8 flex items-center flex-1 min-w-0 px-3 gap-1.5">
      <span style={{ display: 'flex', alignItems: 'center', opacity: 0.6, flexShrink: 0, color: titleColor }}>
        {getPanelIcon(node.kind, dockItem?.icon)}
      </span>
      <span className="text-xs font-medium truncate transition-colors" style={{ color: titleColor }}>
        {dirtyFiles.has(node.contentId) ? `${displayName} ●` : displayName}
      </span>
    </div>
  )

  return (
    <div
      ref={panelRef}
      data-panel-id={node.id}
      className={clsx(
        'relative w-full h-full bg-black/5 rounded-lg flex flex-col overflow-hidden group backdrop-blur-sm',
        isFlashing && 'animate-panel-flash',
        isDraggingThis && 'opacity-40',
      )}
      style={{ border: `1px solid ${isFlashing ? 'rgba(255,255,255,0.5)' : borderColor}`, transition: 'border-color 0.3s' }}
      onDragOver={(e) => {
        if (!draggedDockItem) return
        e.preventDefault()
        setDockHoverPos(getDockDropPosition(e))
      }}
      onDragLeave={() => setDockHoverPos(null)}
      onDrop={(e) => {
        const itemId = e.dataTransfer.getData('application/x-dock-item')
        if (!itemId) return
        e.preventDefault()
        const pos = getDockDropPosition(e)
        setDockHoverPos(null)
        addPanel(itemId, node.id, pos)
      }}
    >
      {/* Header */}
      <div
        className="flex items-center shrink-0 overflow-hidden relative z-20"
        style={{ backgroundColor: headerBg, borderBottom: `1px solid ${headerBorder}` }}
        onContextMenu={(e) => { e.preventDefault(); setContextMenu({ x: e.clientX, y: e.clientY }) }}
      >
        {/* Drag area — title/tabs, takes all available space */}
        <div
          ref={dragRef}
          {...dragListeners}
          {...dragAttributes}
          className="flex items-stretch flex-1 min-w-0 overflow-hidden cursor-grab active:cursor-grabbing"
          style={{ touchAction: 'none' }}
        >
          {headerContent}
        </div>

        {/* Right side badges + close — outside drag area */}
        {isNewFilePanelId && <span className="shrink-0 text-xs text-white/25 px-1">unsaved</span>}
        {project && <span className="shrink-0 text-xs text-white/25 px-1">[{project.name}]</span>}
        <button
          onClick={() => {
            if (node.kind === 'terminal' || node.kind === 'ai') void fetch(`/terminal/close?id=${encodeURIComponent(node.id)}`, { method: 'POST' })
            removePanel(node.id)
          }}
          className="shrink-0 text-white/30 hover:text-white/90 opacity-0 group-hover:opacity-100 transition-opacity px-2 h-8 flex items-center"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>

      <div className={clsx('flex-1 overflow-hidden min-h-0 relative', (showPanelDropZones || dockHoverPos) && 'pointer-events-none')}>
        {renderContent()}
      </div>

      {/* Drop overlay — single droppable, highlights only the hovered region */}
      {showPanelDropZones && (
        <PanelDropOverlay panelId={node.id} canCenter={canGroupCenter} />
      )}
      {/* Dock drop highlight — shown on hover during HTML5 drag from Dock */}
      {dockHoverPos && (
        <div className={clsx(
          'absolute z-20 bg-white/20 border-2 border-white/50 pointer-events-none transition-all duration-100',
          dockHoverPos === 'top'    && 'top-0 left-0 right-0 h-1/3',
          dockHoverPos === 'bottom' && 'bottom-0 left-0 right-0 h-1/3',
          dockHoverPos === 'left'   && 'top-0 bottom-0 left-0 w-1/3',
          dockHoverPos === 'right'  && 'top-0 bottom-0 right-0 w-1/3',
        )} />
      )}

      {contextMenu && (
        <KindContextMenu
          ref={contextMenuRef}
          kind={node.kind}
          x={contextMenu.x}
          y={contextMenu.y}
          onClose={() => setContextMenu(null)}
        />
      )}
    </div>
  )
}

// ─── Split Group ───────────────────────────────────────────────────────────────

function SplitGroup({ node, agents, logLines }: { node: SplitNode; agents: AgentStatus[]; logLines: string[] }) {
  const { panelSizes, setPanelSizes } = useIDEStore()
  const savedLayout = panelSizes[node.id]

  return (
    <Group
      orientation={node.direction}
      id={node.id}
      defaultLayout={savedLayout}
      onLayoutChanged={(layout) => setPanelSizes(node.id, layout)}
    >
      {node.children.map((child, index) => (
        <React.Fragment key={child.id}>
          {child.type === 'panel' ? (
            <Panel id={child.id} minSize={10}>
              <PanelContent node={child} agents={agents} logLines={logLines} />
            </Panel>
          ) : (
            <Panel id={child.id} minSize={10}>
              <SplitGroup node={child} agents={agents} logLines={logLines} />
            </Panel>
          )}
          {index < node.children.length - 1 && (
            <Separator
              className={[
                'shrink-0 bg-white/5 hover:bg-white/20 active:bg-white/30 transition-colors',
                node.direction === 'horizontal'
                  ? 'w-px cursor-col-resize mx-0.5'
                  : 'h-px cursor-row-resize my-0.5',
              ].join(' ')}
            />
          )}
        </React.Fragment>
      ))}
    </Group>
  )
}

function RecursiveLayout({ node, agents, logLines }: { node: LayoutNode; agents: AgentStatus[]; logLines: string[] }) {
  if (node.type === 'panel') {
    return (
      <Panel id={node.id} minSize={10}>
        <PanelContent node={node} agents={agents} logLines={logLines} />
      </Panel>
    )
  }
  return <SplitGroup node={node} agents={agents} logLines={logLines} />
}

// ─── Ghost overlay shown while dragging ───────────────────────────────────────

function DragGhost({ panelId }: { panelId: string }) {
  const { workspaces, dockItems } = useIDEStore()
  const titleIndices = useContext(TitleIndicesContext)
  let panel: PanelNode | null = null
  for (const ws of workspaces) {
    if (!ws.layout) continue
    const search = (n: LayoutNode): PanelNode | null => {
      if (n.type === 'panel') return n.id === panelId ? n : null
      for (const c of n.children) { const r = search(c); if (r) return r }
      return null
    }
    panel = search(ws.layout)
    if (panel) break
  }
  if (!panel) return null
  const baseName = getBaseTitle(panel.contentId, dockItems)
  const label = resolveTitle(baseName, panel.id, titleIndices)
  return (
    <div className="flex items-center gap-2 px-3 py-1.5 rounded-lg bg-[#1a1a2e] border border-white/20 shadow-2xl text-white/80 text-xs font-medium pointer-events-none select-none backdrop-blur-sm">
      <span className="opacity-60">{getPanelIcon(panel.kind)}</span>
      <span>{label}</span>
    </div>
  )
}

// ─── Workspace Area ────────────────────────────────────────────────────────────

export function WorkspaceArea({ agents, logLines }: WorkspaceAreaProps) {
  const { workspaces, activeWorkspaceId, addPanel, movePanel, movePanelToWorkspace, setDraggedPanelId, dockItems } = useIDEStore()
  const activeWorkspace = workspaces.find((w) => w.id === activeWorkspaceId)
  const [draggingPanelId, setDraggingPanelId] = useState<string | null>(null)

  // Compute global title indices (all workspaces, all panels/tabs)
  const titleIndices = computeTitleIndices(workspaces, dockItems)

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } })
  )

  const handleDragStart = (event: DragStartEvent) => {
    const id = String(event.active.id)
    setDraggingPanelId(id)
    setDraggedPanelId(id)
  }

  const handleDragEnd = (event: DragEndEvent) => {
    setDraggingPanelId(null)
    setDraggedPanelId(null)

    const sourceId = String(event.active.id)
    const over = event.over
    if (!over) return

    const overId = String(over.id)

    if (overId === 'empty-workspace') {
      movePanelToWorkspace(sourceId, activeWorkspaceId)
      return
    }

    const targetPanelId = overId
    if (targetPanelId === sourceId) return

    const position = panelHoverPos.get(targetPanelId) ?? 'right'
    panelHoverPos.delete(targetPanelId)
    movePanel(sourceId, targetPanelId, position)
  }

  const handleDragCancel = () => {
    setDraggingPanelId(null)
    setDraggedPanelId(null)
  }

  if (!activeWorkspace) return null

  const rootLayout = activeWorkspace.layout
  const layoutEl = rootLayout
    ? rootLayout.type === 'panel'
      ? (
        <Group id={`root-group-${activeWorkspace.id}`}>
          <Panel id={rootLayout.id} minSize={10}>
            <PanelContent node={rootLayout} agents={agents} logLines={logLines} />
          </Panel>
        </Group>
      )
      : <RecursiveLayout node={rootLayout} agents={agents} logLines={logLines} />
    : null

  return (
    <TitleIndicesContext.Provider value={titleIndices}>
      <DndContext sensors={sensors} onDragStart={handleDragStart} onDragEnd={handleDragEnd} onDragCancel={handleDragCancel}>
        <DraggingPanelContext.Provider value={draggingPanelId}>
          <div className="flex-1 h-full p-2 overflow-hidden relative">
            {layoutEl ?? (
              <EmptyWorkspaceDrop activeWorkspaceId={activeWorkspaceId} onDrop={addPanel} />
            )}
          </div>
        </DraggingPanelContext.Provider>
        <DragOverlay dropAnimation={null}>
          {draggingPanelId && <DragGhost panelId={draggingPanelId} />}
        </DragOverlay>
      </DndContext>
    </TitleIndicesContext.Provider>
  )
}

function EmptyWorkspaceDrop({ activeWorkspaceId, onDrop }: { activeWorkspaceId: string; onDrop: (id: string) => void }) {
  const { isOver, setNodeRef } = useDroppable({ id: 'empty-workspace' })
  const { draggedDockItem } = useIDEStore()

  // Also handle dock item drops via HTML5 (dock uses HTML5 drag)
  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault()
    const itemId = e.dataTransfer.getData('application/x-dock-item')
    if (itemId) onDrop(itemId)
  }

  return (
    <div
      ref={setNodeRef}
      className={clsx(
        'w-full h-full border-2 border-dashed rounded-xl flex flex-col items-center justify-center text-white/40 bg-black/5 gap-3 select-none transition-colors',
        isOver ? 'border-white/30 bg-white/5' : 'border-white/10',
      )}
      onDragOver={(e) => e.preventDefault()}
      onDrop={handleDrop}
    >
      <span className="text-4xl opacity-20">◎</span>
      <span className="text-sm">Drag an item from the dock or click to add a panel</span>
      {(draggedDockItem || isOver) && (
        <span className="text-xs text-white/20">Drop here to open</span>
      )}
    </div>
  )
}
