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

  setSettingsOpen: (isOpen: boolean) => void
  openProjectConfig: (projectId: string) => void
  closeProjectConfig: () => void
  setActiveTheme: (id: string) => void
  setDockPosition: (position: DockPosition) => void
  setDraggedDockItem: (id: string | null) => void
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
  if (contentId === 'terminal' || contentId.startsWith('cli-')) return 'terminal'
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
      const survivor = newChildren[0]
      // Restore the split's id on the survivor so the parent Group keeps its saved sizes.
      // Preserve sessionId so terminal sessions survive the id change.
      if (survivor.type === 'panel')
        return { ...survivor, id: node.id, sessionId: survivor.sessionId ?? survivor.id }
      return { ...survivor, id: node.id }
    }
    return { ...node, children: newChildren }
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

      setSettingsOpen: isOpen => set({ isSettingsOpen: isOpen }),
      openProjectConfig: projectId => set({ projectConfigId: projectId }),
      closeProjectConfig: () => set({ projectConfigId: null }),
      setActiveTheme: id => set({ activeThemeId: id }),
      setDockPosition: position => set({ dockPosition: position }),
      setDraggedDockItem: id => set({ draggedDockItem: id }),
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
          const newPanel: PanelNode = {
            id: newPanelId,
            sessionId: newPanelId,
            type: 'panel',
            contentId,
            tabs: [contentId],
            tabProjects: tabProjectId ? { [contentId]: tabProjectId } : undefined,
            kind,
            projectId: resolvedProjectId,
          }
          let newLayout: LayoutNode

          if (!ws.layout) {
            newLayout = newPanel
          } else if (targetPanelId && position) {
            newLayout = replaceNode(ws.layout, targetPanelId, targetNode => {
              const direction: SplitDirection =
                position === 'left' || position === 'right' ? 'horizontal' : 'vertical'
              const isFirst = position === 'left' || position === 'top'
              const t = targetNode as PanelNode
              const innerPanel: PanelNode = {
                ...t,
                id: crypto.randomUUID(),
                sessionId: t.sessionId ?? t.id,
              }
              const splitNode: SplitNode = {
                id: t.id,
                type: 'split',
                direction,
                children: isFirst ? [newPanel, innerPanel] : [innerPanel, newPanel],
              }
              return splitNode
            })
          } else {
            const placement = findPlacement(ws.layout)
            if (placement.kind === 'split-group') {
              newLayout = replaceNode(ws.layout, placement.groupId, groupNode => ({
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
              newLayout = replaceNode(ws.layout, placement.panelId, targetNode => {
                const t = targetNode as PanelNode
                const innerPanel: PanelNode = {
                  ...t,
                  id: crypto.randomUUID(),
                  sessionId: t.sessionId ?? t.id,
                }
                return {
                  id: t.id,
                  type: 'split',
                  direction: placement.direction,
                  children: [innerPanel, newPanel],
                }
              })
            }
          }

          const newWorkspaces = [...state.workspaces]
          newWorkspaces[wsIndex] = { ...ws, layout: newLayout }
          return { workspaces: newWorkspaces, newFileIndex }
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

          const newTabs = panel.tabs.filter(t => t !== contentId)
          if (newTabs.length === 0) {
            // No tabs left — remove the panel entirely
            const newLayout = removeNodeFromTree(ws.layout, panelId)
            const newWorkspaces = [...state.workspaces]
            newWorkspaces[wsIndex] = { ...ws, layout: newLayout }
            return { workspaces: newWorkspaces }
          }

          const newContentId =
            panel.contentId === contentId
              ? (newTabs[Math.max(0, newTabs.indexOf(contentId) - 1)] ?? newTabs[0])
              : panel.contentId

          const newLayout = replaceNode(ws.layout, panelId, n =>
            n.type === 'panel' ? { ...n, contentId: newContentId, tabs: newTabs } : n,
          )
          const newWorkspaces = [...state.workspaces]
          newWorkspaces[wsIndex] = { ...ws, layout: newLayout }
          return { workspaces: newWorkspaces }
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
          return { workspaces: newWorkspaces, panelSizes }
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
