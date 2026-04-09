// TypeScript types mirroring Go response structs from internal/dashboard

export interface FileEntry {
  name: string
  path: string
  is_dir: boolean
}

export interface AgentStatus {
  name: string
  file: string
  started_at: string
  done: boolean
  error?: string
}

export interface StatusResponse {
  project: string
  version: string
  commit_sha?: string
  build_date?: string
  cwd: string
  watching: boolean
  agent_running: boolean
  dashboard_url: string
}

export interface SnapshotPayload {
  status: StatusResponse
  agents: AgentStatus[] | null
  recent_log: string[] | null
}

export interface ReportSummary {
  id: string
  agent: string
  file: string
  timestamp: string
  decision: string
  severity: string
  summary: string
}

export interface ProviderStatus {
  name: string
  type: string
  ok: boolean
  error?: string
}

// SSE Event kinds
export type EventKind =
  | 'agent_started'
  | 'agent_done'
  | 'watch_started'
  | 'watch_stopped'
  | 'file_changed'
  | 'pr_created'
  | 'report_saved'
  | 'log'
  | 'snapshot'
  | 'chat_token'
  | 'chat_done'
  | 'chat_error'

export interface SSEEvent {
  kind: EventKind
  payload: unknown
}

export interface LogPayload {
  text: string
}

export interface FileChangedPayload {
  path: string
}
