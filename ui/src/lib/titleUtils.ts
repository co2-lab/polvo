import type { LayoutNode, PanelNode } from '../types/ide'

type DockItemsType = { id: string; name: string }[]

/** Returns the raw base title for a content id (no index suffix). */
export function getBaseTitle(contentId: string, dockItems: DockItemsType): string {
  if (contentId.startsWith('file:')) return contentId.slice(5).split('/').pop() ?? contentId.slice(5)
  if (contentId.startsWith('newfile:')) return `Untitled-${contentId.slice(8)}`
  return dockItems.find(i => i.id === contentId)?.name ?? contentId
}

/**
 * Walk all workspaces and return a Map<key, number | null>:
 *   - null   → no permanent index, title is unique
 *   - number → permanent titleIndex stored on the PanelNode (or tabTitleIndices for grouped tabs)
 *
 * Key format:
 *   - standalone panel:  panelId
 *   - grouped tab:       panelId:tabId
 */
export function computeTitleIndices(
  workspaces: { layout: LayoutNode | null }[],
  _dockItems?: DockItemsType,
): Map<string, number | null> {
  const result = new Map<string, number | null>()

  function visitNode(node: LayoutNode) {
    if (node.type === 'split') { node.children.forEach(visitNode); return }
    const panel = node as PanelNode
    if (panel.tabs.length > 1) {
      for (const tabId of panel.tabs) {
        const idx = panel.tabTitleIndices?.[tabId] ?? null
        result.set(`${panel.id}:${tabId}`, idx)
      }
    } else {
      result.set(panel.id, panel.titleIndex ?? null)
    }
  }

  for (const ws of workspaces) {
    if (ws.layout) visitNode(ws.layout)
  }
  return result
}

/** Apply a permanent title index to a base title if present. */
export function resolveTitle(
  baseName: string,
  key: string,
  titleIndices: Map<string, number | null>,
): string {
  const idx = titleIndices.get(key)
  if (idx == null) return baseName
  return `${baseName} (${idx})`
}
