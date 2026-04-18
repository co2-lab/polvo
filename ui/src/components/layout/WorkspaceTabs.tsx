import React from 'react'
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

function useWorkspaceTabDrop(wsId: string) {
  const { setActiveWorkspace, movePanelToWorkspace, draggedPanelId } = useIDEStore()
  const [flashCount, setFlashCount] = useState(0)
  const [isDragOver, setIsDragOver] = useState(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Clean up timers on unmount
  useEffect(() => () => { if (timerRef.current) clearTimeout(timerRef.current) }, [])

  const onDragOver = useCallback((e: React.DragEvent) => {
    if (!draggedPanelId) return
    e.preventDefault()
    setIsDragOver(true)
  }, [draggedPanelId])

  const onDragLeave = useCallback(() => setIsDragOver(false), [])

  const onDrop = useCallback((e: React.DragEvent) => {
    const panelId = e.dataTransfer.getData('application/x-panel')
    if (!panelId) return
    e.preventDefault()
    setIsDragOver(false)

    // Flash 3 times then switch workspace
    let count = 0
    const flash = () => {
      setFlashCount(c => c + 1)
      count++
      if (count < 3) {
        timerRef.current = setTimeout(flash, 200)
      } else {
        timerRef.current = setTimeout(() => {
          setFlashCount(0)
          setActiveWorkspace(wsId)
          movePanelToWorkspace(panelId, wsId)
        }, 200)
      }
    }
    flash()
  }, [wsId, setActiveWorkspace, movePanelToWorkspace])

  return { isDragOver, flashCount, onDragOver, onDragLeave, onDrop }
}

interface ContextMenu {
  wsId: string
  x: number
  y: number
}

interface WorkspaceTabProps {
  ws: { id: string; name: string }
  isActive: boolean
  isPinned: boolean
  renamingId: string | null
  renameValue: string
  renameInputRef: React.RefObject<HTMLInputElement | null>
  setRenameValue: (v: string) => void
  commitRename: () => void
  setRenamingId: (id: string | null) => void
  setActiveWorkspace: (id: string) => void
  handleContextMenu: (e: React.MouseEvent, wsId: string) => void
  startRename: (wsId: string, name: string) => void
}

function WorkspaceTab({
  ws, isActive, isPinned, renamingId, renameValue, renameInputRef,
  setRenameValue, commitRename, setRenamingId, setActiveWorkspace,
  handleContextMenu, startRename,
}: WorkspaceTabProps) {
  const { isDragOver, flashCount, onDragOver, onDragLeave, onDrop } = useWorkspaceTabDrop(ws.id)
  const isFlashing = flashCount > 0 && flashCount % 2 === 1

  return (
    <div className="relative shrink-0">
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
          className="px-3 py-1.5 text-sm font-medium bg-white/10 border border-white/20 text-white outline-none w-32"
        />
      ) : (
        <button
          onClick={() => setActiveWorkspace(ws.id)}
          onContextMenu={e => handleContextMenu(e, ws.id)}
          onDoubleClick={() => startRename(ws.id, ws.name)}
          onDragOver={onDragOver}
          onDragLeave={onDragLeave}
          onDrop={onDrop}
          className={clsx(
            'group flex items-center gap-2 px-3 h-8 text-sm font-medium transition-all select-none cursor-pointer border-b-2',
            isActive
              ? 'text-white border-[color:var(--primary)] bg-white/5'
              : isDragOver || isFlashing
                ? 'text-white/80 border-[color:var(--primary)] bg-white/10'
                : 'text-white/45 border-transparent hover:text-white/80',
          )}
        >
          {isPinned
            ? <Pin className="w-3 h-3 opacity-50" />
            : <LayoutTemplate className="w-4 h-4 opacity-70" />
          }
          {ws.name}
        </button>
      )}
    </div>
  )
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

  const renderTab = (ws: typeof workspaces[0]) => {
    const isActive = activeWorkspaceId === ws.id
    return <WorkspaceTab key={ws.id} ws={ws} isActive={isActive} isPinned={pinnedWorkspaceIds.includes(ws.id)} renamingId={renamingId} renameValue={renameValue} renameInputRef={renameInputRef} setRenameValue={setRenameValue} commitRename={commitRename} setRenamingId={setRenamingId} setActiveWorkspace={setActiveWorkspace} handleContextMenu={handleContextMenu} startRename={startRename} />
  }

  return (
    <div
      className="h-8 bg-black/10 border-b border-white/10 flex items-center px-2 gap-1 relative shrink-0"
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
