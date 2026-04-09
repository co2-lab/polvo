import { useState, useEffect, useRef, useLayoutEffect } from 'react'
import { DiffEditor } from '@monaco-editor/react'
import { X, AlertTriangle, Loader, FolderOpen, FileCode, ArrowLeftRight, Plus, Pencil, Trash2 } from 'lucide-react'
import { createPortal } from 'react-dom'
import { API_BASE } from '../../hooks/useSSE'
import { useIDEStore, type CompareItem, type DiffSession } from '../../store/useIDEStore'

// ── Helpers ───────────────────────────────────────────────────────────────────

function relPath(item: CompareItem): string {
  const root = item.projectRoot ?? ''
  return root && item.path.startsWith(root)
    ? item.path.slice(root.length).replace(/^\//, '')
    : item.path
}

async function fetchFileContent(item: CompareItem): Promise<string> {
  const rel = relPath(item)
  let url = `${API_BASE}/api/fs/read?path=${encodeURIComponent(rel)}`
  if (item.projectId) url += `&project=${encodeURIComponent(item.projectId)}`
  const res = await fetch(url)
  if (!res.ok) throw new Error(`HTTP ${res.status}`)
  return (await res.json()).content as string
}

// ── Folder diff ───────────────────────────────────────────────────────────────

interface FolderEntry { name: string; is_dir: boolean; size?: number; path: string }

async function fetchFolderEntries(rel: string, projectId?: string): Promise<FolderEntry[]> {
  let url = `${API_BASE}/api/fs/list?path=${encodeURIComponent(rel)}`
  if (projectId) url += `&project=${encodeURIComponent(projectId)}`
  const res = await fetch(url)
  if (!res.ok) return []
  const entries = await res.json() as Array<{ name: string; is_dir: boolean; size?: number }>
  return entries.map(e => ({ ...e, path: rel ? `${rel}/${e.name}` : e.name }))
}

async function fetchFolderRecursive(item: CompareItem): Promise<Map<string, FolderEntry>> {
  const base = relPath(item)
  const map = new Map<string, FolderEntry>()
  const stack = [base]
  while (stack.length) {
    const dir = stack.pop()!
    const entries = await fetchFolderEntries(dir, item.projectId)
    for (const e of entries) {
      map.set(e.path.replace(base + '/', ''), e)
      if (e.is_dir) stack.push(e.path)
    }
  }
  return map
}

type EntryStatus = 'equal' | 'different' | 'only-left' | 'only-right'

const statusColor: Record<EntryStatus, string> = { equal: 'text-white/40', different: 'text-yellow-400/80', 'only-left': 'text-red-400/70', 'only-right': 'text-green-400/70' }
const statusLabel: Record<EntryStatus, string> = { equal: '=', different: '≠', 'only-left': '←', 'only-right': '→' }

function FolderDiffView({ left, right }: { left: CompareItem; right: CompareItem }) {
  const [maps, setMaps] = useState<[Map<string, FolderEntry>, Map<string, FolderEntry>] | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    setLoading(true)
    Promise.all([fetchFolderRecursive(left), fetchFolderRecursive(right)])
      .then(([l, r]) => setMaps([l, r]))
      .finally(() => setLoading(false))
  }, [left.path, right.path])

  if (loading) return <div className="flex items-center justify-center gap-2 h-full text-white/30 text-sm"><Loader className="w-4 h-4 animate-spin" />Loading…</div>
  if (!maps) return null

  const [lMap, rMap] = maps
  const allKeys = new Set([...lMap.keys(), ...rMap.keys()])
  const rows: { name: string; isDir: boolean; status: EntryStatus; lSize?: number; rSize?: number }[] = []

  allKeys.forEach(key => {
    const l = lMap.get(key)
    const r = rMap.get(key)
    const isDir = (l ?? r)!.is_dir
    let status: EntryStatus = 'equal'
    if (!l) status = 'only-right'
    else if (!r) status = 'only-left'
    else if (!isDir && l.size !== r.size) status = 'different'
    rows.push({ name: key, isDir, status, lSize: l?.size, rSize: r?.size })
  })
  rows.sort((a, b) => (a.isDir !== b.isDir ? (a.isDir ? -1 : 1) : a.name.localeCompare(b.name)))

  const diffCount = rows.filter(r => r.status !== 'equal').length

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <div className="flex items-center gap-3 px-4 py-1.5 border-b border-white/5 shrink-0 text-xs text-white/40">
        <span>{rows.length} entries</span>
        {diffCount > 0 && <span className="text-yellow-400/70">{diffCount} differences</span>}
      </div>
      <div className="overflow-auto flex-1">
        <table className="w-full text-xs border-collapse">
          <thead>
            <tr className="border-b border-white/5 text-white/30 text-left sticky top-0 bg-[#0d0d0d]">
              <th className="px-4 py-2">Path</th>
              <th className="px-4 py-2 w-8 text-center">Status</th>
              <th className="px-4 py-2">{left.path.split('/').pop()} (left)</th>
              <th className="px-4 py-2">{right.path.split('/').pop()} (right)</th>
            </tr>
          </thead>
          <tbody>
            {rows.map(row => (
              <tr key={row.name} className="border-b border-white/[0.03] hover:bg-white/[0.02]">
                <td className="px-4 py-1.5 text-white/60">
                  <div className="flex items-center gap-1.5">
                    {row.isDir ? <FolderOpen className="w-3.5 h-3.5 shrink-0 text-yellow-400/50" /> : <FileCode className="w-3.5 h-3.5 shrink-0 text-blue-400/50" />}
                    <span className="truncate max-w-[300px]">{row.name}</span>
                  </div>
                </td>
                <td className={`px-4 py-1.5 text-center font-mono ${statusColor[row.status]}`}>{statusLabel[row.status]}</td>
                <td className="px-4 py-1.5 text-white/40">{row.lSize !== undefined ? `${(row.lSize / 1024).toFixed(1)}KB` : '—'}</td>
                <td className="px-4 py-1.5 text-white/40">{row.rSize !== undefined ? `${(row.rSize / 1024).toFixed(1)}KB` : '—'}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}

// ── File diff ─────────────────────────────────────────────────────────────────

const langMap: Record<string, string> = { ts: 'typescript', tsx: 'typescript', js: 'javascript', jsx: 'javascript', go: 'go', py: 'python', rs: 'rust', md: 'markdown', json: 'json', yaml: 'yaml', yml: 'yaml', css: 'css', html: 'html' }

function FileDiffView({ left, right }: { left: CompareItem; right: CompareItem }) {
  const [leftContent, setLeftContent] = useState<string | null>(null)
  const [rightContent, setRightContent] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const { editorFontFamily, editorFontSize } = useIDEStore(s => s.generalSettings)

  useEffect(() => {
    setLeftContent(null); setRightContent(null); setError(null)
    Promise.all([fetchFileContent(left), fetchFileContent(right)])
      .then(([l, r]) => { setLeftContent(l); setRightContent(r) })
      .catch(e => setError(String(e)))
  }, [left.path, right.path])

  if (error) return <div className="flex items-center justify-center h-full text-red-400/70 text-xs gap-1.5"><AlertTriangle className="w-4 h-4" />{error}</div>
  if (leftContent === null || rightContent === null) return <div className="flex items-center justify-center h-full text-white/30 text-xs gap-1.5"><Loader className="w-4 h-4 animate-spin" />Loading…</div>

  const ext = left.path.split('.').pop()?.toLowerCase() ?? 'txt'
  return (
    <div className="flex-1 min-h-0">
      <DiffEditor
        original={leftContent} modified={rightContent}
        language={langMap[ext] ?? 'plaintext'} theme="vs-dark" height="100%"
        options={{ readOnly: true, minimap: { enabled: false }, fontSize: editorFontSize, fontFamily: editorFontFamily, padding: { top: 8 }, scrollBeyondLastLine: false, renderSideBySide: true }}
      />
    </div>
  )
}

// ── Side slots header ─────────────────────────────────────────────────────────

function SideSlot({ label, item, onClear }: { label: 'Left' | 'Right'; item?: CompareItem; onClear: () => void }) {
  return (
    <div className="flex items-center gap-1.5 px-2 py-1 bg-white/[0.03] border border-white/8 rounded-md text-xs min-w-0 flex-1">
      <span className="text-white/25 shrink-0">{label}</span>
      {item ? (
        <>
          {item.isDir ? <FolderOpen className="w-3.5 h-3.5 text-yellow-400/60 shrink-0" /> : <FileCode className="w-3.5 h-3.5 text-blue-400/60 shrink-0" />}
          <span className="truncate text-white/70 flex-1">{item.path.split('/').pop()}</span>
          <button onClick={onClear} className="text-white/25 hover:text-white/60 transition-colors shrink-0"><X className="w-3 h-3" /></button>
        </>
      ) : (
        <span className="text-white/20 italic">empty — right-click a file</span>
      )}
    </div>
  )
}

function SlotsBar({ session }: { session: DiffSession }) {
  const { clearDiffSide } = useIDEStore()
  return (
    <div className="flex items-center gap-2 px-2 py-1.5 border-b border-white/5 shrink-0">
      <SideSlot label="Left" item={session.left} onClear={() => clearDiffSide('left')} />
      <ArrowLeftRight className="w-3.5 h-3.5 text-white/20 shrink-0" />
      <SideSlot label="Right" item={session.right} onClear={() => clearDiffSide('right')} />
    </div>
  )
}

// ── Session tab context menu ──────────────────────────────────────────────────

function SessionTabMenu({ session, x, y, onClose }: { session: DiffSession; x: number; y: number; onClose: () => void }) {
  const { renameDiffSession, deleteDiffSession } = useIDEStore()
  const ref = useRef<HTMLDivElement>(null)
  const [renaming, setRenaming] = useState(false)
  const [nameVal, setNameVal] = useState(session.name)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const handler = (e: MouseEvent) => { if (ref.current && !ref.current.contains(e.target as Node)) onClose() }
    window.addEventListener('mousedown', handler)
    return () => window.removeEventListener('mousedown', handler)
  }, [onClose])

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const { height, width } = el.getBoundingClientRect()
    el.style.top = `${Math.min(y, window.innerHeight - height - 8)}px`
    el.style.left = `${Math.min(x, window.innerWidth - width - 8)}px`
  }, [])

  useEffect(() => { if (renaming) { inputRef.current?.focus(); inputRef.current?.select() } }, [renaming])

  const commitRename = () => { if (nameVal.trim()) renameDiffSession(session.id, nameVal.trim()); onClose() }

  return createPortal(
    <div ref={ref} className="fixed z-[300] min-w-[160px] rounded-lg overflow-hidden shadow-xl border border-white/10 bg-[#111] text-xs py-1" style={{ top: y, left: x }}>
      {renaming ? (
        <div className="px-3 py-1.5">
          <input ref={inputRef} value={nameVal} onChange={e => setNameVal(e.target.value)}
            onKeyDown={e => { if (e.key === 'Enter') commitRename(); if (e.key === 'Escape') onClose() }}
            onBlur={commitRename}
            className="w-full bg-white/10 border border-white/20 rounded px-2 py-1 text-white outline-none focus:border-white/40" />
        </div>
      ) : (
        <>
          <button className="w-full flex items-center gap-2.5 px-3 py-1.5 text-white/60 hover:bg-white/5 hover:text-white/90 transition-colors" onClick={() => setRenaming(true)}>
            <Pencil className="w-3.5 h-3.5 shrink-0" />Rename
          </button>
          <button className="w-full flex items-center gap-2.5 px-3 py-1.5 text-red-400/70 hover:bg-white/5 hover:text-red-400 transition-colors" onClick={() => { deleteDiffSession(session.id); onClose() }}>
            <Trash2 className="w-3.5 h-3.5 shrink-0" />Delete session
          </button>
        </>
      )}
    </div>,
    document.body
  )
}

// ── Session tabs ──────────────────────────────────────────────────────────────

function SessionTabs() {
  const { diffSessions, activeDiffSessionId, setActiveDiffSession, createDiffSession, deleteDiffSession } = useIDEStore()
  const [ctxMenu, setCtxMenu] = useState<{ session: DiffSession; x: number; y: number } | null>(null)

  return (
    <div className="flex items-center border-b border-white/8 shrink-0 bg-black/10">
      <div className="flex items-stretch flex-1 overflow-x-auto scrollbar-none">
        {diffSessions.map(session => {
          const isActive = session.id === activeDiffSessionId
          const filled = (session.left ? 1 : 0) + (session.right ? 1 : 0)
          return (
            <div
              key={session.id}
              className={`group/tab flex items-center gap-1.5 px-3 h-8 shrink-0 cursor-pointer border-r border-white/5 text-xs select-none transition-colors ${isActive ? 'bg-white/8 text-white/80' : 'text-white/35 hover:text-white/60 hover:bg-white/[0.03]'}`}
              onClick={() => setActiveDiffSession(session.id)}
              onContextMenu={e => { e.preventDefault(); setCtxMenu({ session, x: e.clientX, y: e.clientY }) }}
            >
              <span className="truncate max-w-[120px]">{session.name}</span>
              {filled > 0 && <span className="text-[10px] text-white/25">{filled}/2</span>}
              <button
                onClick={e => { e.stopPropagation(); deleteDiffSession(session.id) }}
                className="ml-0.5 text-white/30 hover:text-white/70 transition-colors"
              >
                <X className="w-3 h-3" />
              </button>
            </div>
          )
        })}
      </div>
      <button onClick={() => createDiffSession()} className="shrink-0 px-2 h-8 flex items-center text-white/25 hover:text-white/60 hover:bg-white/5 transition-colors border-l border-white/5" title="New session">
        <Plus className="w-3.5 h-3.5" />
      </button>
      {ctxMenu && <SessionTabMenu session={ctxMenu.session} x={ctxMenu.x} y={ctxMenu.y} onClose={() => setCtxMenu(null)} />}
    </div>
  )
}

// ── DiffPanel ─────────────────────────────────────────────────────────────────

export function DiffPanel() {
  const { diffSessions, activeDiffSessionId } = useIDEStore()
  const session = diffSessions.find(s => s.id === activeDiffSessionId)

  if (diffSessions.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-white/25 text-sm gap-3">
        <ArrowLeftRight className="w-8 h-8 opacity-30" />
        <span>Right-click a file or folder in the explorer</span>
        <span className="text-xs text-white/15">Select for Compare (Left) or (Right)</span>
      </div>
    )
  }

  const { left, right } = session ?? {}
  const canCompare = !!left && !!right
  const isDir = left?.isDir ?? right?.isDir ?? false

  return (
    <div className="flex flex-col h-full overflow-hidden">
      <SessionTabs />
      {session && <SlotsBar session={session} />}

      {!canCompare ? (
        <div className="flex items-center justify-center flex-1 text-white/25 text-sm gap-2">
          <ArrowLeftRight className="w-4 h-4" />
          {!left && !right ? 'Select files or folders to compare' : !left ? 'Select a left item to compare' : 'Select a right item to compare'}
        </div>
      ) : isDir ? (
        <FolderDiffView left={left} right={right} />
      ) : (
        <div className="flex flex-col flex-1 min-h-0 overflow-hidden">
          <FileDiffView left={left} right={right} />
        </div>
      )}
    </div>
  )
}

export { checkHasDraft } from '../../lib/diffUtils'
