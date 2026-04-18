export type SplitDirection = 'horizontal' | 'vertical'

// Kind determines grouping behavior
export type PanelKind = 'editor' | 'terminal' | 'explorer' | 'agents' | 'log' | 'chat' | 'diff' | 'ai' | 'other'

// grouped = all items of same kind share one panel with tabs
// new-panel = each item opens in its own panel
export type KindBehavior = 'grouped' | 'new-panel'

export interface PanelNode {
  id: string
  type: 'panel'
  contentId: string   // active content
  tabs: string[]      // all open content ids in this panel
  tabProjects?: Record<string, string>   // tabId → projectId
  tabSessions?: Record<string, string>    // tabId → sessionId (for terminal tabs with unique session ids)
  tabContentIds?: Record<string, string>  // tabId → original contentId (for labels when tabId is a sessionId)
  tabTitleIndices?: Record<string, number> // tabId → permanent title index (for grouped terminal tabs)
  kind: PanelKind
  projectId?: string
  pinnedProjectId?: string
  /** Stable identity for terminal sessions — survives layout restructuring (id changes). */
  sessionId?: string
  /** Permanent title index assigned when a second panel of the same kind is opened.
   *  Once set it never changes, even if other panels of the same kind are closed. */
  titleIndex?: number
}

export interface SplitNode {
  id: string
  type: 'split'
  direction: SplitDirection
  children: LayoutNode[]
}

export type LayoutNode = PanelNode | SplitNode

export interface Workspace {
  id: string
  name: string
  layout: LayoutNode | null
}

export type DockEdge = 'top' | 'bottom' | 'left' | 'right'
export type DockAlignment = 'start' | 'center' | 'end'

export interface DockPosition {
  edge: DockEdge
  alignment: DockAlignment
}

export interface DockItem {
  id: string
  name: string
  icon: string
  type: 'tool' | 'plugin' | 'ai' | 'action'
  command?: string
  hidden?: boolean
}

export interface ThemeDef {
  id: string
  name: string
  type: 'predefined' | 'marketplace'
  colors: {
    bg: string
    surface: string
    border: string
    accent: string
    text: string
  }
}

export interface IDEProject {
  id: string
  name: string
  hidden: boolean
  path: string
  color?: string
  icon?: string
}
