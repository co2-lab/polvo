import { useCallback, useEffect, useRef, useState } from 'react'
import { FileTree, type FileTreeHandle } from '../filetree/FileTree'
import { API_BASE } from '../../hooks/useSSE'
import type { FileEntry } from '../../types/api'
import { useIDEStore } from '../../store/useIDEStore'
import { Pin, PinOff, ChevronsDownUp, RotateCcw } from 'lucide-react'

interface ExplorerPanelProps {
  onOpenFile?: (path: string, projectId?: string) => void
  panelId: string
  pinnedProjectId?: string
  accentColor?: string
}

export function ExplorerPanel({ onOpenFile, panelId, pinnedProjectId, accentColor }: ExplorerPanelProps) {
  const { activeProjectId, projects, pinExplorer, unpinExplorer } = useIDEStore()
  const effectiveProjectId = pinnedProjectId ?? activeProjectId
  const effectiveProject = projects.find(p => p.id === effectiveProjectId)
  const rootName = effectiveProject?.name ?? 'Explorer'

  const fetchDir = useCallback(async (path: string): Promise<FileEntry[]> => {
    let url = `${API_BASE}/api/fs/list?path=${encodeURIComponent(path)}`
    if (effectiveProjectId) {
      url += `&project=${encodeURIComponent(effectiveProjectId)}`
    }
    const res = await fetch(url)
    if (!res.ok) return []
    return res.json()
  }, [effectiveProjectId])

  const handleOpenFile = useCallback((path: string) => {
    onOpenFile?.(path, effectiveProjectId ?? undefined)
  }, [onOpenFile, effectiveProjectId])

  const bodyRef = useRef<HTMLDivElement>(null)
  const treeHandleRef = useRef<FileTreeHandle>(null)
  const [treeHeight, setTreeHeight] = useState(0)
  const [reloadKey, setReloadKey] = useState(0)

  useEffect(() => {
    const el = bodyRef.current
    if (!el) return
    const ro = new ResizeObserver(([entry]) => setTreeHeight(entry.contentRect.height))
    ro.observe(el)
    setTreeHeight(el.clientHeight)
    return () => ro.disconnect()
  }, [])

  const isPinned = pinnedProjectId != null

  const handlePinToggle = () => {
    if (isPinned) {
      unpinExplorer(panelId)
    } else if (activeProjectId) {
      pinExplorer(panelId, activeProjectId)
    }
  }

  const headerBorder = accentColor
    ? `color-mix(in srgb, ${accentColor} 20%, rgba(255,255,255,0.04))`
    : 'rgba(255,255,255,0.05)'
  const titleColor = accentColor
    ? `color-mix(in srgb, ${accentColor} 80%, white)`
    : 'rgba(255,255,255,0.40)'

  return (
    <div className="w-full h-full flex flex-col overflow-hidden">
      <div
        className="px-3 py-2 shrink-0 flex items-center justify-between"
        style={{ borderBottom: `1px solid ${headerBorder}` }}
      >
        <h3 className="text-xs font-medium uppercase tracking-wider truncate" style={{ color: titleColor }}>{rootName}</h3>
        <div className="flex items-center gap-0.5">
          <button
            onClick={() => setReloadKey(k => k + 1)}
            className="p-0.5 text-white/30 hover:text-white/80 transition-colors"
            title="Reload"
          >
            <RotateCcw className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={() => treeHandleRef.current?.collapseAll()}
            className="p-0.5 text-white/30 hover:text-white/80 transition-colors"
            title="Collapse all folders"
          >
            <ChevronsDownUp className="w-3.5 h-3.5" />
          </button>
          <button
            onClick={handlePinToggle}
            className="p-0.5 text-white/30 hover:text-white/80 transition-colors"
            title={isPinned ? 'Unpin explorer (follow active project)' : 'Pin explorer to current project'}
          >
            {isPinned
              ? <Pin className="w-3.5 h-3.5" style={{ color: accentColor ?? 'rgba(255,255,255,0.6)' }} />
              : <PinOff className="w-3.5 h-3.5" />
            }
          </button>
        </div>
      </div>
      <div ref={bodyRef} className="flex-1 min-h-0 overflow-hidden px-1 py-1">
        {treeHeight > 0 && (
          <FileTree
            ref={treeHandleRef}
            key={`${effectiveProjectId ?? 'no-project'}-${reloadKey}`}
            fetchDir={fetchDir}
            onOpenFile={handleOpenFile}
            height={treeHeight}
            projectId={effectiveProjectId ?? undefined}
            projectRoot={effectiveProject?.path}
          />
        )}
      </div>
    </div>
  )
}
