import { useIDEStore } from '../../store/useIDEStore'
import { useT } from '../../lib/i18n'
import {
  Plus,
  LayoutTemplate,
  PanelLeftClose,
  PanelLeftOpen,
  PanelRightClose,
  PanelRightOpen,
  PanelBottom,
  PanelTop,
  PanelLeft,
  PanelRight,
  Settings,
  Pencil,
  Copy,
  Trash2,
  Pin,
  PinOff,
} from 'lucide-react'
import { clsx } from 'clsx'
import { motion, type PanInfo } from 'framer-motion'
import { useState, useRef, useEffect, useCallback } from 'react'
import { getCurrentWindow } from '@tauri-apps/api/window'

interface ContextMenu {
  wsId: string
  x: number
  y: number
}

export function WorkspaceTabs() {
  const t = useT()
  const {
    workspaces,
    activeWorkspaceId,
    pinnedWorkspaceIds,
    setActiveWorkspace,
    addWorkspace,
    renameWorkspace,
    removeWorkspace,
    duplicateWorkspace,
    togglePinWorkspace,
    isSidePanelOpen,
    setSidePanelOpen,
    sidePanelPosition,
    setSidePanelPosition,
    isDockPinned,
    isDockOpen,
    setDockOpen,
    dockPosition,
    setSettingsOpen,
  } = useIDEStore()

  const pinnedWorkspaces = workspaces.filter(ws => pinnedWorkspaceIds.includes(ws.id))
  const scrollableWorkspaces = workspaces.filter(ws => !pinnedWorkspaceIds.includes(ws.id))

  const [contextMenu, setContextMenu] = useState<ContextMenu | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const renameInputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (renamingId && renameInputRef.current) {
      renameInputRef.current.focus()
      renameInputRef.current.select()
    }
  }, [renamingId])

  const closeMenu = useCallback(() => setContextMenu(null), [])

  useEffect(() => {
    if (!contextMenu) return
    window.addEventListener('click', closeMenu)
    window.addEventListener('contextmenu', closeMenu)
    return () => {
      window.removeEventListener('click', closeMenu)
      window.removeEventListener('contextmenu', closeMenu)
    }
  }, [contextMenu, closeMenu])

  const handleContextMenu = (e: React.MouseEvent, wsId: string) => {
    e.preventDefault()
    e.stopPropagation()
    setContextMenu({ wsId, x: e.clientX, y: e.clientY })
  }

  const startRename = (wsId: string, currentName: string) => {
    setContextMenu(null)
    setRenamingId(wsId)
    setRenameValue(currentName)
  }

  const commitRename = () => {
    if (renamingId && renameValue.trim()) {
      renameWorkspace(renamingId, renameValue.trim())
    }
    setRenamingId(null)
  }

  const handleDragEnd = (_event: MouseEvent | TouchEvent | PointerEvent, info: PanInfo) => {
    const { innerWidth } = window
    if (info.point.x > innerWidth / 2) {
      setSidePanelPosition('right')
    } else {
      setSidePanelPosition('left')
    }
  }

  const ToggleIcon =
    sidePanelPosition === 'left'
      ? isSidePanelOpen
        ? PanelLeftClose
        : PanelLeftOpen
      : isSidePanelOpen
        ? PanelRightClose
        : PanelRightOpen

  const DockToggleIcon =
    dockPosition.edge === 'bottom'
      ? PanelBottom
      : dockPosition.edge === 'top'
        ? PanelTop
        : dockPosition.edge === 'left'
          ? PanelLeft
          : PanelRight

  // Detect macOS in Tauri to pad around traffic light buttons
  const isMacosTauri =
    typeof window !== 'undefined' &&
    navigator.userAgent.includes('Macintosh') &&
    ('__TAURI_INTERNALS__' in window || '__TAURI__' in window)

  const isTauri = typeof window !== 'undefined' && ('__TAURI_INTERNALS__' in window || '__TAURI__' in window)

  const handleSpacerMouseDown = useCallback((e: React.MouseEvent) => {
    if (!isTauri) return
    e.preventDefault()
    getCurrentWindow().startDragging().catch(() => {})
  }, [isTauri])

  const handleSpacerDblClick = useCallback(async () => {
    if (!isTauri) return
    const win = getCurrentWindow()
    const maximized = await win.isMaximized()
    if (maximized) { win.unmaximize() } else { win.maximize() }
  }, [isTauri])

  const renderTab = (ws: typeof workspaces[0]) => (
    <div key={ws.id} className="relative shrink-0">
      {renamingId === ws.id ? (
        <input
          ref={renameInputRef}
          value={renameValue}
          onChange={e => setRenameValue(e.target.value)}
          onBlur={commitRename}
          onKeyDown={e => {
            if (e.key === 'Enter') commitRename()
            if (e.key === 'Escape') setRenamingId(null)
          }}
          className="px-3 py-1.5 rounded-md text-sm font-medium bg-white/10 border border-white/20 text-white outline-none w-32"
        />
      ) : (
        <button
          onClick={() => setActiveWorkspace(ws.id)}
          onContextMenu={e => handleContextMenu(e, ws.id)}
          onDoubleClick={() => startRename(ws.id, ws.name)}
          className={clsx(
            'flex items-center gap-2 px-4 py-1.5 rounded-md text-sm font-medium transition-all border border-transparent select-none',
            activeWorkspaceId === ws.id
              ? 'bg-white/10 text-white shadow-sm border-white/5'
              : 'text-white/50 hover:bg-white/5 hover:text-white/80'
          )}
        >
          {pinnedWorkspaceIds.includes(ws.id)
            ? <Pin className="w-3 h-3 opacity-50" />
            : <LayoutTemplate className="w-4 h-4 opacity-70" />
          }
          {ws.name}
        </button>
      )}
    </div>
  )

  return (
    <div
      className="h-11 bg-black/10 border-b border-white/10 flex items-center px-2 gap-1 relative shrink-0"
      style={isMacosTauri ? { paddingLeft: '88px' } : undefined}
      data-tauri-drag-region
    >
      {sidePanelPosition === 'left' && (
        <>
          <motion.button
            drag="x"
            dragConstraints={{ left: 0, right: 0 }}
            dragElastic={1}
            onDragEnd={handleDragEnd}
            onClick={() => setSidePanelOpen(!isSidePanelOpen)}
            className="p-1.5 text-white/50 hover:text-white/90 hover:bg-white/10 rounded-md transition-colors cursor-grab active:cursor-grabbing z-10 shrink-0"
            title={t('workspace.toggleSidePanel')}
          >
            <ToggleIcon className="w-4 h-4" />
          </motion.button>
          <button
            onClick={() => setSettingsOpen(true)}
            className="p-1.5 text-white/50 hover:text-white/90 hover:bg-white/10 rounded-md transition-colors z-10 mr-1 shrink-0"
            title={t('workspace.settings')}
          >
            <Settings className="w-4 h-4" />
          </button>
        </>
      )}

      {/* Pinned tabs — always visible, never scroll */}
      {pinnedWorkspaces.length > 0 && (
        <div className="flex items-center gap-1 shrink-0">
          {pinnedWorkspaces.map(renderTab)}
          <div className="w-px h-5 bg-white/10 mx-1 shrink-0" />
        </div>
      )}

      {/* Scrollable tabs + add button — shrinks when space is tight, scrolls internally */}
      <div className="flex items-center gap-1 overflow-x-auto no-scrollbar min-w-0 shrink">
        {scrollableWorkspaces.map(renderTab)}
        <button
          onClick={addWorkspace}
          className="p-1.5 text-white/50 hover:text-white/90 hover:bg-white/10 rounded-md ml-1 transition-colors shrink-0"
        >
          <Plus className="w-4 h-4" />
        </button>
      </div>

      {/* Flexible spacer — drag region for moving the app window */}
      <div
        className="flex-1 min-w-0 self-stretch"
        data-tauri-drag-region
        onMouseDown={handleSpacerMouseDown}
        onDoubleClick={handleSpacerDblClick}
      />

      {/* Context menu */}
      {contextMenu && (() => {
        const ws = workspaces.find(w => w.id === contextMenu.wsId)
        if (!ws) return null
        const isPinned = pinnedWorkspaceIds.includes(ws.id)
        return (
          <div
            className="fixed z-[500] min-w-[160px] rounded-lg border border-white/10 bg-[#111] shadow-2xl py-1 text-sm"
            style={{ left: contextMenu.x, top: contextMenu.y }}
            onClick={e => e.stopPropagation()}
          >
            <button
              onClick={() => startRename(ws.id, ws.name)}
              className="w-full flex items-center gap-2.5 px-3 py-1.5 text-white/70 hover:bg-white/10 hover:text-white transition-colors"
            >
              <Pencil className="w-3.5 h-3.5 opacity-60" /> {t('workspace.rename')}
            </button>
            <button
              onClick={() => { togglePinWorkspace(ws.id); closeMenu() }}
              className="w-full flex items-center gap-2.5 px-3 py-1.5 text-white/70 hover:bg-white/10 hover:text-white transition-colors"
            >
              {isPinned
                ? <><PinOff className="w-3.5 h-3.5 opacity-60" /> {t('workspace.unpin')}</>
                : <><Pin className="w-3.5 h-3.5 opacity-60" /> {t('workspace.pin')}</>
              }
            </button>
            <button
              onClick={() => { duplicateWorkspace(ws.id); closeMenu() }}
              className="w-full flex items-center gap-2.5 px-3 py-1.5 text-white/70 hover:bg-white/10 hover:text-white transition-colors"
            >
              <Copy className="w-3.5 h-3.5 opacity-60" /> {t('workspace.duplicate')}
            </button>
            {workspaces.length > 1 && (
              <>
                <div className="my-1 border-t border-white/10" />
                <button
                  onClick={() => { removeWorkspace(ws.id); closeMenu() }}
                  className="w-full flex items-center gap-2.5 px-3 py-1.5 text-red-400 hover:bg-red-500/10 transition-colors"
                >
                  <Trash2 className="w-3.5 h-3.5 opacity-70" /> {t('workspace.close')}
                </button>
              </>
            )}
          </div>
        )
      })()}

      {isDockPinned && (
        <button
          onClick={() => setDockOpen(!isDockOpen)}
          className={clsx(
            'p-1.5 rounded-md ml-1 transition-colors z-10',
            isDockOpen ? 'text-white/90 bg-white/10' : 'text-white/50 hover:text-white/90 hover:bg-white/10'
          )}
          title={t('workspace.toggleDock')}
        >
          <DockToggleIcon className="w-4 h-4" />
        </button>
      )}

      {sidePanelPosition === 'right' && (
        <>
          <button
            onClick={() => setSettingsOpen(true)}
            className="p-1.5 text-white/50 hover:text-white/90 hover:bg-white/10 rounded-md transition-colors z-10 ml-1"
            title={t('workspace.settings')}
          >
            <Settings className="w-4 h-4" />
          </button>
          <motion.button
            drag="x"
            dragConstraints={{ left: 0, right: 0 }}
            dragElastic={1}
            onDragEnd={handleDragEnd}
            onClick={() => setSidePanelOpen(!isSidePanelOpen)}
            className="p-1.5 text-white/50 hover:text-white/90 hover:bg-white/10 rounded-md transition-colors cursor-grab active:cursor-grabbing z-10"
            title={t('workspace.toggleSidePanel')}
          >
            <ToggleIcon className="w-4 h-4" />
          </motion.button>
        </>
      )}
    </div>
  )
}
