import { Panel, Group, Separator } from 'react-resizable-panels'
import { useIDEStore } from '../../store/useIDEStore'
import type { LayoutNode, PanelNode, SplitNode, PanelKind } from '../../types/ide'
import React, { useState, useEffect, useRef, forwardRef } from 'react'
import { createPortal } from 'react-dom'
import { X, Layers, Square } from 'lucide-react'
import { clsx } from 'clsx'
import type { AgentStatus } from '../../types/api'
import { getFileIconUrl } from '../../lib/fileIcons'
import { getPanelIcon } from '../icons/panelIcon'

// Panel content components
import { AgentPanel } from '../agents/AgentPanel'
import { LogPanel } from '../log/LogPanel'
import { ChatPanel } from '../chat/ChatPanel'
import { DiffPanel } from '../diff/DiffPanel'
import { TerminalPanel } from '../terminal/TerminalPanel'
import { ExplorerPanel } from '../panels/ExplorerPanel'
import { FileEditorPanel } from '../panels/FileEditorPanel'
import { getIconColor } from '../../lib/fileIconColor'

interface WorkspaceAreaProps {
  agents: AgentStatus[]
  logLines: string[]
  watching: boolean
  version: string
}

function DropZone({ position, onDrop }: { position: 'top' | 'bottom' | 'left' | 'right'; onDrop: () => void }) {
  const [isOver, setIsOver] = useState(false)

  const positionClasses = {
    top: 'top-0 left-0 right-0 h-1/4',
    bottom: 'bottom-0 left-0 right-0 h-1/4',
    left: 'top-0 bottom-0 left-0 w-1/4',
    right: 'top-0 bottom-0 right-0 w-1/4',
  }

  return (
    <div
      className={clsx(
        'absolute z-10 transition-colors duration-200',
        positionClasses[position],
        isOver ? 'bg-white/10 border-2 border-white/30' : 'bg-transparent'
      )}
      onDragOver={(e) => {
        e.preventDefault()
        setIsOver(true)
      }}
      onDragLeave={() => setIsOver(false)}
      onDrop={(e) => {
        e.preventDefault()
        setIsOver(false)
        const itemId = e.dataTransfer.getData('application/x-dock-item')
        if (itemId) onDrop()
      }}
    />
  )
}

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

function tabLabel(contentId: string, dockItems: ReturnType<typeof useIDEStore.getState>['dockItems']): string {
  if (contentId.startsWith('file:')) return contentId.slice(5).split('/').pop() ?? contentId.slice(5)
  if (contentId.startsWith('newfile:')) return `Untitled-${contentId.slice(8)}`
  return dockItems.find(i => i.id === contentId)?.name ?? contentId
}

function PanelContent({ node, agents, logLines }: { node: PanelNode; agents: AgentStatus[]; logLines: string[] }) {
  const { removePanel, addPanel, switchTab, closeTab, draggedDockItem, projects, activeProjectId, dockItems, openFile, dirtyFiles, flashPanelId } = useIDEStore()
  const isFlashing = flashPanelId === node.id
  // Fall back to activeProjectId if the panel's projectId doesn't match any loaded project
  const project = projects.find((p) => p.id === node.projectId)
    ?? projects.find((p) => p.id === activeProjectId)
  const dockItem = dockItems.find((i) => i.id === node.contentId)

  const isFilePanelId = node.contentId.startsWith('file:')
  const isNewFilePanelId = node.contentId.startsWith('newfile:')

  // For explorer panels, accent color comes from the effective project's color
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

  // Close context menu on outside click
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
  const displayName = isFilePanelId
    ? (node.contentId.slice(5).split('/').pop() ?? node.contentId.slice(5))
    : isNewFilePanelId
    ? `Untitled-${node.contentId.slice(8)}`
    : dockItem?.name || node.contentId

  const hasTabs = node.tabs.length > 1

  const renderContent = () => {
    if (isFilePanelId) {
      return <FileEditorPanel path={node.contentId.slice(5)} panelId={node.id} />
    }
    if (isNewFilePanelId) {
      return <FileEditorPanel path={node.contentId} panelId={node.id} />
    }
    switch (node.contentId) {
      case 'explorer':
        return <ExplorerPanel onOpenFile={openFile} panelId={node.id} pinnedProjectId={node.pinnedProjectId} accentColor={accentColor ?? undefined} />
      case 'terminal':
        return <TerminalPanel sessionId={node.sessionId ?? node.id} />
      case 'agents':
        return <AgentPanel agents={agents} />
      case 'log':
        return <LogPanel lines={logLines} />
      case 'chat':
        return <ChatPanel openFiles={[]} activeTab={null} />
      case 'diff':
        return <DiffPanel />
      case 'editor':
        return (
          <div className="flex-1 flex items-center justify-center text-white/30 text-sm">
            Open a file from the explorer
          </div>
        )
      default:
        if (node.contentId.startsWith('cli-')) {
          return <TerminalPanel sessionId={node.sessionId ?? node.id} executable={dockItem?.command} />
        }
        return (
          <div className="flex-1 flex items-center justify-center text-white/30 text-sm">
            {node.contentId}
          </div>
        )
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

  return (
    <div
      className={clsx(
        'relative w-full h-full bg-black/5 rounded-lg flex flex-col overflow-hidden group backdrop-blur-sm',
        isFlashing && 'animate-panel-flash'
      )}
      style={{ border: `1px solid ${isFlashing ? 'rgba(255,255,255,0.5)' : borderColor}`, transition: 'border-color 0.3s' }}
    >
      {/* Header: tab bar (multi-tab) or simple title (single) */}
      <div
        className="flex items-center shrink-0 overflow-hidden"
        style={{ backgroundColor: headerBg, borderBottom: `1px solid ${headerBorder}` }}
        onContextMenu={(e) => { e.preventDefault(); setContextMenu({ x: e.clientX, y: e.clientY }) }}
      >
        {hasTabs ? (
          /* Tab bar */
          <div className="flex items-stretch flex-1 overflow-x-auto overflow-y-hidden min-w-0 scrollbar-none">
            {node.tabs.map((tabId) => {
              const isActive = tabId === node.contentId
              const label = tabLabel(tabId, dockItems)
              const iconUrl = tabId.startsWith('file:')
                ? getFileIconUrl(label, false, false)
                : null
              return (
                <div
                  key={tabId}
                  className={clsx(
                    'group/tab flex items-center gap-1.5 px-3 h-8 shrink-0 cursor-pointer border-r transition-colors select-none',
                    isActive
                      ? 'text-white/80 bg-white/5 border-r-white/10'
                      : 'text-white/35 hover:text-white/60 border-r-white/5 hover:bg-white/[0.03]'
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
                        : 'text-transparent group-hover/tab:text-white/30 hover:!text-white/70'
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
          /* Single-panel title */
          <div className="h-8 flex items-center flex-1 min-w-0 px-3 gap-1.5">
            <span style={{ display: 'flex', alignItems: 'center', opacity: 0.6, flexShrink: 0, color: titleColor }}>
              {getPanelIcon(node.kind, dockItem?.icon)}
            </span>
            <span className="text-xs font-medium truncate transition-colors" style={{ color: titleColor }}>
              {dirtyFiles.has(node.contentId) ? `${displayName} ●` : displayName}
            </span>
          </div>
        )}

        {/* Unsaved badge for new files */}
        {isNewFilePanelId && (
          <span className="shrink-0 text-xs text-white/25 px-1">unsaved</span>
        )}
        {/* Project badge */}
        {project && (
          <span className="shrink-0 text-xs text-white/25 px-1">
            [{project.name}]
          </span>
        )}
        <button
          onClick={() => {
            if (node.kind === 'terminal') {
              void fetch(`/terminal/close?id=${encodeURIComponent(node.id)}`, { method: 'POST' })
            }
            removePanel(node.id)
          }}
          className="shrink-0 text-white/30 hover:text-white/90 opacity-0 group-hover:opacity-100 transition-opacity px-2 h-8 flex items-center"
        >
          <X className="w-3.5 h-3.5" />
        </button>
      </div>

      <div className="flex-1 overflow-hidden min-h-0">
        {renderContent()}
      </div>

      {draggedDockItem && (
        <>
          <DropZone position="top" onDrop={() => addPanel(draggedDockItem, node.id, 'top')} />
          <DropZone position="bottom" onDrop={() => addPanel(draggedDockItem, node.id, 'bottom')} />
          <DropZone position="left" onDrop={() => addPanel(draggedDockItem, node.id, 'left')} />
          <DropZone position="right" onDrop={() => addPanel(draggedDockItem, node.id, 'right')} />
        </>
      )}

      {/* Context menu for kind behavior toggle */}
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

export function WorkspaceArea({ agents, logLines }: WorkspaceAreaProps) {
  const { workspaces, activeWorkspaceId, draggedDockItem, addPanel } = useIDEStore()
  const activeWorkspace = workspaces.find((w) => w.id === activeWorkspaceId)

  if (!activeWorkspace) return null

  // If the root node is a bare panel (no split), wrap it in a Group so
  // react-resizable-panels never sees a Panel without a Group ancestor.
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
    <div className="flex-1 h-full p-2 overflow-hidden relative">
      {layoutEl ?? (
        <div
          className="w-full h-full border-2 border-dashed border-white/10 rounded-xl flex flex-col items-center justify-center text-white/40 bg-black/5 gap-3"
          onDragOver={(e) => e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault()
            const itemId = e.dataTransfer.getData('application/x-dock-item')
            if (itemId) addPanel(itemId)
          }}
        >
          <span className="text-4xl opacity-20">◎</span>
          <span className="text-sm">Drag an item from the dock or click to add a panel</span>
          {draggedDockItem && (
            <span className="text-xs text-white/20">Drop here to open</span>
          )}
        </div>
      )}
    </div>
  )
}
