import { useEffect, useState } from 'react'
import { Loader, AlertCircle } from 'lucide-react'
import { EditorPane } from '../editor/EditorPane'
import { SaveAsModal } from '../editor/SaveAsModal'
import { API_BASE } from '../../hooks/useSSE'
import { useIDEStore } from '../../store/useIDEStore'

const IMAGE_EXTS = new Set(['png', 'jpg', 'jpeg', 'gif', 'webp', 'svg', 'ico', 'bmp', 'avif'])

function getExt(path: string) {
  return path.split('.').pop()?.toLowerCase() ?? ''
}

function isImage(path: string) {
  return IMAGE_EXTS.has(getExt(path))
}

interface FileEditorPanelProps {
  path: string
  panelId: string
}

export function FileEditorPanel({ path, panelId }: FileEditorPanelProps) {
  const { projects, activeProjectId, replaceTabContent, markDirty, markClean } = useIDEStore()
  const isNewFile = path.startsWith('newfile:')

  const [content, setContent] = useState<string | null>(isNewFile ? '' : null)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)
  const [showSaveAs, setShowSaveAs] = useState(false)
  // contentId matches what's stored in the layout tree (newfile:N or file:/abs/path)
  const contentId = isNewFile ? path : `file:${path}`

  const activeProject = projects.find(p => p.id === activeProjectId)
  const img = !isNewFile && isImage(path)

  useEffect(() => {
    if (isNewFile || img) return
    setContent(null)
    setError(null)
    fetch(`${API_BASE}/api/fs/read?path=${encodeURIComponent(path)}`)
      .then(async (res) => {
        if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
        const json = await res.json()
        return json.content as string
      })
      .then(setContent)
      .catch((e) => setError(e.message))
  }, [path, img, isNewFile])

  if (img) {
    return (
      <div className="w-full h-full flex items-center justify-center bg-[#111] overflow-auto p-4">
        <img
          src={`${API_BASE}/api/fs/read?raw=true&path=${encodeURIComponent(path)}`}
          alt={path}
          className="max-w-full max-h-full object-contain rounded"
          style={{ imageRendering: getExt(path) === 'ico' ? 'pixelated' : 'auto' }}
        />
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex items-center justify-center gap-2 h-full text-red-400 text-sm">
        <AlertCircle className="w-4 h-4" /> {error}
      </div>
    )
  }

  if (content === null) {
    return (
      <div className="flex items-center justify-center gap-2 h-full text-white/30 text-sm">
        <Loader className="w-4 h-4 animate-spin" /> Loading…
      </div>
    )
  }

  const handleSave = async () => {
    if (saving || content === null) return
    if (isNewFile) {
      setShowSaveAs(true)
      return
    }
    setSaving(true)
    try {
      await fetch(`${API_BASE}/api/fs/write`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path, content }),
      })
      markClean(contentId)
    } finally {
      setSaving(false)
    }
  }

  const handleSaveAs = async (savePath: string) => {
    setShowSaveAs(false)
    setSaving(true)
    try {
      const res = await fetch(`${API_BASE}/api/fs/write`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ path: savePath, content }),
      })
      if (res.ok) {
        markClean(contentId)
        replaceTabContent(panelId, path, `file:${savePath}`)
      }
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="w-full h-full flex flex-col">
      {saving && (
        <div className="absolute top-2 right-4 text-xs text-white/30 z-10">Saving…</div>
      )}
      <EditorPane
        path={isNewFile ? `Untitled-${path.slice(8)}.txt` : path}
        content={content}
        onChange={(v) => {
          markDirty(contentId)
          setContent(v)
        }}
        onSave={handleSave}
      />
      {showSaveAs && (
        <SaveAsModal
          projectRoot={activeProject?.path ?? ''}
          projectId={activeProjectId ?? undefined}
          suggestedName={isNewFile ? `Untitled-${path.slice(8)}.txt` : undefined}
          onSave={handleSaveAs}
          onCancel={() => setShowSaveAs(false)}
        />
      )}
    </div>
  )
}
