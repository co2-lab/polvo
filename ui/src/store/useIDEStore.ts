import { create } from 'zustand'
import { persist, createJSONStorage } from 'zustand/middleware'
import type { Layout } from 'react-resizable-panels'
import type {
  LayoutNode,
  PanelNode,
  SplitNode,
  Workspace,
  IDEProject,
  DockItem,
  SplitDirection,
  DockPosition,
  ThemeDef,
  PanelKind,
  KindBehavior,
} from '../types/ide'

export interface CompareItem {
  path: string
  isDir: boolean
  projectId?: string
  projectRoot?: string
}

export interface DiffSession {
  id: string
  name: string
  left?: CompareItem
  right?: CompareItem
}

export interface GeneralSettings {
  language: string
  restoreLastLayout: boolean
  confirmOnClose: boolean
  autoDetectCLIs: boolean
  editorFontFamily: string
  editorFontSize: number
}

export interface Shortcut {
  id: string
  name: string
  context: 'Global' | 'Editor' | 'Terminal' | 'Panels'
  keys: string[]
}

interface IDEState {
  workspaces: Workspace[]
  activeWorkspaceId: string
  pinnedWorkspaceIds: string[]
  projects: IDEProject[]
  activeProjectId: string | null
  dockItems: DockItem[]
  draggedDockItem: string | null
  draggedPanelId: string | null
  dockPosition: DockPosition
  isSettingsOpen: boolean
  projectConfigId: string | null
  activeThemeId: string
  themes: ThemeDef[]
  isSidePanelOpen: boolean
  sidePanelPosition: 'left' | 'right'
  isDockPinned: boolean
  isDockOpen: boolean
  isDockManagerOpen: boolean
  generalSettings: GeneralSettings
  shortcuts: Shortcut[]
  panelSizes: Record<string, Layout> // groupId → { panelId: percentage }
  kindBehaviors: Record<PanelKind, KindBehavior>
  newFileIndex: number
  dirtyFiles: Set<string>
  flashPanelId: string | null
  diffSessions: DiffSession[]
  activeDiffSessionId: string | null
  /** Per-kind next title index to assign. Resets to 0 when all panels of the kind are closed. */
  kindTitleCounters: Record<string, number>

  setSettingsOpen: (isOpen: boolean) => void
  openProjectConfig: (projectId: string) => void
  closeProjectConfig: () => void
  setActiveTheme: (id: string) => void
  setDockPosition: (position: DockPosition) => void
  setDraggedDockItem: (id: string | null) => void
  setDraggedPanelId: (id: string | null) => void
  movePanel: (
    sourcePanelId: string,
    targetPanelId: string,
    position: 'top' | 'bottom' | 'left' | 'right' | 'center',
  ) => void
  movePanelToWorkspace: (sourcePanelId: string, targetWorkspaceId: string) => void
  setSidePanelOpen: (isOpen: boolean) => void
  setSidePanelPosition: (position: 'left' | 'right') => void
  setDockPinned: (isPinned: boolean) => void
  setDockOpen: (isOpen: boolean) => void
  setDockManagerOpen: (isOpen: boolean) => void
  updateGeneralSettings: (settings: Partial<GeneralSettings>) => void
  updateShortcut: (id: string, keys: string[]) => void
  toggleDockItemVisibility: (id: string) => void
  toggleProjectVisibility: (id: string) => void
  setActiveWorkspace: (id: string) => void
  addWorkspace: () => void
  renameWorkspace: (id: string, name: string) => void
  removeWorkspace: (id: string) => void
  duplicateWorkspace: (id: string) => void
  togglePinWorkspace: (id: string) => void
  addPanel: (
    contentId: string,
    targetPanelId?: string,
    position?: 'top' | 'bottom' | 'left' | 'right',
    tabProjectId?: string,
  ) => void
  openFile: (path: string, projectId?: string) => void
  switchTab: (panelId: string, contentId: string) => void
  closeTab: (panelId: string, contentId: string) => void
  setKindBehavior: (kind: PanelKind, behavior: KindBehavior) => void
  removePanel: (panelId: string) => void
  updateLayout: (workspaceId: string, layout: LayoutNode | null) => void
  setActiveProject: (id: string) => void
  setProjects: (projects: IDEProject[]) => void
  addProject: (name: string, path: string) => Promise<void>
  removeProject: (id: string) => Promise<void>
  loadProjects: () => Promise<void>
  replaceTabContent: (panelId: string, oldContentId: string, newContentId: string) => void
  markDirty: (contentId: string) => void
  markClean: (contentId: string) => void
  // Diff sessions
  createDiffSession: (name?: string) => string // returns new session id
  deleteDiffSession: (id: string) => void
  renameDiffSession: (id: string, name: string) => void
  setActiveDiffSession: (id: string) => void
  setDiffSide: (side: 'left' | 'right', item: CompareItem, sessionId?: string) => void
  clearDiffSide: (side: 'left' | 'right', sessionId?: string) => void
  pinExplorer: (panelId: string, projectId: string) => void
  unpinExplorer: (panelId: string) => void
  setPanelSizes: (groupId: string, layout: Layout) => void
  detectCLIs: () => Promise<void>
  focusPanel: (panelId: string) => void
}

export function getKind(contentId: string): PanelKind {
  if (contentId.startsWith('file:') || contentId.startsWith('newfile:') || contentId === 'editor')
    return 'editor'
  if (contentId === 'terminal') return 'terminal'
  if (contentId.startsWith('cli-')) return 'ai'
  if (contentId === 'explorer') return 'explorer'
  if (contentId === 'agents') return 'agents'
  if (contentId === 'log') return 'log'
  if (contentId === 'chat') return 'chat'
  if (contentId === 'diff') return 'diff'
  return 'other'
}

const defaultKindBehaviors: Record<PanelKind, KindBehavior> = {
  editor: 'grouped',
  terminal: 'new-panel',
  explorer: 'new-panel',
  agents: 'new-panel',
  log: 'new-panel',
  chat: 'new-panel',
  diff: 'grouped',
  ai: 'new-panel',
  other: 'new-panel',
}

const initialLayout: LayoutNode | null = null

const initialWorkspaces: Workspace[] = [{ id: 'ws-1', name: 'Workspace 1', layout: initialLayout }]

const initialDockItems: DockItem[] = [
  { id: 'explorer', name: 'Explorer', icon: 'FolderTree', type: 'tool' },
  { id: 'editor', name: 'Editor', icon: 'FileCode', type: 'tool' },
  { id: 'terminal', name: 'Terminal', icon: 'Terminal', type: 'tool' },
  { id: 'diff', name: 'Diff', icon: 'GitCompare', type: 'tool' },
  { id: 'cli-polvo', name: 'Polvo', icon: 'ai:polvo', type: 'ai', command: 'polvo' },
]

const predefinedThemes: ThemeDef[] = [
  {
    id: 'theme-midnight',
    name: 'Midnight Aura',
    type: 'predefined',
    colors: {
      bg: '#0B0F19',
      surface: '#111827',
      border: '#1F2937',
      accent: '#38BDF8',
      text: '#F3F4F6',
    },
  },
  {
    id: 'theme-tokyo',
    name: 'Tokyo Night',
    type: 'predefined',
    colors: {
      bg: '#1A1B26',
      surface: '#24283B',
      border: '#414868',
      accent: '#BB9AF7',
      text: '#C0CAF5',
    },
  },
  {
    id: 'theme-monokai',
    name: 'Monokai Vibrant',
    type: 'predefined',
    colors: {
      bg: '#2D2A2E',
      surface: '#3A3839',
      border: '#5B595C',
      accent: '#FF6188',
      text: '#FCFCFA',
    },
  },
  {
    id: 'theme-cyberpunk',
    name: 'Cyberpunk',
    type: 'predefined',
    colors: {
      bg: '#09090B',
      surface: '#18181B',
      border: '#27272A',
      accent: '#FDE047',
      text: '#FAFAFA',
    },
  },
  {
    id: 'theme-dark',
    name: 'Vercel Dark',
    type: 'predefined',
    colors: {
      bg: '#000000',
      surface: '#0A0A0A',
      border: '#222222',
      accent: '#EDEDED',
      text: '#888888',
    },
  },
  {
    id: 'theme-light',
    name: 'Clean Light',
    type: 'predefined',
    colors: {
      bg: '#F8FAFC',
      surface: '#FFFFFF',
      border: '#E2E8F0',
      accent: '#6366F1',
      text: '#0F172A',
    },
  },
]

// Result: either split a single leaf panel, or wrap a whole group of siblings together.
type PlacementResult =
  | { kind: 'split-leaf'; panelId: string; direction: SplitDirection; parentSplitId: string | null }
  | { kind: 'split-group'; groupId: string; direction: SplitDirection }

interface LeafCandidate {
  panelId: string
  // product of sibling counts on path — lower = more screen space
  weight: number
  hDepth: number
  vDepth: number
  parentSplitId: string | null
  // direct siblings in the same split (used for the "treat as one" rule)
  siblings: LayoutNode[]
  parentDirection: SplitDirection | null
}

function collectLeaves(
  node: LayoutNode,
  weight: number,
  hDepth: number,
  vDepth: number,
  parentSplitId: string | null,
  siblings: LayoutNode[],
  parentDirection: SplitDirection | null,
  acc: LeafCandidate[],
): void {
  if (node.type === 'panel') {
    acc.push({ panelId: node.id, weight, hDepth, vDepth, parentSplitId, siblings, parentDirection })
    return
  }
  const n = node.children.length
  for (const child of node.children) {
    collectLeaves(
      child,
      weight * n,
      node.direction === 'horizontal' ? hDepth + 1 : hDepth,
      node.direction === 'vertical' ? vDepth + 1 : vDepth,
      node.id,
      node.children,
      node.direction,
      acc,
    )
  }
}

function findPlacement(root: LayoutNode): PlacementResult {
  if (root.type === 'panel') {
    return { kind: 'split-leaf', panelId: root.id, direction: 'horizontal', parentSplitId: null }
  }

  const leaves: LeafCandidate[] = []
  collectLeaves(root, 1, 0, 0, null, [], null, leaves)

  const minWeight = Math.min(...leaves.map(l => l.weight))
  const candidates = leaves.filter(l => l.weight === minWeight)

  // Rule: all tied candidates in the same parent split → treat the group as one unit.
  // Wrap the parent split itself with a new split in the perpendicular direction.
  const allSameParent =
    candidates.length > 1 &&
    candidates.every(c => c.parentSplitId === candidates[0].parentSplitId) &&
    candidates[0].parentSplitId !== null

  if (allSameParent) {
    const parentDir = candidates[0].parentDirection!
    // They share space along parentDir → new panel goes perpendicular (opposite axis)
    const newDirection: SplitDirection = parentDir === 'horizontal' ? 'vertical' : 'horizontal'
    return { kind: 'split-group', groupId: candidates[0].parentSplitId!, direction: newDirection }
  }

  // Single best candidate — split it along its longest axis
  const best = candidates[candidates.length - 1]
  const direction: SplitDirection = best.hDepth > best.vDepth ? 'vertical' : 'horizontal'
  return { kind: 'split-leaf', panelId: best.panelId, direction, parentSplitId: best.parentSplitId }
}

// Wraps an existing panel node in a new split, placing newChild beside it.
// The existing panel KEEPS its original id so React doesn't remount it.
// The new SplitNode gets a fresh id (it's a new structural node).
function makeSplit(
  existing: PanelNode,
  newChild: LayoutNode,
  direction: SplitDirection,
  newChildFirst: boolean,
): SplitNode {
  return {
    id: crypto.randomUUID(),
    type: 'split',
    direction,
    children: newChildFirst ? [newChild, existing] : [existing, newChild],
  }
}

const collectSplitIds = (node: LayoutNode, ids: Set<string> = new Set()): Set<string> => {
  if (node.type === 'split') {
    ids.add(node.id)
    node.children.forEach(c => collectSplitIds(c, ids))
  }
  return ids
}

const hasContentId = (node: LayoutNode, contentId: string): boolean => {
  if (node.type === 'panel') return node.tabs.includes(contentId)
  return node.children.some(child => hasContentId(child, contentId))
}

// Find the first panel matching a predicate (receives contentId and the node itself)
const findPanel = (
  node: LayoutNode,
  pred: (contentId: string, n: PanelNode) => boolean,
): PanelNode | null => {
  if (node.type === 'panel') return pred(node.contentId, node) ? node : null
  for (const child of node.children) {
    const found = findPanel(child, pred)
    if (found) return found
  }
  return null
}

// Replace contentId of a specific panel node by id
const updatePanelContentId = (node: LayoutNode, panelId: string, contentId: string): LayoutNode => {
  if (node.type === 'panel') return node.id === panelId ? { ...node, contentId } : node
  return { ...node, children: node.children.map(c => updatePanelContentId(c, panelId, contentId)) }
}

const replaceNode = (
  node: LayoutNode,
  targetId: string,
  replacer: (node: LayoutNode) => LayoutNode,
): LayoutNode => {
  if (node.id === targetId) return replacer(node)
  if (node.type === 'split') {
    return { ...node, children: node.children.map(child => replaceNode(child, targetId, replacer)) }
  }
  return node
}

const removeNodeFromTree = (node: LayoutNode, targetId: string): LayoutNode | null => {
  if (node.id === targetId) return null
  if (node.type === 'split') {
    const newChildren = node.children
      .map(child => removeNodeFromTree(child, targetId))
      .filter((child): child is LayoutNode => child !== null)
    if (newChildren.length === 0) return null
    if (newChildren.length === 1) {
      // Return the survivor as-is, preserving its original id.
      // This prevents React from remounting components (e.g. terminals) when a
      // sibling panel is removed and the split collapses.
      return newChildren[0]
    }
    return { ...node, children: newChildren }
  }
  return node
}

/** Collect every PanelNode across all workspaces matching a predicate. */
function collectAllPanels(workspaces: { layout: LayoutNode | null }[], pred?: (p: PanelNode) => boolean): PanelNode[] {
  const result: PanelNode[] = []
  function visit(n: LayoutNode) {
    if (n.type === 'panel') { if (!pred || pred(n)) result.push(n) }
    else n.children.forEach(visit)
  }
  for (const ws of workspaces) { if (ws.layout) visit(ws.layout) }
  return result
}

/** Returns the base title for a panel's contentId given the dock items. */
function getPanelBaseTitle(contentId: string, dockItems: DockItem[]): string {
  if (contentId.startsWith('file:')) return contentId.slice(5).split('/').pop() ?? contentId.slice(5)
  if (contentId.startsWith('newfile:')) return `Untitled-${contentId.slice(8)}`
  return dockItems.find(i => i.id === contentId)?.name ?? contentId
}

/**
 * When a new panel with `baseTitle` is added and another panel with the same title already exists,
 * activate indexed title mode for that baseTitle: assign titleIndex to all panels with that title
 * that don't have one yet, and return the next index to assign to the new panel.
 */
function activateIndexedMode(
  workspaces: { id: string; layout: LayoutNode | null; name: string }[],
  baseTitle: string,
  dockItems: DockItem[],
  currentCounter: number,
): { workspaces: typeof workspaces; nextCounter: number } {
  let counter = currentCounter
  const updatedWorkspaces = workspaces.map(ws => {
    if (!ws.layout) return ws
    const newLayout = assignMissingIndices(ws.layout, baseTitle, dockItems, () => ++counter)
    return newLayout !== ws.layout ? { ...ws, layout: newLayout } : ws
  })
  return { workspaces: updatedWorkspaces, nextCounter: counter }
}

/** Walk tree and assign titleIndex to PanelNodes matching baseTitle that don't have one yet. */
function assignMissingIndices(
  node: LayoutNode,
  baseTitle: string,
  dockItems: DockItem[],
  next: () => number,
): LayoutNode {
  if (node.type === 'split') {
    const newChildren = node.children.map(c => assignMissingIndices(c, baseTitle, dockItems, next))
    const changed = newChildren.some((c, i) => c !== node.children[i])
    return changed ? { ...node, children: newChildren } : node
  }
  if (getPanelBaseTitle(node.contentId, dockItems) === baseTitle && node.titleIndex === undefined) {
    return { ...node, titleIndex: next() }
  }
  return node
}

/**
 * After a panel with `baseTitle` is removed, reconcile indexed title mode:
 * - 0 remaining → reset counter, strip titleIndex from all panels with that title
 * - 1 remaining → strip its titleIndex (back to plain title), reset counter
 * - 2+ remaining → no change
 */
function reconcileTitleIndicesAfterRemoval(
  workspaces: { id: string; layout: LayoutNode | null; name: string }[],
  baseTitle: string,
  dockItems: DockItem[],
  kindTitleCounters: Record<string, number>,
): { workspaces: typeof workspaces; kindTitleCounters: Record<string, number> } {
  const remaining = collectAllPanels(workspaces, p => getPanelBaseTitle(p.contentId, dockItems) === baseTitle)
  if (remaining.length >= 2) return { workspaces, kindTitleCounters }

  const updatedWorkspaces = workspaces.map(ws => {
    if (!ws.layout) return ws
    const newLayout = stripTitleIndices(ws.layout, baseTitle, dockItems)
    return newLayout !== ws.layout ? { ...ws, layout: newLayout } : ws
  })
  return {
    workspaces: updatedWorkspaces,
    kindTitleCounters: { ...kindTitleCounters, [baseTitle]: 0 },
  }
}

/** Walk tree and remove titleIndex from PanelNodes matching baseTitle. */
function stripTitleIndices(node: LayoutNode, baseTitle: string, dockItems: DockItem[]): LayoutNode {
  if (node.type === 'split') {
    const newChildren = node.children.map(c => stripTitleIndices(c, baseTitle, dockItems))
    const changed = newChildren.some((c, i) => c !== node.children[i])
    return changed ? { ...node, children: newChildren } : node
  }
  if (getPanelBaseTitle(node.contentId, dockItems) === baseTitle && node.titleIndex !== undefined) {
    const { titleIndex: _, tabTitleIndices: __, ...rest } = node
    return rest as PanelNode
  }
  return node
}

// Helper: update a specific prop on a PanelNode by id within a layout tree
const setPanelProp = (node: LayoutNode, panelId: string, props: Partial<PanelNode>): LayoutNode => {
  if (node.type === 'panel') {
    return node.id === panelId ? { ...node, ...props } : node
  }
  return { ...node, children: node.children.map(c => setPanelProp(c, panelId, props)) }
}

export const useIDEStore = create<IDEState>()(
  persist(
    set => ({
      workspaces: initialWorkspaces,
      activeWorkspaceId: 'ws-1',
      pinnedWorkspaceIds: [],
      projects: [],
      activeProjectId: null,
      dockItems: initialDockItems,
      draggedDockItem: null,
      draggedPanelId: null,
      dockPosition: { edge: 'bottom', alignment: 'center' },
      isSettingsOpen: false,
      projectConfigId: null,
      activeThemeId: 'theme-midnight',
      themes: predefinedThemes,
      isSidePanelOpen: false,
      sidePanelPosition: 'left',
      isDockPinned: false,
      isDockOpen: true,
      isDockManagerOpen: false,
      generalSettings: {
        language: 'en-US',
        restoreLastLayout: true,
        confirmOnClose: true,
        autoDetectCLIs: true,
        editorFontFamily: 'Fira Code, monospace',
        editorFontSize: 14,
      },
      shortcuts: [
        { id: 'cmd-p', name: 'Quick Open', context: 'Global', keys: ['Ctrl', 'P'] },
        {
          id: 'cmd-shift-f',
          name: 'Global Search',
          context: 'Global',
          keys: ['Ctrl', 'Shift', 'F'],
        },
        { id: 'cmd-s', name: 'Save File', context: 'Editor', keys: ['Ctrl', 'S'] },
        { id: 'cmd-w', name: 'Close Panel', context: 'Panels', keys: ['Ctrl', 'W'] },
        { id: 'cmd-backtick', name: 'Toggle Terminal', context: 'Terminal', keys: ['Ctrl', '`'] },
      ],
      panelSizes: {},
      kindBehaviors: defaultKindBehaviors,
      newFileIndex: 0,
      dirtyFiles: new Set<string>(),
      flashPanelId: null,
      diffSessions: [],
      activeDiffSessionId: null,
      kindTitleCounters: {},

      setSettingsOpen: isOpen => set({ isSettingsOpen: isOpen }),
      openProjectConfig: projectId => set({ projectConfigId: projectId }),
      closeProjectConfig: () => set({ projectConfigId: null }),
      setActiveTheme: id => set({ activeThemeId: id }),
      setDockPosition: position => set({ dockPosition: position }),
      setDraggedDockItem: id => set({ draggedDockItem: id }),
      setDraggedPanelId: id => set({ draggedPanelId: id }),
      movePanel: (sourcePanelId, targetPanelId, position) =>
        set(state => {
          if (sourcePanelId === targetPanelId) return state

          // Find source panel and target workspace
          let sourcePanel: PanelNode | null = null
          let sourceWsIndex = -1
          let targetWsIndex = -1

          for (let i = 0; i < state.workspaces.length; i++) {
            const ws = state.workspaces[i]
            if (!ws.layout) continue
            if (!sourcePanel) {
              const found = findPanel(ws.layout, (_, n) => n.id === sourcePanelId)
              if (found) { sourcePanel = found; sourceWsIndex = i }
            }
            if (targetWsIndex === -1) {
              const found = findPanel(ws.layout, (_, n) => n.id === targetPanelId)
              if (found) targetWsIndex = i
            }
          }

          if (!sourcePanel || sourceWsIndex === -1 || targetWsIndex === -1) {
            return state
          }

          const newWorkspaces = [...state.workspaces]

          // center = merge tabs from source into target
          if (position === 'center') {
            // For panels where contentId collides (e.g. two "terminal" panels),
            // use sessionId as the tab identifier so they are distinct tabs.
            const resolveTabId = (panel: PanelNode): string =>
              panel.tabs.length === 1 && panel.tabs[0] === panel.contentId && panel.sessionId && panel.sessionId !== panel.contentId
                ? panel.sessionId
                : panel.contentId

            const doMerge = (targetNode: PanelNode, src: PanelNode): PanelNode => {
              const srcTabId = resolveTabId(src)
              const tgtTabId = resolveTabId(targetNode)
              const tgtTabs = targetNode.tabs.map(t => t === targetNode.contentId ? tgtTabId : t)
              if (tgtTabs.includes(srcTabId)) return { ...targetNode, tabs: tgtTabs }
              // Preserve permanent titleIndex for both tabs
              const tgtTitleIndex = targetNode.tabTitleIndices?.[tgtTabId] ?? targetNode.titleIndex
              const srcTitleIndex = src.tabTitleIndices?.[srcTabId] ?? src.titleIndex
              return {
                ...targetNode,
                contentId: tgtTabId,
                sessionId: targetNode.sessionId,
                tabs: [...tgtTabs, srcTabId],
                tabSessions: {
                  ...targetNode.tabSessions,
                  [tgtTabId]: targetNode.sessionId ?? targetNode.id,
                  [srcTabId]: src.sessionId ?? src.id,
                },
                tabContentIds: {
                  ...targetNode.tabContentIds,
                  [tgtTabId]: targetNode.contentId,
                  [srcTabId]: src.contentId,
                },
                tabTitleIndices: {
                  ...targetNode.tabTitleIndices,
                  ...(tgtTitleIndex !== undefined ? { [tgtTabId]: tgtTitleIndex } : {}),
                  ...(srcTitleIndex !== undefined ? { [srcTabId]: srcTitleIndex } : {}),
                },
              }
            }

            if (sourceWsIndex === targetWsIndex) {
              const ws = newWorkspaces[sourceWsIndex]
              if (!ws.layout) return state
              const targetPanel = findPanel(ws.layout, (_, n) => n.id === targetPanelId)
              if (!targetPanel) return state
              const targetSessionId = targetPanel.sessionId ?? targetPanel.id
              const layoutAfterRemove = removeNodeFromTree(ws.layout, sourcePanelId)
              if (!layoutAfterRemove) return state
              const targetAfterRemove = findPanel(
                layoutAfterRemove,
                (_, n) => n.id === targetPanelId || (n.sessionId ?? n.id) === targetSessionId,
              )
              if (!targetAfterRemove) return state
              const layoutAfterMerge = replaceNode(layoutAfterRemove, targetAfterRemove.id, n =>
                n.type === 'panel' ? doMerge(n, sourcePanel!) : n
              )
              newWorkspaces[sourceWsIndex] = { ...ws, layout: layoutAfterMerge }
            } else {
              const tgtWs = newWorkspaces[targetWsIndex]
              if (!tgtWs.layout) return state
              newWorkspaces[targetWsIndex] = { ...tgtWs, layout: mergeInto(tgtWs.layout) }
              const srcWs = newWorkspaces[sourceWsIndex]
              newWorkspaces[sourceWsIndex] = {
                ...srcWs,
                layout: srcWs.layout ? removeNodeFromTree(srcWs.layout, sourcePanelId) : null,
              }
            }
            return { workspaces: newWorkspaces, draggedPanelId: null }
          }

          // directional: split target and insert source panel there
          const direction: SplitDirection =
            position === 'left' || position === 'right' ? 'horizontal' : 'vertical'
          const isFirst = position === 'left' || position === 'top'

          const movedPanel: PanelNode = { ...sourcePanel }

          if (sourceWsIndex === targetWsIndex) {
            const ws = newWorkspaces[sourceWsIndex]
            if (!ws.layout) return state

            // removeNodeFromTree collapses single-child splits by reassigning the
            // split's id to the surviving child — so targetPanelId may change.
            // We find the target panel BEFORE removal to capture its node, then
            // do the remove, then locate the target by its sessionId or original id.
            const targetPanel = findPanel(ws.layout, (_, n) => n.id === targetPanelId)
            if (!targetPanel) return state

            const layoutAfterRemove = removeNodeFromTree(ws.layout, sourcePanelId)
            if (!layoutAfterRemove) return state

            // After collapse the target panel may have a new id — find it by sessionId
            const targetSessionId = targetPanel.sessionId ?? targetPanel.id
            const targetAfterRemove = findPanel(
              layoutAfterRemove,
              (_, n) => n.id === targetPanelId || (n.sessionId ?? n.id) === targetSessionId,
            )
            if (!targetAfterRemove) return state

            const layoutAfterInsert = replaceNode(layoutAfterRemove, targetAfterRemove.id, targetNode =>
              makeSplit(targetNode as PanelNode, movedPanel, direction, isFirst)
            )
            newWorkspaces[sourceWsIndex] = { ...ws, layout: layoutAfterInsert }
          } else {
            // Different workspaces: insert into target first, then remove from source.
            const tgtWs = newWorkspaces[targetWsIndex]
            if (!tgtWs.layout) return state
            const layoutAfterInsert = replaceNode(tgtWs.layout, targetPanelId, targetNode =>
              makeSplit(targetNode as PanelNode, movedPanel, direction, isFirst)
            )
            newWorkspaces[targetWsIndex] = { ...tgtWs, layout: layoutAfterInsert }

            const srcWs = newWorkspaces[sourceWsIndex]
            newWorkspaces[sourceWsIndex] = {
              ...srcWs,
              layout: srcWs.layout ? removeNodeFromTree(srcWs.layout, sourcePanelId) : null,
            }
          }

          return { workspaces: newWorkspaces, draggedPanelId: null }
        }),
      movePanelToWorkspace: (sourcePanelId, targetWorkspaceId) =>
        set(state => {
          let sourcePanel: PanelNode | null = null
          let sourceWsIndex = -1
          for (let i = 0; i < state.workspaces.length; i++) {
            const ws = state.workspaces[i]
            if (!ws.layout) continue
            const found = findPanel(ws.layout, (_, n) => n.id === sourcePanelId)
            if (found) { sourcePanel = found; sourceWsIndex = i; break }
          }
          if (!sourcePanel || sourceWsIndex === -1) return state
          const targetWsIndex = state.workspaces.findIndex(w => w.id === targetWorkspaceId)
          if (targetWsIndex === -1) return state

          const newWorkspaces = [...state.workspaces]
          // Remove from source
          const srcWs = newWorkspaces[sourceWsIndex]
          newWorkspaces[sourceWsIndex] = {
            ...srcWs,
            layout: srcWs.layout ? removeNodeFromTree(srcWs.layout, sourcePanelId) : null,
          }
          // Place in target (as root or split with existing)
          const tgtWs = newWorkspaces[targetWsIndex]
          if (!tgtWs.layout) {
            newWorkspaces[targetWsIndex] = { ...tgtWs, layout: sourcePanel }
          } else {
            const placement = findPlacement(tgtWs.layout)
            let newLayout: LayoutNode
            if (placement.kind === 'split-group') {
              newLayout = replaceNode(tgtWs.layout, placement.groupId, groupNode => ({
                id: groupNode.id,
                type: 'split',
                direction: placement.direction,
                children: [
                  groupNode.type === 'split' ? { ...groupNode, id: crypto.randomUUID() } : groupNode,
                  sourcePanel!,
                ],
              }))
            } else {
              newLayout = replaceNode(tgtWs.layout, placement.panelId, targetNode =>
                makeSplit(targetNode as PanelNode, sourcePanel!, placement.direction, false)
              )
            }
            newWorkspaces[targetWsIndex] = { ...tgtWs, layout: newLayout }
          }
          return { workspaces: newWorkspaces, draggedPanelId: null }
        }),
      setSidePanelOpen: isOpen => set({ isSidePanelOpen: isOpen }),
      setSidePanelPosition: position => set({ sidePanelPosition: position }),
      setDockPinned: isPinned => set({ isDockPinned: isPinned }),
      setDockOpen: isOpen => set({ isDockOpen: isOpen }),
      setDockManagerOpen: isOpen => set({ isDockManagerOpen: isOpen }),
      updateGeneralSettings: settings =>
        set(state => ({
          generalSettings: { ...state.generalSettings, ...settings },
        })),
      updateShortcut: (id, keys) =>
        set(state => ({
          shortcuts: state.shortcuts.map(s => (s.id === id ? { ...s, keys } : s)),
        })),
      toggleDockItemVisibility: id =>
        set(state => ({
          dockItems: state.dockItems.map(item =>
            item.id === id ? { ...item, hidden: !item.hidden } : item,
          ),
        })),
      toggleProjectVisibility: id =>
        set(state => ({
          projects: state.projects.map(p => (p.id === id ? { ...p, hidden: !p.hidden } : p)),
        })),
      setActiveWorkspace: id => set({ activeWorkspaceId: id }),
      addWorkspace: () =>
        set(state => {
          const newWs: Workspace = {
            id: crypto.randomUUID(),
            name: `Workspace ${state.workspaces.length + 1}`,
            layout: null,
          }
          return { workspaces: [...state.workspaces, newWs], activeWorkspaceId: newWs.id }
        }),
      renameWorkspace: (id, name) =>
        set(state => ({
          workspaces: state.workspaces.map(ws => (ws.id === id ? { ...ws, name } : ws)),
        })),
      removeWorkspace: id =>
        set(state => {
          if (state.workspaces.length <= 1) return state
          const remaining = state.workspaces.filter(ws => ws.id !== id)
          const activeId =
            state.activeWorkspaceId === id ? remaining[0].id : state.activeWorkspaceId
          return { workspaces: remaining, activeWorkspaceId: activeId }
        }),
      togglePinWorkspace: id =>
        set(state => ({
          pinnedWorkspaceIds: state.pinnedWorkspaceIds.includes(id)
            ? state.pinnedWorkspaceIds.filter(p => p !== id)
            : [...state.pinnedWorkspaceIds, id],
        })),
      duplicateWorkspace: id =>
        set(state => {
          const src = state.workspaces.find(ws => ws.id === id)
          if (!src) return state
          const newWs: Workspace = {
            id: crypto.randomUUID(),
            name: `${src.name} (copy)`,
            layout: src.layout,
          }
          const srcIdx = state.workspaces.findIndex(ws => ws.id === id)
          const next = [...state.workspaces]
          next.splice(srcIdx + 1, 0, newWs)
          return { workspaces: next, activeWorkspaceId: newWs.id }
        }),
      addPanel: (contentIdRaw, targetPanelId, position, tabProjectId) =>
        set(state => {
          // 'editor' dock item creates a new unsaved file buffer with incrementing index
          let newFileIndex = state.newFileIndex
          let contentId = contentIdRaw
          if (contentIdRaw === 'editor') {
            newFileIndex = state.newFileIndex + 1
            contentId = `newfile:${newFileIndex}`
          }
          const wsIndex = state.workspaces.findIndex(w => w.id === state.activeWorkspaceId)
          if (wsIndex === -1) return state
          const ws = state.workspaces[wsIndex]
          const kind = getKind(contentId)
          const behavior = state.kindBehaviors[kind]

          // grouped behavior: add as tab to existing panel of same kind
          if (behavior === 'grouped' && !targetPanelId && ws.layout) {
            const existing = findPanel(ws.layout, (_, n) => n.kind === kind)
            if (existing) {
              if (existing.tabs.includes(contentId)) {
                // Already open — just switch to it
                const newLayout = replaceNode(ws.layout, existing.id, n =>
                  n.type === 'panel' ? { ...n, contentId } : n,
                )
                const newWorkspaces = [...state.workspaces]
                newWorkspaces[wsIndex] = { ...ws, layout: newLayout }
                return { workspaces: newWorkspaces }
              }
              // Add new tab
              const newLayout = replaceNode(ws.layout, existing.id, n => {
                if (n.type !== 'panel') return n
                const tabProjects = tabProjectId
                  ? { ...n.tabProjects, [contentId]: tabProjectId }
                  : n.tabProjects
                return { ...n, contentId, tabs: [...n.tabs, contentId], tabProjects }
              })
              const newWorkspaces = [...state.workspaces]
              newWorkspaces[wsIndex] = { ...ws, layout: newLayout }
              return { workspaces: newWorkspaces }
            }
          }

          // new-panel behavior (or no existing grouped panel): create a new panel
          const newPanelId = crypto.randomUUID()
          const resolvedProjectId = tabProjectId ?? state.activeProjectId ?? undefined

          // Determine if indexed title mode should activate for this baseTitle.
          // It activates when there's already at least one panel with the same base title.
          const baseTitle = getPanelBaseTitle(contentId, state.dockItems)
          const existingSameTitlePanels = collectAllPanels(state.workspaces, p =>
            getPanelBaseTitle(p.contentId, state.dockItems) === baseTitle
          )
          const indexedModeActive = existingSameTitlePanels.length > 0
          let kindTitleCounters = state.kindTitleCounters
          let newPanelTitleIndex: number | undefined

          let workspacesForLayout = [...state.workspaces]
          if (indexedModeActive) {
            const currentCounter = kindTitleCounters[baseTitle] ?? 0
            const { workspaces: indexed, nextCounter } = activateIndexedMode(workspacesForLayout, baseTitle, state.dockItems, currentCounter)
            workspacesForLayout = indexed
            newPanelTitleIndex = nextCounter + 1
            kindTitleCounters = { ...kindTitleCounters, [baseTitle]: nextCounter + 1 }
          }

          const newPanel: PanelNode = {
            id: newPanelId,
            sessionId: newPanelId,
            type: 'panel',
            contentId,
            tabs: [contentId],
            tabProjects: tabProjectId ? { [contentId]: tabProjectId } : undefined,
            kind,
            projectId: resolvedProjectId,
            titleIndex: newPanelTitleIndex,
          }
          let newLayout: LayoutNode
          const wsAfterIndex = workspacesForLayout[wsIndex]

          if (!wsAfterIndex.layout) {
            newLayout = newPanel
          } else if (targetPanelId && position) {
            newLayout = replaceNode(wsAfterIndex.layout, targetPanelId, targetNode => {
              const direction: SplitDirection =
                position === 'left' || position === 'right' ? 'horizontal' : 'vertical'
              const isFirst = position === 'left' || position === 'top'
              return makeSplit(targetNode as PanelNode, newPanel, direction, isFirst)
            })
          } else {
            const placement = findPlacement(wsAfterIndex.layout)
            if (placement.kind === 'split-group') {
              newLayout = replaceNode(wsAfterIndex.layout, placement.groupId, groupNode => ({
                // Reuse groupNode's id so its parent keeps saved sizes
                id: groupNode.id,
                type: 'split',
                direction: placement.direction,
                children: [
                  groupNode.type === 'split'
                    ? { ...groupNode, id: crypto.randomUUID() }
                    : groupNode,
                  newPanel,
                ],
              }))
            } else {
              newLayout = replaceNode(wsAfterIndex.layout, placement.panelId, targetNode =>
                makeSplit(targetNode as PanelNode, newPanel, placement.direction, false)
              )
            }
          }

          workspacesForLayout[wsIndex] = { ...wsAfterIndex, layout: newLayout }
          return { workspaces: workspacesForLayout, newFileIndex, kindTitleCounters }
        }),
      openFile: (path, projectId) => {
        useIDEStore.getState().addPanel(`file:${path}`, undefined, undefined, projectId)
      },
      createDiffSession: name => {
        const id = crypto.randomUUID()
        set(state => {
          const n = name ?? `Session ${state.diffSessions.length + 1}`
          return {
            diffSessions: [...state.diffSessions, { id, name: n, items: [] }],
            activeDiffSessionId: id,
          }
        })
        return id
      },
      deleteDiffSession: id =>
        set(state => {
          const sessions = state.diffSessions.filter(s => s.id !== id)
          const activeId =
            state.activeDiffSessionId === id
              ? (sessions[sessions.length - 1]?.id ?? null)
              : state.activeDiffSessionId
          return { diffSessions: sessions, activeDiffSessionId: activeId }
        }),
      renameDiffSession: (id, name) =>
        set(state => ({
          diffSessions: state.diffSessions.map(s => (s.id === id ? { ...s, name } : s)),
        })),
      setActiveDiffSession: id => set({ activeDiffSessionId: id }),
      setDiffSide: (side, item, sessionId) =>
        set(state => {
          const sid = sessionId ?? state.activeDiffSessionId
          if (!sid) return state
          return {
            diffSessions: state.diffSessions.map(s => (s.id === sid ? { ...s, [side]: item } : s)),
          }
        }),
      clearDiffSide: (side, sessionId) =>
        set(state => {
          const sid = sessionId ?? state.activeDiffSessionId
          if (!sid) return state
          return {
            diffSessions: state.diffSessions.map(s =>
              s.id === sid ? { ...s, [side]: undefined } : s,
            ),
          }
        }),
      markDirty: contentId =>
        set(state => {
          const next = new Set(state.dirtyFiles)
          next.add(contentId)
          return { dirtyFiles: next }
        }),
      markClean: contentId =>
        set(state => {
          const next = new Set(state.dirtyFiles)
          next.delete(contentId)
          return { dirtyFiles: next }
        }),
      replaceTabContent: (panelId, oldContentId, newContentId) =>
        set(state => {
          const wsIndex = state.workspaces.findIndex(w => w.id === state.activeWorkspaceId)
          if (wsIndex === -1) return state
          const ws = state.workspaces[wsIndex]
          if (!ws.layout) return state
          const newLayout = replaceNode(ws.layout, panelId, n => {
            if (n.type !== 'panel') return n
            const newTabs = n.tabs.map(t => (t === oldContentId ? newContentId : t))
            return {
              ...n,
              contentId: n.contentId === oldContentId ? newContentId : n.contentId,
              tabs: newTabs,
            }
          })
          const newWorkspaces = [...state.workspaces]
          newWorkspaces[wsIndex] = { ...ws, layout: newLayout }
          const dirtyFiles = new Set(state.dirtyFiles)
          dirtyFiles.delete(oldContentId)
          return { workspaces: newWorkspaces, dirtyFiles }
        }),
      switchTab: (panelId, contentId) =>
        set(state => {
          const wsIndex = state.workspaces.findIndex(w => w.id === state.activeWorkspaceId)
          if (wsIndex === -1) return state
          const ws = state.workspaces[wsIndex]
          if (!ws.layout) return state
          const newLayout = replaceNode(ws.layout, panelId, n =>
            n.type === 'panel' ? { ...n, contentId } : n,
          )
          const newWorkspaces = [...state.workspaces]
          newWorkspaces[wsIndex] = { ...ws, layout: newLayout }
          return { workspaces: newWorkspaces }
        }),
      closeTab: (panelId, contentId) =>
        set(state => {
          const wsIndex = state.workspaces.findIndex(w => w.id === state.activeWorkspaceId)
          if (wsIndex === -1) return state
          const ws = state.workspaces[wsIndex]
          if (!ws.layout) return state

          const panel = findPanel(ws.layout, (_, n) => n.id === panelId)
          if (!panel || panel.type !== 'panel') return state
          // Resolve the real contentId for title tracking (tabContentIds maps sessionId → contentId for terminal tabs)
          const resolvedContentId = panel.tabContentIds?.[contentId] ?? contentId
          const closedBaseTitle = getPanelBaseTitle(resolvedContentId, state.dockItems)

          const newTabs = panel.tabs.filter(t => t !== contentId)
          let baseWorkspaces: typeof state.workspaces
          if (newTabs.length === 0) {
            // No tabs left — remove the panel entirely
            const newLayout = removeNodeFromTree(ws.layout, panelId)
            baseWorkspaces = [...state.workspaces]
            baseWorkspaces[wsIndex] = { ...ws, layout: newLayout }
          } else {
            const rawNewContentId =
              panel.contentId === contentId
                ? (newTabs[Math.max(0, newTabs.indexOf(contentId) - 1)] ?? newTabs[0])
                : panel.contentId
            // When only one tab remains, ungroup: restore original contentId and sessionId,
            // clear the session/content/titleIndex maps that were only needed for multi-tab mode.
            let updatedPanel: PanelNode
            if (newTabs.length === 1) {
              const survivingTabId = newTabs[0]
              const restoredContentId = panel.tabContentIds?.[survivingTabId] ?? survivingTabId
              const restoredSessionId = panel.tabSessions?.[survivingTabId] ?? panel.sessionId
              const restoredTitleIndex = panel.tabTitleIndices?.[survivingTabId] ?? panel.titleIndex
              updatedPanel = {
                ...panel,
                contentId: restoredContentId,
                tabs: [restoredContentId],
                sessionId: restoredSessionId,
                tabSessions: undefined,
                tabContentIds: undefined,
                tabTitleIndices: undefined,
                titleIndex: restoredTitleIndex,
              }
            } else {
              updatedPanel = { ...panel, contentId: rawNewContentId, tabs: newTabs }
            }
            const newLayout = replaceNode(ws.layout, panelId, n =>
              n.type === 'panel' ? updatedPanel : n,
            )
            baseWorkspaces = [...state.workspaces]
            baseWorkspaces[wsIndex] = { ...ws, layout: newLayout }
          }

          // Reconcile indexed title mode for the closed tab's base title
          const { workspaces: finalWorkspaces, kindTitleCounters } =
            reconcileTitleIndicesAfterRemoval(baseWorkspaces, closedBaseTitle, state.dockItems, state.kindTitleCounters)
          return { workspaces: finalWorkspaces, kindTitleCounters }
        }),
      setKindBehavior: (kind, behavior) =>
        set(state => ({
          kindBehaviors: { ...state.kindBehaviors, [kind]: behavior },
        })),
      removePanel: panelId =>
        set(state => {
          const wsIndex = state.workspaces.findIndex(w => w.id === state.activeWorkspaceId)
          if (wsIndex === -1) return state
          const ws = state.workspaces[wsIndex]
          if (!ws.layout) return state

          // Capture base title of the panel being removed before removal
          const removedPanel = findPanel(ws.layout, (_, n) => n.id === panelId)
          const removedBaseTitle = removedPanel ? getPanelBaseTitle(removedPanel.contentId, state.dockItems) : undefined

          const newLayout = removeNodeFromTree(ws.layout, panelId)
          const newWorkspaces = [...state.workspaces]
          newWorkspaces[wsIndex] = { ...ws, layout: newLayout }
          // Prune orphan panelSizes for split nodes that no longer exist in any workspace
          const allLayouts = newWorkspaces.map(w => w.layout).filter(Boolean) as LayoutNode[]
          const liveIds = new Set<string>()
          allLayouts.forEach(l => collectSplitIds(l, liveIds))
          const panelSizes = Object.fromEntries(
            Object.entries(state.panelSizes).filter(([id]) => liveIds.has(id)),
          )
          // Reconcile indexed title mode for the removed base title
          let finalWorkspaces = newWorkspaces
          let kindTitleCounters = state.kindTitleCounters
          if (removedBaseTitle) {
            const r = reconcileTitleIndicesAfterRemoval(newWorkspaces, removedBaseTitle, state.dockItems, kindTitleCounters)
            finalWorkspaces = r.workspaces
            kindTitleCounters = r.kindTitleCounters
          }
          return { workspaces: finalWorkspaces, panelSizes, kindTitleCounters }
        }),
      updateLayout: (workspaceId, layout) =>
        set(state => ({
          workspaces: state.workspaces.map(ws => (ws.id === workspaceId ? { ...ws, layout } : ws)),
        })),
      setActiveProject: id => set({ activeProjectId: id }),
      setProjects: projects =>
        set(state => ({
          projects,
          activeProjectId: state.activeProjectId || (projects[0]?.id ?? null),
        })),
      addProject: async (name, path) => {
        const res = await fetch('/api/projects', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ name, path }),
        })
        if (!res.ok) return
        const project: IDEProject = await res.json()
        set(state => ({
          projects: state.projects.some(p => p.id === project.id)
            ? state.projects
            : [...state.projects, project],
          activeProjectId: state.activeProjectId || project.id,
        }))
      },
      removeProject: async id => {
        const res = await fetch(`/api/projects/${encodeURIComponent(id)}`, { method: 'DELETE' })
        if (!res.ok) return
        set(state => ({
          projects: state.projects.filter(p => p.id !== id),
          activeProjectId:
            state.activeProjectId === id
              ? (state.projects.find(p => p.id !== id)?.id ?? null)
              : state.activeProjectId,
        }))
      },
      loadProjects: async () => {
        try {
          const res = await fetch('/api/projects')
          if (!res.ok) return
          const fetched: IDEProject[] = await res.json()
          set(state => ({
            projects: fetched,
            activeProjectId: fetched.some(p => p.id === state.activeProjectId)
              ? state.activeProjectId
              : (fetched[0]?.id ?? null),
          }))
        } catch {
          // best-effort
        }
      },
      pinExplorer: (panelId, projectId) =>
        set(state => ({
          workspaces: state.workspaces.map(ws => ({
            ...ws,
            layout: ws.layout
              ? setPanelProp(ws.layout, panelId, { pinnedProjectId: projectId })
              : null,
          })),
        })),
      unpinExplorer: panelId =>
        set(state => ({
          workspaces: state.workspaces.map(ws => ({
            ...ws,
            layout: ws.layout
              ? setPanelProp(ws.layout, panelId, { pinnedProjectId: undefined })
              : null,
          })),
        })),
      setPanelSizes: (groupId, layout) =>
        set(state => ({
          panelSizes: { ...state.panelSizes, [groupId]: layout },
        })),
      detectCLIs: async () => {
        try {
          const res = await fetch('/api/clis')
          if (!res.ok) return
          const clis: Array<{ id: string; label: string; command: string }> = await res.json()
          set(state => {
            const existingIds = new Set(state.dockItems.map(i => i.id))
            const newItems: DockItem[] = clis
              .filter(cli => !existingIds.has(`cli-${cli.id}`))
              .map(cli => ({
                id: `cli-${cli.id}`,
                name: cli.label,
                icon: `ai:${cli.id}`,
                type: 'ai' as const,
                command: cli.command,
              }))
            return { dockItems: [...state.dockItems, ...newItems] }
          })
        } catch {
          // CLI detection is best-effort
        }
      },
      focusPanel: panelId => {
        const hasPanelId = (node: LayoutNode): boolean =>
          node.type === 'panel' ? node.id === panelId : node.children.some(hasPanelId)
        const state = useIDEStore.getState()
        const ws = state.workspaces.find(w => w.layout && hasPanelId(w.layout))
        if (!ws) return
        if (ws.id !== state.activeWorkspaceId) {
          set({ activeWorkspaceId: ws.id })
        }
        set({ flashPanelId: panelId })
        setTimeout(() => set({ flashPanelId: null }), 600)
      },
    }),
    {
      name: 'polvo',
      version: 9,
      storage: createJSONStorage(() => localStorage),
      migrate: (persisted: unknown, version: number) => {
        const state = persisted as Record<string, unknown>
        if (version < 10) {
          // Ensure cli-polvo appears after diff in dock items
          const dockItems = Array.isArray(state.dockItems)
            ? (state.dockItems as Array<Record<string, unknown>>)
            : []
          const withoutPolvo = dockItems.filter(i => i.id !== 'cli-polvo')
          const diffIdx = withoutPolvo.findIndex(i => i.id === 'diff')
          const polvoItem = dockItems.find(i => i.id === 'cli-polvo') ?? {
            id: 'cli-polvo', name: 'Polvo', icon: 'ai:polvo', type: 'ai', command: 'polvo',
          }
          if (diffIdx !== -1) {
            withoutPolvo.splice(diffIdx + 1, 0, polvoItem)
          } else {
            withoutPolvo.push(polvoItem)
          }
          return { ...state, dockItems: withoutPolvo }
        }
        if (version < 8) {
          // Remove agents, chat, log from dock items
          const removedIds = new Set(['agents', 'chat', 'log'])
          const dockItems = Array.isArray(state.dockItems)
            ? (state.dockItems as Array<Record<string, unknown>>).filter(
                i => !removedIds.has(i.id as string),
              )
            : state.dockItems
          return { ...state, dockItems }
        }
        if (version < 7) {
          // Fix editorFontFamily default from JetBrains Mono to Fira Code
          const generalSettings = (state.generalSettings as Record<string, unknown>) ?? {}
          if (generalSettings.editorFontFamily === 'JetBrains Mono, monospace') {
            generalSettings.editorFontFamily = 'Fira Code, monospace'
          }
          return { ...state, generalSettings }
        }
        if (version < 6) {
          // Remove old CLI dock items (type 'plugin') so they get re-detected with correct icons
          const dockItems = Array.isArray(state.dockItems)
            ? (state.dockItems as Array<Record<string, unknown>>).filter(i => i.type !== 'plugin')
            : state.dockItems
          return { ...state, dockItems }
        }
        if (version < 3) {
          const migrateLayout = (node: unknown): unknown => {
            if (!node || typeof node !== 'object') return node
            const n = node as Record<string, unknown>
            if (n.type === 'panel') {
              return {
                ...n,
                tabs: Array.isArray(n.tabs) ? n.tabs : [n.contentId],
                kind: getKind(n.contentId as string),
              }
            }
            if (n.type === 'split' && Array.isArray(n.children)) {
              return { ...n, children: n.children.map(migrateLayout) }
            }
            return n
          }
          const workspaces = Array.isArray(state.workspaces)
            ? state.workspaces.map((ws: unknown) => {
                const w = ws as Record<string, unknown>
                return { ...w, layout: w.layout ? migrateLayout(w.layout) : null }
              })
            : state.workspaces
          return { ...state, workspaces }
        }
        if (version < 4) {
          const { compareItems: _, ...rest } = state
          return { ...rest, diffSessions: [], activeDiffSessionId: null }
        }
        if (version < 5) {
          // Migrate sessions from items[] to left/right model
          const sessions = Array.isArray(state.diffSessions)
            ? (state.diffSessions as Array<Record<string, unknown>>).map(s => ({
                id: s.id,
                name: s.name,
                left: undefined,
                right: undefined,
              }))
            : []
          return { ...state, diffSessions: sessions, activeDiffSessionId: null }
        }
        return state
      },
      // After rehydration, ensure all default dock items are present.
      // This makes new default items appear even for existing users without
      // requiring a version bump + migration every time.
      onRehydrateStorage: () => (state) => {
        if (!state) return
        const existingIds = new Set(state.dockItems.map(i => i.id))
        const missing = initialDockItems.filter(i => !existingIds.has(i.id))
        const merged = [...state.dockItems]
        // Insert missing items in their initialDockItems order relative to existing items.
        for (const item of missing) {
          const defaultIdx = initialDockItems.findIndex(d => d.id === item.id)
          const insertBefore = merged.findIndex(m => {
            const mIdx = initialDockItems.findIndex(d => d.id === m.id)
            return mIdx > defaultIdx
          })
          if (insertBefore === -1) {
            merged.push(item)
          } else {
            merged.splice(insertBefore, 0, item)
          }
        }
        // Reorder existing items to match initialDockItems order when they diverge.
        // Only reorder items that exist in initialDockItems; user-added items stay in place.
        const knownIds = new Set(initialDockItems.map(d => d.id))
        const knownInMerged = merged.filter(i => knownIds.has(i.id))
        const expectedOrder = initialDockItems.filter(d => knownInMerged.some(i => i.id === d.id))
        const isCorrectOrder = knownInMerged.every((item, idx) => item.id === expectedOrder[idx]?.id)
        if (!isCorrectOrder) {
          let expectedIdx = 0
          for (let i = 0; i < merged.length; i++) {
            if (knownIds.has(merged[i].id)) {
              merged[i] = { ...merged[i], ...expectedOrder[expectedIdx] }
              expectedIdx++
            }
          }
        }
        state.dockItems = merged
      },
      // Only persist layout-related state; transient UI state is excluded.
      // projects and activeProjectId are NOT persisted — they come from the backend.
      partialize: state => ({
        workspaces: state.workspaces,
        activeWorkspaceId: state.activeWorkspaceId,
        pinnedWorkspaceIds: state.pinnedWorkspaceIds,
        panelSizes: state.panelSizes,
        dockItems: state.dockItems,
        dockPosition: state.dockPosition,
        isDockPinned: state.isDockPinned,
        isSidePanelOpen: state.isSidePanelOpen,
        sidePanelPosition: state.sidePanelPosition,
        activeThemeId: state.activeThemeId,
        generalSettings: state.generalSettings,
        shortcuts: state.shortcuts,
        kindBehaviors: state.kindBehaviors,
      }),
    },
  ),
)
