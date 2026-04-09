import { API_BASE } from '../hooks/useSSE'

export async function checkHasDraft(path: string): Promise<boolean> {
  try {
    const resp = await fetch(`${API_BASE}/api/diff?path=${encodeURIComponent(path)}`)
    if (!resp.ok) return false
    const data = await resp.json() as { has_draft: boolean }
    return data.has_draft === true
  } catch { return false }
}
