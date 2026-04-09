import { useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Folder, FolderOpen, ChevronRight, X, Save } from 'lucide-react'
import { API_BASE } from '../../hooks/useSSE'
import type { FileEntry } from '../../types/api'

interface DirNode {
  path: string
  name: string
  children?: DirNode[]
  loaded: boolean
  expanded: boolean
}

async function fetchDirs(path: string, projectId?: string): Promise<DirNode[]> {
  let url = `${API_BASE}/api/fs/list?path=${encodeURIComponent(path)}`
  if (projectId) url += `&project=${encodeURIComponent(projectId)}`
  const res = await fetch(url)
  if (!res.ok) return []
  const entries: FileEntry[] = await res.json()
  return entries
    .filter(e => e.is_dir)
    .map(e => ({ path: e.path, name: e.name, loaded: false, expanded: false }))
}

function DirTree({
  nodes,
  selectedPath,
  onSelect,
  onToggle,
}: {
  nodes: DirNode[]
  selectedPath: string
  onSelect: (path: string) => void
  onToggle: (path: string) => void
}) {
  return (
    <div>
      {nodes.map(node => (
        <div key={node.path}>
          <div
            className={`flex items-center gap-1.5 px-2 py-1 cursor-pointer rounded text-xs select-none ${
              selectedPath === node.path ? 'bg-white/15 text-white' : 'text-white/60 hover:bg-white/5 hover:text-white/80'
            }`}
            onClick={() => onSelect(node.path)}
            onDoubleClick={() => onToggle(node.path)}
          >
            <ChevronRight
              className={`w-3 h-3 shrink-0 transition-transform ${node.expanded ? 'rotate-90' : ''} ${node.children?.length === 0 && node.loaded ? 'opacity-0' : ''}`}
              onClick={(e) => { e.stopPropagation(); onToggle(node.path) }}
            />
            {node.expanded ? <FolderOpen className="w-3.5 h-3.5 shrink-0 text-yellow-400/70" /> : <Folder className="w-3.5 h-3.5 shrink-0 text-yellow-400/50" />}
            <span className="truncate">{node.name}</span>
          </div>
          {node.expanded && node.children && (
            <div className="pl-4">
              <DirTree nodes={node.children} selectedPath={selectedPath} onSelect={onSelect} onToggle={onToggle} />
            </div>
          )}
        </div>
      ))}
    </div>
  )
}

interface SaveAsModalProps {
  projectRoot: string
  projectId?: string
  suggestedName?: string
  onSave: (path: string) => void
  onCancel: () => void
}

export function SaveAsModal({ projectRoot, projectId, suggestedName, onSave, onCancel }: SaveAsModalProps) {
  const [filename, setFilename] = useState(suggestedName ?? 'untitled.txt')
  const [nodes, setNodes] = useState<DirNode[]>([])
  const [selectedDir, setSelectedDir] = useState(projectRoot)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    inputRef.current?.focus()
    inputRef.current?.select()
    // Load root dirs
    fetchDirs('', projectId).then(dirs => setNodes(dirs))
  }, [])

  const handleToggle = async (path: string) => {
    const toggle = (ns: DirNode[]): DirNode[] =>
      ns.map(n => {
        if (n.path === path) {
          const expanding = !n.expanded
          if (expanding && !n.loaded) {
            // Load async, then update
            fetchDirs(path, projectId).then(children => {
              setNodes(prev => updateNode(prev, path, { children, loaded: true, expanded: true }))
            })
            return { ...n, expanded: true }
          }
          return { ...n, expanded: !n.expanded }
        }
        if (n.children) return { ...n, children: toggle(n.children) }
        return n
      })
    setNodes(prev => toggle(prev))
  }

  const updateNode = (ns: DirNode[], path: string, updates: Partial<DirNode>): DirNode[] =>
    ns.map(n => {
      if (n.path === path) return { ...n, ...updates }
      if (n.children) return { ...n, children: updateNode(n.children, path, updates) }
      return n
    })

  const handleSave = () => {
    const name = filename.trim()
    if (!name) return
    const fullPath = selectedDir ? `${selectedDir}/${name}` : name
    onSave(fullPath)
  }

  const handleKeyDown = (e: React.KeyboardEvent) => {
    if (e.key === 'Enter') handleSave()
    if (e.key === 'Escape') onCancel()
  }

  return createPortal(
    <div className="fixed inset-0 z-[100] flex items-center justify-center bg-black/30 backdrop-blur-[2px]" onMouseDown={onCancel}>
      <div
        className="w-[420px] max-h-[540px] flex flex-col bg-[#111] border border-white/10 rounded-xl shadow-2xl overflow-hidden"
        onMouseDown={e => e.stopPropagation()}
        onKeyDown={handleKeyDown}
      >
        {/* Header */}
        <div className="flex items-center justify-between px-4 py-3 border-b border-white/10">
          <span className="text-sm font-medium text-white/80">Save File As</span>
          <button onClick={onCancel} className="text-white/30 hover:text-white/70 transition-colors">
            <X className="w-4 h-4" />
          </button>
        </div>

        {/* Filename input */}
        <div className="px-4 py-3 border-b border-white/5">
          <label className="text-xs text-white/40 block mb-1">File name</label>
          <input
            ref={inputRef}
            value={filename}
            onChange={e => setFilename(e.target.value)}
            className="w-full bg-white/5 border border-white/10 rounded-lg px-3 py-2 text-sm text-white/90 outline-none focus:border-white/30 placeholder:text-white/20"
            placeholder="filename.ext"
          />
        </div>

        {/* Directory tree */}
        <div className="flex-1 overflow-y-auto px-2 py-2 min-h-0">
          <div className="text-xs text-white/30 px-2 pb-1.5">Save location</div>
          {/* Root entry */}
          <div
            className={`flex items-center gap-1.5 px-2 py-1 cursor-pointer rounded text-xs select-none ${
              selectedDir === projectRoot || selectedDir === '' ? 'bg-white/15 text-white' : 'text-white/60 hover:bg-white/5 hover:text-white/80'
            }`}
            onClick={() => setSelectedDir(projectRoot)}
          >
            <FolderOpen className="w-3.5 h-3.5 shrink-0 text-yellow-400/70 ml-4" />
            <span className="truncate">{projectRoot.split('/').pop() || projectRoot} <span className="text-white/30">(root)</span></span>
          </div>
          <div className="pl-4">
            <DirTree
              nodes={nodes}
              selectedPath={selectedDir}
              onSelect={setSelectedDir}
              onToggle={handleToggle}
            />
          </div>
        </div>

        {/* Selected path hint */}
        <div className="px-4 py-2 border-t border-white/5 text-xs text-white/30 truncate">
          {selectedDir || projectRoot}/{filename}
        </div>

        {/* Actions */}
        <div className="flex items-center justify-end gap-2 px-4 py-3 border-t border-white/10">
          <button
            onClick={onCancel}
            className="px-3 py-1.5 text-xs text-white/50 hover:text-white/80 transition-colors"
          >
            Cancel
          </button>
          <button
            onClick={handleSave}
            className="flex items-center gap-1.5 px-4 py-1.5 bg-white/10 hover:bg-white/15 text-white/80 text-xs rounded-lg transition-colors border border-white/10"
          >
            <Save className="w-3.5 h-3.5" />
            Save
          </button>
        </div>
      </div>
    </div>,
    document.body
  )
}
