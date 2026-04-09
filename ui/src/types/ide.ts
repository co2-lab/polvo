export type SplitDirection = 'horizontal' | 'vertical'

// Kind determines grouping behavior
export type PanelKind = 'editor' | 'terminal' | 'explorer' | 'agents' | 'log' | 'chat' | 'diff' | 'other'

// grouped = all items of same kind share one panel with tabs
// new-panel = each item opens in its own panel
export type KindBehavior = 'grouped' | 'new-panel'

export interface PanelNode {
  id: string
  type: 'panel'
  contentId: string   // active content
  tabs: string[]      // all open content ids in this panel
  tabProjects?: Record<string, string>  // contentId → projectId
  kind: PanelKind
  projectId?: string
  pinnedProjectId?: string
  /** Stable identity for terminal sessions — survives layout restructuring (id changes). */
  sessionId?: string
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
