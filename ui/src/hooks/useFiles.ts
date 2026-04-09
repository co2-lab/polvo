import { useState, useCallback, useRef } from 'react'
import { API_BASE } from './useSSE'
import type { FileEntry } from '../types/api'

export interface OpenFile {
  path: string
  content: string
  dirty: boolean
}

export function useFiles() {
  const [openFiles, setOpenFiles] = useState<OpenFile[]>([])
  const [activeTab, setActiveTab] = useState<string | null>(null)
  const openFilesRef = useRef<OpenFile[]>(openFiles)

  // Keep ref in sync with state so callbacks can read current value without
  // stale closures and without triggering extra re-renders.
  const setOpenFilesTracked = useCallback((updater: (prev: OpenFile[]) => OpenFile[]) => {
    setOpenFiles((prev) => {
      const next = updater(prev)
      openFilesRef.current = next
      return next
    })
  }, [])

  const fetchDir = useCallback(async (path: string): Promise<FileEntry[]> => {
    const resp = await fetch(`${API_BASE}/api/fs/list?path=${encodeURIComponent(path)}`)
    if (!resp.ok) throw new Error(`fs/list error: ${resp.status}`)
    return resp.json()
  }, [])

  const openFile = useCallback(async (path: string) => {
    // If the file is already open, just switch to it and return.
    if (openFilesRef.current.find((f) => f.path === path)) {
      setActiveTab(path)
      return
    }

    try {
      const resp = await fetch(`${API_BASE}/api/fs/read?path=${encodeURIComponent(path)}`)
      if (!resp.ok) throw new Error(`fs/read error: ${resp.status}`)
      const data = await resp.json() as { content: string }
      setOpenFilesTracked((prev) => {
        if (prev.find((f) => f.path === path)) {
          // already loaded by a concurrent call; skip
          return prev
        }
        return [...prev, { path, content: data.content, dirty: false }]
      })
      setActiveTab(path)
    } catch (err) {
      console.error('Failed to open file', path, err)
    }
  }, [setOpenFilesTracked])

  const closeFile = useCallback((path: string) => {
    setOpenFilesTracked((prev) => {
      const idx = prev.findIndex((f) => f.path === path)
      if (idx === -1) return prev
      const next = prev.filter((f) => f.path !== path)
      setActiveTab((tab) => {
        if (tab !== path) return tab
        if (next.length === 0) return null
        const newIdx = Math.min(idx, next.length - 1)
        return next[newIdx].path
      })
      return next
    })
  }, [setOpenFilesTracked])

  const updateContent = useCallback((path: string, content: string) => {
    setOpenFilesTracked((prev) =>
      prev.map((f) => (f.path === path ? { ...f, content, dirty: true } : f))
    )
  }, [setOpenFilesTracked])

  const saveFile = useCallback(async (path: string) => {
    const file = openFilesRef.current.find((f) => f.path === path)
    if (!file) return

    const resp = await fetch(`${API_BASE}/api/fs/write`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ path, content: file.content }),
    })
    if (!resp.ok) throw new Error(`fs/write error: ${resp.status}`)

    setOpenFilesTracked((prev) =>
      prev.map((f) => (f.path === path ? { ...f, dirty: false } : f))
    )
  }, [setOpenFilesTracked])

  // reloadFile re-fetches the file content from disk and updates the open file,
  // marking it as clean (not dirty). Does nothing if the file is not open.
  const reloadFile = useCallback(async (path: string) => {
    try {
      const resp = await fetch(`${API_BASE}/api/fs/read?path=${encodeURIComponent(path)}`)
      if (!resp.ok) throw new Error(`fs/read error: ${resp.status}`)
      const data = await resp.json() as { content: string }
      setOpenFilesTracked((prev) =>
        prev.map((f) => (f.path === path ? { ...f, content: data.content, dirty: false } : f))
      )
    } catch (err) {
      console.error('Failed to reload file', path, err)
    }
  }, [setOpenFilesTracked])

  const activeFile = openFiles.find((f) => f.path === activeTab) ?? null

  return {
    openFiles,
    activeTab,
    activeFile,
    setActiveTab,
    fetchDir,
    openFile,
    closeFile,
    updateContent,
    saveFile,
    reloadFile,
  }
}
