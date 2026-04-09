export type PanelType =
  | 'files' | 'editor' | 'terminal' | 'chat'
  | 'diff' | 'log' | 'agents' | 'panels'
  | 'marketplace' | 'settings' | 'cli'

export interface Panel {
  id: string
  type: PanelType
  title: string
  projectId?: string
  visible: boolean
  cliCommand?: string
  cliLabel?: string
}

export type SplitAxis = 'horizontal' | 'vertical'

export interface SplitNode {
  axis: SplitAxis
  ratio: number          // 0–1
  first: LayoutNode
  second: LayoutNode
}

export type LayoutNode =
  | { kind: 'panel'; panelId: string }
  | { kind: 'split'; split: SplitNode }

export interface Workspace {
  id: string
  name: string
  layout: LayoutNode | null
}

export interface Project {
  id: string
  name: string
  minimized: boolean
}

export interface IDEState {
  version: number
  workspaces: Workspace[]
  activeWorkspaceId: string
  panels: Record<string, Panel>
  projects: Record<string, Project>
  activeProjectId?: string
  settingsOpen: boolean
  dockPosition: 'bottom' | 'left' | 'right'
  activeThemeId: string
  persistLayout: boolean
}
