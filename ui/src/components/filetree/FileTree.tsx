import { useEffect, useLayoutEffect, useImperativeHandle, useRef, useState, forwardRef, useCallback } from 'react'
import { createPortal } from 'react-dom'
import { Tree, type NodeRendererProps, type TreeApi } from 'react-arborist'
import { ChevronRight, Loader, FilePlus, FolderPlus, FolderOpen, Scissors, Copy, Clipboard, FileCode, FileText, Pencil, Trash2, GitCompare } from 'lucide-react'
import type { FileEntry } from '../../types/api'
import { getFileIconUrl } from '../../lib/fileIcons'
import { getIconColor } from '../../lib/fileIconColor'
import { API_BASE } from '../../hooks/useSSE'
import { useIDEStore, type CompareItem } from '../../store/useIDEStore'

export interface FileTreeHandle {
  collapseAll: () => void
}

interface FileTreeProps {
  fetchDir: (path: string) => Promise<FileEntry[]>
  onOpenFile: (path: string) => void
  height: number
  projectId?: string
  projectRoot?: string
}

interface TreeData {
  id: string
  name: string
  isDir: boolean
  children?: TreeData[]
}

async function fetchEntries(
  fetchDir: (path: string) => Promise<FileEntry[]>,
  parentId: string | null
): Promise<TreeData[]> {
  const path = parentId ?? '.'
  const entries = await fetchDir(path)
  return entries.map((e) => {
    const id = parentId ? `${parentId}/${e.name}` : e.name
    return { id, name: e.name, isDir: e.is_dir }
  })
}

function updateById(nodes: TreeData[], id: string, fn: (n: TreeData) => TreeData): TreeData[] {
  return nodes.map((n) => {
    if (n.id === id) return fn(n)
    if (n.children) return { ...n, children: updateById(n.children, id, fn) }
    return n
  })
}

function removeById(nodes: TreeData[], id: string): TreeData[] {
  return nodes
    .filter((n) => n.id !== id)
    .map((n) => n.children ? { ...n, children: removeById(n.children, id) } : n)
}

function insertIntoDir(nodes: TreeData[], dirId: string | null, entry: TreeData): TreeData[] {
  if (dirId === null) return [...nodes, entry]
  return nodes.map((n) => {
    if (n.id === dirId) return { ...n, children: [...(n.children ?? []), entry] }
    if (n.children) return { ...n, children: insertIntoDir(n.children, dirId, entry) }
    return n
  })
}

interface LoadCtx {
  loadedDirs: Set<string>
  loadingDirs: Set<string>
  fetchDir: (path: string) => Promise<FileEntry[]>
  onOpenFile: (path: string) => void
  setData: React.Dispatch<React.SetStateAction<TreeData[]>>
  treeRef: React.MutableRefObject<TreeApi<TreeData> | null>
  projectId?: string
  projectRoot?: string
  clipboard: React.MutableRefObject<{ path: string; op: 'cut' | 'copy' } | null>
  reloadDir: (dirId: string | null) => Promise<void>
}

async function openDir(id: string, ctx: LoadCtx) {
  const { loadedDirs, loadingDirs, fetchDir, setData, treeRef } = ctx
  if (loadingDirs.has(id)) return
  if (!loadedDirs.has(id)) {
    loadingDirs.add(id)
    try {
      const children = await fetchEntries(fetchDir, id)
      const withChildren = children.map((e) => (e.isDir ? { ...e, children: [] } : e))
      setData((prev) => updateById(prev, id, (n) => ({ ...n, children: withChildren })))
      loadedDirs.add(id)
    } catch (err) {
      console.error('Failed to load dir', id, err)
      loadingDirs.delete(id)
      return
    } finally {
      loadingDirs.delete(id)
    }
  }
  setTimeout(() => treeRef.current?.open(id), 0)
}

function projectQuery(ctx: LoadCtx) {
  return ctx.projectId ? `?project=${encodeURIComponent(ctx.projectId)}` : ''
}

// ── Context Menu ─────────────────────────────────────────────────────────────

interface CtxMenuState {
  x: number
  y: number
  node: TreeData
}

interface CtxMenuProps {
  state: CtxMenuState
  ctx: LoadCtx
  onClose: () => void
  onStartRename: (node: TreeData) => void
  onStartNewFile: (parentId: string | null) => void
  onStartNewFolder: (parentId: string | null) => void
  onSelectForCompare: (node: TreeData, side: 'left' | 'right') => void
}

function TreeContextMenu({ state, ctx, onClose, onStartRename, onStartNewFile, onStartNewFolder, onSelectForCompare }: CtxMenuProps) {

  const ref = useRef<HTMLDivElement>(null)
  const [pos, setPos] = useState({ top: state.y, left: state.x })
  const { node } = state

  useEffect(() => {
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose()
    }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [onClose])

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const { height, width } = el.getBoundingClientRect()
    const { innerWidth, innerHeight } = window
    const top = state.y + height > innerHeight - 8 ? state.y - height : state.y
    const left = state.x + width > innerWidth - 8 ? state.x - width : state.x
    setPos({ top: Math.max(8, top), left: Math.max(8, left) })
  }, [])

  const parentId = node.isDir ? node.id : node.id.includes('/') ? node.id.split('/').slice(0, -1).join('/') : null

  const fsPost = async (endpoint: string, body: object) => {
    const q = projectQuery(ctx)
    await fetch(`${API_BASE}/api/fs/${endpoint}${q}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    })
  }

  const handleReveal = async () => {
    onClose()
    await fsPost('reveal', { path: node.id })
  }

  const handleCut = () => {
    ctx.clipboard.current = { path: node.id, op: 'cut' }
    onClose()
  }

  const handleCopy = () => {
    ctx.clipboard.current = { path: node.id, op: 'copy' }
    onClose()
  }

  const handlePaste = async () => {
    onClose()
    const cb = ctx.clipboard.current
    if (!cb) return
    const destDir = node.isDir ? node.id : (node.id.includes('/') ? node.id.split('/').slice(0, -1).join('/') : null)
    const fileName = cb.path.split('/').pop() ?? cb.path
    const newPath = destDir ? `${destDir}/${fileName}` : fileName

    if (cb.op === 'cut') {
      await fsPost('rename', { old_path: cb.path, new_path: newPath })
      ctx.clipboard.current = null
    } else {
      // For copy: read + write (files only for now)
      const q = ctx.projectId ? `?project=${encodeURIComponent(ctx.projectId)}` : ''
      const res = await fetch(`${API_BASE}/api/fs/read?path=${encodeURIComponent(cb.path)}${q.replace('?','&')}`)
      if (res.ok) {
        const { content } = await res.json() as { content: string }
        await fsPost('write', { path: newPath, content })
      }
    }
    await ctx.reloadDir(destDir)
  }

  const handleCopyPath = () => {
    onClose()
    const root = ctx.projectRoot ?? ''
    const abs = root ? `${root}/${node.id}` : node.id
    void navigator.clipboard.writeText(abs)
  }

  const handleCopyRelativePath = () => {
    onClose()
    void navigator.clipboard.writeText(node.id)
  }

  const handleDelete = async () => {
    onClose()
    if (!window.confirm(`Delete "${node.name}"?`)) return
    await fsPost('delete', { path: node.id })
    ctx.setData((prev) => removeById(prev, node.id))
  }

  const isMac = navigator.platform.toUpperCase().includes('MAC')
  const revealLabel = isMac ? 'Reveal in Finder' : 'Reveal in Explorer'

  const sep = <div className="my-1 border-t border-white/5" />

  const item = (icon: React.ReactNode, label: string, onClick: () => void, danger = false) => (
    <button
      className={`w-full flex items-center gap-2.5 px-3 py-1.5 text-left transition-colors hover:bg-white/5 ${danger ? 'text-red-400/70 hover:text-red-400' : 'text-white/60 hover:text-white/90'}`}
      onClick={onClick}
    >
      {icon}{label}
    </button>
  )

  return createPortal(
    <div
      ref={ref}
      className="fixed z-[200] min-w-[200px] rounded-lg overflow-hidden shadow-xl border border-white/10 bg-[#111] text-xs py-1"
      style={{ top: pos.top, left: pos.left }}
    >
      {item(<FilePlus className="w-3.5 h-3.5 shrink-0" />, 'New File…', () => { onClose(); onStartNewFile(parentId) })}
      {item(<FolderPlus className="w-3.5 h-3.5 shrink-0" />, 'New Folder…', () => { onClose(); onStartNewFolder(parentId) })}
      {sep}
      {item(<FolderOpen className="w-3.5 h-3.5 shrink-0" />, revealLabel, () => { void handleReveal() })}
      {sep}
      {item(<Scissors className="w-3.5 h-3.5 shrink-0" />, 'Cut', handleCut)}
      {item(<Copy className="w-3.5 h-3.5 shrink-0" />, 'Copy', handleCopy)}
      {item(<Clipboard className="w-3.5 h-3.5 shrink-0" />, 'Paste', () => { void handlePaste() })}
      {sep}
      {item(<FileCode className="w-3.5 h-3.5 shrink-0" />, 'Copy Path', handleCopyPath)}
      {item(<FileText className="w-3.5 h-3.5 shrink-0" />, 'Copy Relative Path', handleCopyRelativePath)}
      {sep}
      {item(<Pencil className="w-3.5 h-3.5 shrink-0" />, 'Rename', () => { onClose(); onStartRename(node) })}
      {item(<Trash2 className="w-3.5 h-3.5 shrink-0" />, 'Delete', () => { void handleDelete() }, true)}
      {sep}
      {item(<GitCompare className="w-3.5 h-3.5 shrink-0" />, 'Select for Compare (Left)', () => { onClose(); onSelectForCompare(node, 'left') })}
      {item(<GitCompare className="w-3.5 h-3.5 shrink-0" />, 'Select for Compare (Right)', () => { onClose(); onSelectForCompare(node, 'right') })}
    </div>,
    document.body
  )
}

// ── Inline input (new file / new folder / rename) ─────────────────────────────

interface InlineInputProps {
  style: React.CSSProperties
  defaultValue: string
  onCommit: (value: string) => void
  onCancel: () => void
}

function InlineInput({ style, defaultValue, onCommit, onCancel }: InlineInputProps) {
  const [value, setValue] = useState(defaultValue)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
    inputRef.current?.select()
  }, [])

  return (
    <div style={{ ...style, height: 30 }} className="flex items-center px-2 gap-1">
      <input
        ref={inputRef}
        className="flex-1 bg-white/10 border border-white/30 rounded px-1.5 py-0.5 text-xs text-white outline-none focus:border-white/60"
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onBlur={() => value.trim() ? onCommit(value.trim()) : onCancel()}
        onKeyDown={(e) => {
          if (e.key === 'Enter' && value.trim()) { e.preventDefault(); onCommit(value.trim()) }
          if (e.key === 'Escape') onCancel()
        }}
      />
    </div>
  )
}

// ── NodeRow ───────────────────────────────────────────────────────────────────

interface NodeRowExtras {
  ctx: LoadCtx
  renamingId: string | null
  onCommitRename: (node: TreeData, newName: string) => void
  onCancelRename: () => void
  onContextMenu: (e: React.MouseEvent, node: TreeData) => void
}

function NodeRow({ node, dragHandle, style, ctx, renamingId, onCommitRename, onCancelRename, onContextMenu }: NodeRendererProps<TreeData> & NodeRowExtras) {
  const isDir = node.data.isDir
  const isLoading = ctx.loadingDirs.has(node.data.id)
  const iconUrl = getFileIconUrl(node.data.name, isDir, node.isOpen)
  const [color, setColor] = useState<string | null>(null)
  const [hovered, setHovered] = useState(false)

  useEffect(() => {
    getIconColor(iconUrl).then(setColor)
  }, [iconUrl])

  if (renamingId === node.data.id) {
    return (
      <InlineInput
        style={style ?? {}}
        defaultValue={node.data.name}
        onCommit={(name) => onCommitRename(node.data, name)}
        onCancel={onCancelRename}
      />
    )
  }

  return (
    <div
      ref={dragHandle}
      style={{
        ...style,
        height: 30,
        backgroundColor: node.isSelected
          ? color ? `color-mix(in srgb, ${color} 15%, transparent)` : 'rgba(255,255,255,0.10)'
          : hovered
            ? color ? `color-mix(in srgb, ${color} 10%, transparent)` : 'rgba(255,255,255,0.05)'
            : undefined,
      }}
      className="flex items-center gap-1 pr-2 rounded cursor-pointer select-none transition-colors group"
      onClick={() => {
        if (!isDir) { ctx.onOpenFile(node.data.id); return }
        node.isOpen ? node.close() : openDir(node.data.id, ctx)
      }}
      onContextMenu={(e) => { e.preventDefault(); e.stopPropagation(); onContextMenu(e, node.data) }}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
    >
      <span className="w-4 shrink-0 flex items-center justify-center">
        {isDir && (isLoading
          ? <Loader className="w-3.5 h-3.5 text-white/30 animate-spin" />
          : <ChevronRight
              className="w-3.5 h-3.5 text-white/25 transition-transform duration-150"
              style={{ transform: node.isOpen ? 'rotate(90deg)' : 'rotate(0)' }}
            />
        )}
      </span>
      <img src={iconUrl} alt="" className="w-4 h-4 shrink-0" draggable={false} />
      <span
        className="text-sm truncate leading-none transition-opacity duration-150"
        style={{ color: color ?? 'white', opacity: hovered ? 1 : 0.55 }}
      >
        {node.data.name}
      </span>
    </div>
  )
}

// ── Inline new-item row ───────────────────────────────────────────────────────

interface NewItemRowProps {
  parentId: string | null
  isDir: boolean
  ctx: LoadCtx
  onDone: () => void
}

function NewItemRow({ parentId, isDir, ctx, onDone }: NewItemRowProps) {
  const [value, setValue] = useState('')
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => { inputRef.current?.focus() }, [])

  const commit = async (name: string) => {
    if (!name.trim()) { onDone(); return }
    const path = parentId ? `${parentId}/${name}` : name
    const q = ctx.projectId ? `?project=${encodeURIComponent(ctx.projectId)}` : ''
    if (isDir) {
      await fetch(`${API_BASE}/api/fs/mkdir${q}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path }),
      })
      const entry: TreeData = { id: path, name, isDir: true, children: [] }
      ctx.setData((prev) => insertIntoDir(prev, parentId, entry))
    } else {
      await fetch(`${API_BASE}/api/fs/write${q}`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path, content: '' }),
      })
      const entry: TreeData = { id: path, name, isDir: false }
      ctx.setData((prev) => insertIntoDir(prev, parentId, entry))
    }
    onDone()
  }

  const indent = parentId ? (parentId.split('/').length) * 12 + 16 : 16

  return (
    <div style={{ height: 30, paddingLeft: indent }} className="flex items-center gap-1 pr-2">
      <input
        ref={inputRef}
        className="flex-1 bg-white/10 border border-white/30 rounded px-1.5 py-0.5 text-xs text-white outline-none focus:border-white/60"
        value={value}
        placeholder={isDir ? 'folder name' : 'file name'}
        onChange={(e) => setValue(e.target.value)}
        onBlur={() => { void commit(value.trim()) }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') { e.preventDefault(); void commit(value.trim()) }
          if (e.key === 'Escape') onDone()
        }}
      />
    </div>
  )
}

// ── Layout helpers ────────────────────────────────────────────────────────────

function layoutHasContent(node: import('../../types/ide').LayoutNode, contentId: string): boolean {
  if (node.type === 'panel') return node.tabs.includes(contentId)
  return node.children.some(c => layoutHasContent(c, contentId))
}

// ── Mode Mismatch Modal ───────────────────────────────────────────────────────

function SideConflictModal({ item, side, existingLeft, existingRight, onReplace, onSwapSide, onNewSession, onCancel }: {
  item: CompareItem
  side: 'left' | 'right'
  existingLeft?: CompareItem
  existingRight?: CompareItem
  onReplace: () => void
  onSwapSide: (() => void) | null
  onNewSession: () => void
  onCancel: () => void
}) {
  const oppositeSide = side === 'left' ? 'right' : 'left'
  const existingOnSide = side === 'left' ? existingLeft : existingRight
  const existingOnOpposite = side === 'left' ? existingRight : existingLeft
  const newName = item.path.split('/').pop()
  const existingName = existingOnSide?.path.split('/').pop()

  return createPortal(
    <div className="fixed inset-0 z-[200] flex items-center justify-center bg-black/30 backdrop-blur-[2px]" onMouseDown={onCancel}>
      <div className="w-[420px] bg-[#111] border border-white/10 rounded-xl shadow-2xl overflow-hidden" onMouseDown={e => e.stopPropagation()}>
        <div className="px-4 py-3 border-b border-white/8 text-xs font-medium text-white/50 uppercase tracking-wider">Compare conflict</div>
        <div className="px-4 py-4 flex flex-col gap-4">
          {/* Current state */}
          <div className="flex items-center gap-2 text-xs bg-white/[0.03] rounded-lg p-3 border border-white/5">
            <div className="flex-1 text-center">
              <div className="text-white/30 mb-1">Left</div>
              <div className="text-white/70 truncate">{existingLeft?.path.split('/').pop() ?? <span className="text-white/20">empty</span>}</div>
            </div>
            <div className="text-white/20 text-lg">↔</div>
            <div className="flex-1 text-center">
              <div className="text-white/30 mb-1">Right</div>
              <div className="text-white/70 truncate">{existingRight?.path.split('/').pop() ?? <span className="text-white/20">empty</span>}</div>
            </div>
          </div>
          <p className="text-xs text-white/60">
            <span className="text-white/80">{newName}</span> → <span className="text-white/50">{side}</span> — but <span className="text-white/80">{existingName}</span> is already there. What would you like to do?
          </p>
          <div className="flex flex-col gap-1.5">
            <button onClick={onReplace} className="w-full text-left px-3 py-2 text-xs rounded-lg bg-white/5 hover:bg-white/10 text-white/70 hover:text-white/90 border border-white/5 transition-colors">
              Replace <span className="text-white/40">{existingName}</span> with <span className="text-white/80">{newName}</span> on the {side}
            </button>
            {onSwapSide && (
              <button onClick={onSwapSide} className="w-full text-left px-3 py-2 text-xs rounded-lg bg-white/5 hover:bg-white/10 text-white/70 hover:text-white/90 border border-white/5 transition-colors">
                Place <span className="text-white/80">{newName}</span> on the {oppositeSide} instead
                {existingOnOpposite && <span className="text-white/30 ml-1">(replaces {existingOnOpposite.path.split('/').pop()})</span>}
              </button>
            )}
            <button onClick={onNewSession} className="w-full text-left px-3 py-2 text-xs rounded-lg bg-white/5 hover:bg-white/10 text-white/70 hover:text-white/90 border border-white/5 transition-colors">
              Start a new session with <span className="text-white/80">{newName}</span> on the {side}
            </button>
          </div>
        </div>
        <div className="flex justify-end px-4 py-3 border-t border-white/5">
          <button onClick={onCancel} className="px-3 py-1.5 text-xs text-white/40 hover:text-white/70 transition-colors">Cancel</button>
        </div>
      </div>
    </div>,
    document.body
  )
}

// ── FileTree ──────────────────────────────────────────────────────────────────

export const FileTree = forwardRef<FileTreeHandle, FileTreeProps>(function FileTree(
  { fetchDir, onOpenFile, height, projectId, projectRoot }, ref
) {
  const { setDiffSide, createDiffSession, diffSessions, activeDiffSessionId, addPanel } = useIDEStore()
  const [data, setData] = useState<TreeData[]>([])
  const [loading, setLoading] = useState(true)
  const [contextMenu, setContextMenu] = useState<CtxMenuState | null>(null)
  const [renamingId, setRenamingId] = useState<string | null>(null)
  const [newItem, setNewItem] = useState<{ parentId: string | null; isDir: boolean } | null>(null)
  const [sideConflict, setSideConflict] = useState<{
    item: CompareItem
    side: 'left' | 'right'
    existingLeft?: CompareItem
    existingRight?: CompareItem
  } | null>(null)
  const treeRef = useRef<TreeApi<TreeData> | null>(null)
  const clipboard = useRef<{ path: string; op: 'cut' | 'copy' } | null>(null)

  const reloadDir = useCallback(async (dirId: string | null) => {
    const entries = await fetchEntries(fetchDir, dirId)
    const withChildren = entries.map((e) => (e.isDir ? { ...e, children: [] } : e))
    if (dirId === null) {
      setData(withChildren)
    } else {
      setData((prev) => updateById(prev, dirId, (n) => ({ ...n, children: withChildren })))
    }
  }, [fetchDir])

  const ctx = useRef<LoadCtx>({
    loadedDirs: new Set(),
    loadingDirs: new Set(),
    fetchDir,
    onOpenFile,
    setData,
    treeRef,
    projectId,
    projectRoot,
    clipboard,
    reloadDir,
  })
  ctx.current.fetchDir = fetchDir
  ctx.current.onOpenFile = onOpenFile
  ctx.current.projectId = projectId
  ctx.current.projectRoot = projectRoot
  ctx.current.reloadDir = reloadDir

  useImperativeHandle(ref, () => ({
    collapseAll: () => treeRef.current?.closeAll(),
  }))

  const handleContextMenu = useCallback((e: React.MouseEvent, node: TreeData) => {
    setContextMenu({ x: e.clientX, y: e.clientY, node })
  }, [])

  const handleSelectForCompare = useCallback((node: TreeData, side: 'left' | 'right') => {
    const absPath = projectRoot ? `${projectRoot}/${node.id}` : node.id
    const item: CompareItem = { path: absPath, isDir: node.isDir, projectId, projectRoot }

    const storeState = useIDEStore.getState()
    let activeSession = storeState.diffSessions.find(s => s.id === storeState.activeDiffSessionId)

    // No active session → create one and set directly
    if (!activeSession) {
      createDiffSession()
      // createDiffSession updates state synchronously via set(), re-read
      const s2 = useIDEStore.getState()
      activeSession = s2.diffSessions.find(x => x.id === s2.activeDiffSessionId)
    }

    // If the target side is already occupied → show conflict modal
    const existing = side === 'left' ? activeSession?.left : activeSession?.right
    if (existing) {
      setSideConflict({ item, side, existingLeft: activeSession?.left, existingRight: activeSession?.right })
      return
    }

    setDiffSide(side, item)

    // Open diff panel if not already open
    const ws = storeState.workspaces.find(w => w.id === storeState.activeWorkspaceId)
    const hasDiff = ws?.layout ? layoutHasContent(ws.layout, 'diff') : false
    if (!hasDiff) addPanel('diff')
  }, [diffSessions, activeDiffSessionId, setDiffSide, createDiffSession, addPanel, projectId, projectRoot])

  const handleCommitRename = useCallback(async (node: TreeData, newName: string) => {
    setRenamingId(null)
    if (newName === node.name) return
    const dir = node.id.includes('/') ? node.id.split('/').slice(0, -1).join('/') : null
    const newPath = dir ? `${dir}/${newName}` : newName
    const q = ctx.current.projectId ? `?project=${encodeURIComponent(ctx.current.projectId)}` : ''
    await fetch(`${API_BASE}/api/fs/rename${q}`, {
      method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ old_path: node.id, new_path: newPath }),
    })
    setData((prev) => updateById(prev, node.id, (n) => ({ ...n, id: newPath, name: newName })))
  }, [])

  const NodeRowComponent = useCallback((props: NodeRendererProps<TreeData>) => (
    <NodeRow
      {...props}
      style={props.style}
      ctx={ctx.current}
      renamingId={renamingId}
      onCommitRename={handleCommitRename}
      onCancelRename={() => setRenamingId(null)}
      onContextMenu={handleContextMenu}
    />
  ), [renamingId, handleCommitRename, handleContextMenu])

  useEffect(() => {
    fetchEntries(fetchDir, null)
      .then((entries) => setData(entries.map((e) => (e.isDir ? { ...e, children: [] } : e))))
      .catch(console.error)
      .finally(() => setLoading(false))
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  if (loading) {
    return (
      <div className="flex items-center justify-center gap-2 py-8 text-white/30 text-xs">
        <Loader className="w-3 h-3 animate-spin" /> Loading…
      </div>
    )
  }

  return (
    <>
      {newItem && (
        <NewItemRow
          parentId={newItem.parentId}
          isDir={newItem.isDir}
          ctx={ctx.current}
          onDone={() => setNewItem(null)}
        />
      )}
      <Tree
        ref={treeRef}
        data={data}
        width="100%"
        height={newItem ? Math.max(0, height - 30) : height}
        indent={12}
        rowHeight={30}
        overscanCount={10}
        disableDrag
        disableDrop
        openByDefault={false}
      >
        {NodeRowComponent}
      </Tree>

      {contextMenu && (
        <TreeContextMenu
          state={contextMenu}
          ctx={ctx.current}
          onClose={() => setContextMenu(null)}
          onStartRename={(node) => setRenamingId(node.id)}
          onStartNewFile={(parentId) => setNewItem({ parentId, isDir: false })}
          onStartNewFolder={(parentId) => setNewItem({ parentId, isDir: true })}
          onSelectForCompare={(node, side) => handleSelectForCompare(node, side)}
        />
      )}
      {sideConflict && (
        <SideConflictModal
          item={sideConflict.item}
          side={sideConflict.side}
          existingLeft={sideConflict.existingLeft}
          existingRight={sideConflict.existingRight}
          onReplace={() => {
            setDiffSide(sideConflict.side, sideConflict.item)
            setSideConflict(null)
          }}
          onSwapSide={() => {
            const opposite: 'left' | 'right' = sideConflict.side === 'left' ? 'right' : 'left'
            setDiffSide(opposite, sideConflict.item)
            setSideConflict(null)
          }}
          onNewSession={() => {
            createDiffSession()
            setDiffSide(sideConflict.side, sideConflict.item)
            setSideConflict(null)
          }}
          onCancel={() => setSideConflict(null)}
        />
      )}
    </>
  )
})
