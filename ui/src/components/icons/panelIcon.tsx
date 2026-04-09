import {
  FolderOpen, Code2, TerminalSquare, MessageSquare, GitCompare,
  ScrollText, Activity, TerminalIcon,
} from 'lucide-react'
import { ClaudeIcon, GeminiIcon } from './AIIcons'
import type { PanelKind } from '../../types/ide'

const AI_ICONS: Record<string, React.ReactNode> = {
  claude: <ClaudeIcon size={13} />,
  gemini: <GeminiIcon size={13} />,
}

const KIND_ICONS: Record<PanelKind, React.ReactNode> = {
  editor:   <Code2 size={13} />,
  terminal: <TerminalSquare size={13} />,
  explorer: <FolderOpen size={13} />,
  agents:   <Activity size={13} />,
  log:      <ScrollText size={13} />,
  chat:     <MessageSquare size={13} />,
  diff:     <GitCompare size={13} />,
  other:    <TerminalIcon size={13} />,
}

/**
 * Returns the icon for a panel given its kind and optional dock icon string.
 * dockIcon format: "ai:claude", "ai:gemini", etc.
 */
export function getPanelIcon(kind: PanelKind, dockIcon?: string): React.ReactNode {
  if (dockIcon?.startsWith('ai:')) {
    const id = dockIcon.slice(3)
    if (AI_ICONS[id]) return AI_ICONS[id]
  }
  return KIND_ICONS[kind]
}
