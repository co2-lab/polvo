import { useState, useEffect, useRef } from 'react'
import { createPortal } from 'react-dom'
import { useIDEStore } from '../../store/useIDEStore'
import { useT } from '../../lib/i18n'
import { FolderGit2, LayoutGrid, Blocks, Plus, FolderOpen, Pencil, Eye, EyeOff, Trash2, X, ChevronDown, ChevronRight } from 'lucide-react'
import { clsx } from 'clsx'
import type { LayoutNode, PanelNode } from '../../types/ide'
import { getPanelIcon } from '../icons/panelIcon'
import { getKind } from '../../store/useIDEStore'
import { getFileIconUrl } from '../../lib/fileIcons'
import { getIconColor } from '../../lib/fileIconColor'
import { getBaseTitle, computeTitleIndices, resolveTitle } from '../../lib/titleUtils'

function collectPanels(node: LayoutNode): PanelNode[] {
  if (node.type === 'panel') return [node]
  return node.children.flatMap(collectPanels)
}

async function pickDirectory(): Promise<string | null> {
  try {
    const { open } = await import('@tauri-apps/plugin-dialog')
    const result = await open({ directory: true, multiple: false, title: 'Select project folder' })
    return typeof result === 'string' ? result : null
  } catch (e) {
    console.error('dialog error', e)
    return null
  }
}

type Tab = 'projects' | 'panels' | 'marketplace'

interface ContextMenuProps {
  x: number
  y: number
  project: { id: string; name: string; hidden: boolean; path: string; color?: string; icon?: string }
  onClose: () => void
  onRename: () => void
}

function ProjectContextMenu({ x, y, project, onClose, onRename }: ContextMenuProps) {
  const t = useT()
  const { toggleProjectVisibility, removeProject } = useIDEStore()
  const ref = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [onClose])

  return createPortal(
    <div
      ref={ref}
      className="fixed z-50 min-w-[160px] rounded-lg overflow-hidden shadow-xl border border-white/10 bg-[#111] text-xs py-1"
      style={{ top: y, left: x }}
    >
      <button
        className="w-full flex items-center gap-2.5 px-3 py-2 text-white/60 hover:bg-white/5 hover:text-white/90 transition-colors"
        onClick={() => { onRename(); onClose() }}
      >
        <Pencil className="w-3.5 h-3.5 shrink-0" /> {t('sidebar.project.rename')}
      </button>
      <button
        className="w-full flex items-center gap-2.5 px-3 py-2 text-white/60 hover:bg-white/5 hover:text-white/90 transition-colors"
        onClick={() => { toggleProjectVisibility(project.id); onClose() }}
      >
        {project.hidden
          ? <><Eye className="w-3.5 h-3.5 shrink-0" /> {t('sidebar.project.showPanels')}</>
          : <><EyeOff className="w-3.5 h-3.5 shrink-0" /> {t('sidebar.project.hidePanels')}</>
        }
      </button>
      <div className="my-1 border-t border-white/5" />
      <button
        className="w-full flex items-center gap-2.5 px-3 py-2 text-red-400/70 hover:bg-white/5 hover:text-red-400 transition-colors"
        onClick={() => { void removeProject(project.id); onClose() }}
      >
        <Trash2 className="w-3.5 h-3.5 shrink-0" /> {t('sidebar.project.remove')}
      </button>
    </div>,
    document.body
  )
}

function ProjectItem({ p, isActive }: { p: { id: string; name: string; hidden: boolean; path: string; color?: string; icon?: string }; isActive: boolean }) {
  const { setActiveProject, toggleProjectVisibility, removeProject, addProject, openProjectConfig } = useIDEStore()
  const [editing, setEditing] = useState(false)
  const [editName, setEditName] = useState(p.name)
  const [hovered, setHovered] = useState(false)
  const [contextMenu, setContextMenu] = useState<{ x: number; y: number } | null>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const c = p.color

  useEffect(() => {
    if (editing) { setEditName(p.name); inputRef.current?.select() }
  }, [editing, p.name])

  const commitRename = async () => {
    const trimmed = editName.trim()
    if (trimmed && trimmed !== p.name) {
      await removeProject(p.id)
      await addProject(trimmed, p.path)
    } else {
      setEditName(p.name)
    }
    setEditing(false)
  }

  const bgStyle = c
    ? isActive
      ? { backgroundColor: `color-mix(in srgb, ${c} 20%, transparent)`, borderColor: `color-mix(in srgb, ${c} 40%, transparent)` }
      : hovered
        ? { backgroundColor: `color-mix(in srgb, ${c} 12%, transparent)` }
        : {}
    : {}

  return (
    <>
      <div
        style={{ height: 30, ...bgStyle }}
        className={clsx(
          'flex items-center gap-1.5 px-2 rounded-md cursor-pointer transition-all border border-transparent select-none',
          !c && (isActive ? 'bg-white/10 text-white border-white/5' : 'text-white/70 hover:bg-white/5 hover:text-white')
        )}
        onClick={() => { if (!editing) { setActiveProject(p.id); openProjectConfig(p.id) } }}
        onMouseEnter={() => setHovered(true)}
        onMouseLeave={() => setHovered(false)}
        onContextMenu={(e) => { e.preventDefault(); setContextMenu({ x: e.clientX, y: e.clientY }) }}
      >
        <FolderOpen className="w-3.5 h-3.5 shrink-0" style={{ color: c ? `color-mix(in srgb, ${c} 60%, white)` : 'rgba(255,255,255,0.3)' }} />

        {editing ? (
          <input
            ref={inputRef}
            className="flex-1 min-w-0 bg-white/10 border border-white/20 rounded px-1.5 py-0.5 text-xs text-white outline-none focus:border-white/40"
            value={editName}
            onChange={(e) => setEditName(e.target.value)}
            onBlur={commitRename}
            onKeyDown={(e) => {
              if (e.key === 'Enter') { e.preventDefault(); void commitRename() }
              if (e.key === 'Escape') { setEditName(p.name); setEditing(false) }
            }}
            onClick={(e) => e.stopPropagation()}
          />
        ) : (
          <span
            className="flex-1 min-w-0 truncate font-medium text-xs"
            style={{ color: c ? (isActive || hovered ? c : `color-mix(in srgb, ${c} 70%, rgba(255,255,255,0.5))`) : undefined }}
            title={p.path}
          >
            {p.name}
          </span>
        )}

        {(hovered || isActive) && !editing && (
          <button
            className="shrink-0 p-0.5 text-white/30 hover:text-white/70 transition-colors"
            onClick={(e) => { e.stopPropagation(); toggleProjectVisibility(p.id) }}
            title={p.hidden ? 'Show panels' : 'Hide panels'}
          >
            {p.hidden ? <Eye className="w-3 h-3" /> : <EyeOff className="w-3 h-3" />}
          </button>
        )}
      </div>

      {contextMenu && (
        <ProjectContextMenu
          x={contextMenu.x}
          y={contextMenu.y}
          project={p}
          onClose={() => setContextMenu(null)}
          onRename={() => setEditing(true)}
        />
      )}
    </>
  )
}

// editor and diff panels always show as grouped tree — regardless of persisted kindBehaviors
const GROUPED_KINDS = new Set(['editor', 'diff'])

function useFileColor(tabId: string): string | null {
  const [color, setColor] = useState<string | null>(null)
  useEffect(() => {
    if (!tabId.startsWith('file:') && !tabId.startsWith('newfile:')) return
    const filename = tabId.startsWith('file:')
      ? tabId.slice(5).split('/').pop() ?? ''
      : `Untitled`
    const url = getFileIconUrl(filename, false, false)
    getIconColor(url).then(setColor)
  }, [tabId])
  return color
}

function FileTabItem({ tabId, panelId, isActive, label }: {
  tabId: string
  panelId: string
  isActive: boolean
  label: string
}) {
  const { closeTab, focusPanel } = useIDEStore()
  const color = useFileColor(tabId)
  const filename = label
  const iconUrl = (tabId.startsWith('file:') || tabId.startsWith('newfile:'))
    ? getFileIconUrl(filename, false, false)
    : null

  const [hovered, setHovered] = useState(false)

  return (
    <div
      className="flex items-center gap-1.5 px-1 py-1 rounded group/tab cursor-pointer transition-colors"
      style={{ backgroundColor: hovered && color ? `color-mix(in srgb, ${color} 8%, transparent)` : undefined }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      onClick={() => { focusPanel(panelId); useIDEStore.getState().switchTab(panelId, tabId) }}
    >
      <span className="shrink-0 opacity-70">
        {iconUrl
          ? <img src={iconUrl} alt="" className="w-3.5 h-3.5" draggable={false} />
          : getPanelIcon(getKind(tabId))
        }
      </span>
      <span
        className="flex-1 min-w-0 truncate text-xs transition-colors"
        style={{ color: color
          ? isActive
            ? color
            : `color-mix(in srgb, ${color} 50%, rgba(255,255,255,0.25))`
          : isActive ? 'rgba(255,255,255,0.85)' : 'rgba(255,255,255,0.40)'
        }}
      >
        {label}
      </span>
      <button
        onClick={(e) => { e.stopPropagation(); closeTab(panelId, tabId) }}
        className="shrink-0 opacity-0 group-hover/tab:opacity-100 p-0.5 rounded transition-all"
        style={{ color: color ? `color-mix(in srgb, ${color} 60%, rgba(255,255,255,0.3))` : 'rgba(255,255,255,0.3)' }}
        title="Close tab"
      >
        <X className="w-3 h-3" />
      </button>
    </div>
  )
}

function PanelRow({ panel, projectColor, isGrouped, isOpen, onToggle, titleIndices }: {
  panel: PanelNode
  projectColor: string | null
  isGrouped: boolean
  isOpen: boolean
  onToggle: () => void
  titleIndices: Map<string, number | null>
}) {
  const t = useT()
  const { removePanel, focusPanel, dockItems } = useIDEStore()
  const kind = panel.kind ?? getKind(panel.contentId)
  const dockItem = dockItems.find(i => i.id === panel.contentId)
  const label = isGrouped
    ? (kind === 'editor' ? t('panel.editor') : dockItem?.name ?? kind)
    : resolveTitle(getBaseTitle(panel.contentId, dockItems), panel.id, titleIndices)
  const color = projectColor

  return (
    <div
      className="flex items-center gap-1.5 px-2 py-1.5 rounded-md group cursor-pointer transition-colors"
      onClick={() => !isGrouped && focusPanel(panel.id)}
      style={color ? {
        ['--hover-bg' as string]: `color-mix(in srgb, ${color} 10%, transparent)`,
      } : undefined}
      onMouseEnter={e => { if (color) (e.currentTarget as HTMLElement).style.backgroundColor = `color-mix(in srgb, ${color} 8%, transparent)` }}
      onMouseLeave={e => { (e.currentTarget as HTMLElement).style.backgroundColor = '' }}
    >
      {isGrouped ? (
        <button
          onClick={(e) => { e.stopPropagation(); onToggle() }}
          className="shrink-0 flex items-center gap-1.5 transition-colors"
          style={{ color: color ? `color-mix(in srgb, ${color} 50%, rgba(255,255,255,0.4))` : 'rgba(255,255,255,0.4)' }}
        >
          {isOpen ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
          {getPanelIcon(kind, dockItem?.icon)}
        </button>
      ) : (
        <span
          className="shrink-0"
          style={{ color: color ? `color-mix(in srgb, ${color} 60%, rgba(255,255,255,0.4))` : 'rgba(255,255,255,0.4)' }}
        >
          {getPanelIcon(kind, dockItem?.icon)}
        </span>
      )}

      <span
        className="flex-1 min-w-0 truncate text-xs"
        style={{ color: color ? `color-mix(in srgb, ${color} 70%, rgba(255,255,255,0.7))` : 'rgba(255,255,255,0.7)' }}
      >
        {label}
      </span>

      {isGrouped && (
        <span className="text-[10px] shrink-0" style={{ color: 'rgba(255,255,255,0.2)' }}>{panel.tabs.length}</span>
      )}

      <button
        onClick={(e) => { e.stopPropagation(); removePanel(panel.id) }}
        className="shrink-0 opacity-0 group-hover:opacity-100 p-0.5 rounded transition-all"
        style={{ color: color ? `color-mix(in srgb, ${color} 50%, rgba(255,255,255,0.3))` : 'rgba(255,255,255,0.3)' }}
        title="Close panel"
      >
        <X className="w-3 h-3" />
      </button>
    </div>
  )
}

function PanelsTab() {
  const t = useT()
  const { workspaces, activeWorkspaceId, projects, activeProjectId, dockItems } = useIDEStore()
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())

  const titleIndices = computeTitleIndices(workspaces, dockItems)

  const toggle = (id: string) => setCollapsed(prev => {
    const next = new Set(prev)
    next.has(id) ? next.delete(id) : next.add(id)
    return next
  })

  return (
    <div className="flex flex-col gap-3">
      {workspaces.map(ws => {
        const panels = ws.layout ? collectPanels(ws.layout) : []
        return (
          <div key={ws.id}>
            <div className="px-1 pb-1 text-[10px] font-semibold text-white/30 uppercase tracking-wider flex items-center gap-1">
              <span className={clsx('w-1.5 h-1.5 rounded-full shrink-0', ws.id === activeWorkspaceId ? 'bg-white/50' : 'bg-white/15')} />
              {ws.name}
            </div>
            {panels.length === 0 ? (
              <div className="px-2 py-2 text-xs text-white/25 italic">{t('sidebar.panels.empty')}</div>
            ) : (
              <div className="flex flex-col gap-0.5">
                {panels.map(panel => {
                  const kind = panel.kind ?? getKind(panel.contentId)
                  const isGrouped = GROUPED_KINDS.has(kind)
                  const isOpen = !collapsed.has(panel.id)
                  // Resolve color per panel kind
                  let projectColor: string | null = null
                  if (kind === 'explorer') {
                    const p = projects.find(p => p.id === (panel.pinnedProjectId ?? activeProjectId))
                    projectColor = p?.color ?? null
                  } else if (kind === 'editor') {
                    const tabProjectIds = Object.values(panel.tabProjects ?? {})
                    const unique = [...new Set(tabProjectIds)]
                    const p = unique.length === 1
                      ? projects.find(p => p.id === unique[0])
                      : projects.find(p => p.id === panel.projectId)
                    projectColor = p?.color ?? null
                  } else {
                    const p = projects.find(p => p.id === panel.projectId)
                      ?? projects.find(p => p.id === activeProjectId)
                    projectColor = p?.color ?? null
                  }

                  return (
                    <div key={panel.id}>
                      <PanelRow
                        panel={panel}
                        projectColor={projectColor}
                        isGrouped={isGrouped}
                        isOpen={isOpen}
                        onToggle={() => toggle(panel.id)}
                        titleIndices={titleIndices}
                      />
                      {isGrouped && isOpen && (
                        <div className="ml-5 border-l border-white/5 pl-2 flex flex-col gap-0.5 mb-0.5">
                          {panel.tabs.map(tabId => (
                            <FileTabItem
                              key={tabId}
                              tabId={tabId}
                              panelId={panel.id}
                              isActive={tabId === panel.contentId}
                              label={resolveTitle(getBaseTitle(panel.tabContentIds?.[tabId] ?? tabId, dockItems), `${panel.id}:${tabId}`, titleIndices)}
                            />
                          ))}
                        </div>
                      )}
                    </div>
                  )
                })}
              </div>
            )}
          </div>
        )
      })}
    </div>
  )
}

export function SidePanel() {
  const t = useT()
  const { projects, activeProjectId, addProject, sidePanelPosition } = useIDEStore()
  const [activeTab, setActiveTab] = useState<Tab>('projects')
  const [showPathInput, setShowPathInput] = useState(false)
  const [manualPath, setManualPath] = useState('')
  const [width, setWidth] = useState(256)
  const pathInputRef = useRef<HTMLInputElement>(null)
  const dragging = useRef(false)
  const startX = useRef(0)
  const startW = useRef(0)

  const onHandleMouseDown = (e: React.MouseEvent) => {
    e.preventDefault()
    dragging.current = true
    startX.current = e.clientX
    startW.current = width
    const onMove = (ev: MouseEvent) => {
      if (!dragging.current) return
      const delta = sidePanelPosition === 'left' ? ev.clientX - startX.current : startX.current - ev.clientX
      setWidth(Math.max(160, Math.min(480, startW.current + delta)))
    }
    const onUp = () => {
      dragging.current = false
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

  // Projects are loaded by App.tsx once the server is confirmed up (ready state).
  // No need to fetch here.
  useEffect(() => { if (showPathInput) pathInputRef.current?.focus() }, [showPathInput])

  const handleAddProject = async () => {
    const isTauri = '__TAURI_INTERNALS__' in window || '__TAURI__' in window
    if (isTauri) {
      const selected = await pickDirectory()
      if (selected) {
        const name = selected.split('/').filter(Boolean).pop() ?? selected
        await addProject(name, selected)
      }
    } else {
      setShowPathInput(true)
    }
  }

  const handleManualPathSubmit = async () => {
    const path = manualPath.trim()
    if (!path) return
    const name = path.split('/').filter(Boolean).pop() ?? path
    await addProject(name, path)
    setManualPath('')
    setShowPathInput(false)
  }

  return (
    <div className="relative h-full flex shrink-0" style={{ width }}>
      <div className="w-full h-full bg-black/5 flex flex-col overflow-hidden backdrop-blur-md border-white/10"
        style={{ borderRight: sidePanelPosition === 'left' ? '1px solid rgba(255,255,255,0.10)' : undefined, borderLeft: sidePanelPosition === 'right' ? '1px solid rgba(255,255,255,0.10)' : undefined }}
      >
      <div className="flex items-center border-b border-white/10 shrink-0 p-1 gap-1">
        <button onClick={() => setActiveTab('projects')} className={clsx('flex-1 py-1.5 flex justify-center items-center transition-all rounded-md', activeTab === 'projects' ? 'bg-white/10 text-white shadow-sm' : 'text-white/50 hover:text-white/90 hover:bg-white/5')} title="Projects">
          <FolderGit2 className="w-4 h-4" />
        </button>
        <button onClick={() => setActiveTab('panels')} className={clsx('flex-1 py-1.5 flex justify-center items-center transition-all rounded-md', activeTab === 'panels' ? 'bg-white/10 text-white shadow-sm' : 'text-white/50 hover:text-white/90 hover:bg-white/5')} title="Panels">
          <LayoutGrid className="w-4 h-4" />
        </button>
        <button onClick={() => setActiveTab('marketplace')} className={clsx('flex-1 py-1.5 flex justify-center items-center transition-all rounded-md', activeTab === 'marketplace' ? 'bg-white/10 text-white shadow-sm' : 'text-white/50 hover:text-white/90 hover:bg-white/5')} title="Marketplace">
          <Blocks className="w-4 h-4" />
        </button>
      </div>

      <div className="flex-1 overflow-y-auto p-2 flex flex-col">
        {activeTab === 'projects' && (
          <div className="flex flex-col gap-0.5 flex-1">
            {projects.length === 0 && (
              <div className="px-3 py-8 text-xs text-white/30 text-center">{t('sidebar.noProjects')}</div>
            )}
            {projects.map((p) => (
              <ProjectItem key={p.id} p={p} isActive={activeProjectId === p.id} />
            ))}

            <div className="mt-auto pt-2">
              {showPathInput ? (
                <div className="flex flex-col gap-1.5 px-1">
                  <input
                    ref={pathInputRef}
                    type="text"
                    placeholder="/path/to/project"
                    value={manualPath}
                    onChange={(e) => setManualPath(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') void handleManualPathSubmit()
                      if (e.key === 'Escape') { setShowPathInput(false); setManualPath('') }
                    }}
                    className="w-full bg-white/5 border border-white/10 rounded px-2 py-1.5 text-xs text-white placeholder-white/25 outline-none focus:border-white/25"
                  />
                  <div className="flex gap-1">
                    <button onClick={() => void handleManualPathSubmit()} disabled={!manualPath.trim()} className="flex-1 py-1 rounded text-xs bg-white/10 hover:bg-white/15 text-white/80 disabled:opacity-40 transition-colors">{t('action.add')}</button>
                    <button onClick={() => { setShowPathInput(false); setManualPath('') }} className="flex-1 py-1 rounded text-xs text-white/40 hover:text-white/70 hover:bg-white/5 transition-colors">{t('action.cancel')}</button>
                  </div>
                </div>
              ) : (
                <button
                  onClick={() => void handleAddProject()}
                  className="w-full flex items-center gap-1.5 px-3 py-2 rounded-md text-xs text-white/40 hover:text-white/70 hover:bg-white/5 transition-colors"
                >
                  <Plus className="w-3.5 h-3.5" /> {t('sidebar.addProject')}
                </button>
              )}
            </div>
          </div>
        )}

        {activeTab === 'panels' && <PanelsTab />}
        {activeTab === 'marketplace' && (
          <div className="px-4 py-8 text-xs text-white/40 text-center">Extensions and plugins.</div>
        )}
      </div>
      </div>
      {/* Resize handle */}
      <div
        onMouseDown={onHandleMouseDown}
        className="absolute top-0 bottom-0 w-1 cursor-col-resize hover:bg-white/10 active:bg-white/20 transition-colors z-10"
        style={{ [sidePanelPosition === 'left' ? 'right' : 'left']: 0 }}
      />
    </div>
  )
}
