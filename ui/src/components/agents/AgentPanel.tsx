import { Cpu, CheckCircle, AlertCircle, Loader } from 'lucide-react'
import type { AgentStatus } from '../../types/api'

interface AgentPanelProps {
  agents: AgentStatus[]
}

export function AgentPanel({ agents }: AgentPanelProps) {
  if (agents.length === 0) {
    return (
      <div className="agent-panel">
        <div className="panel-empty">No agent activity</div>
      </div>
    )
  }

  return (
    <div className="agent-panel">
      {agents.map((agent, i) => (
        <AgentCard key={`${agent.name}-${agent.file}-${i}`} agent={agent} />
      ))}
    </div>
  )
}

function AgentCard({ agent }: { agent: AgentStatus }) {
  const filename = agent.file.split('/').pop() ?? agent.file
  const started = agent.started_at
    ? new Date(agent.started_at).toLocaleTimeString()
    : ''

  let icon: React.ReactNode
  let statusClass: string

  if (!agent.done) {
    icon = <Loader size={13} className="spin" />
    statusClass = 'running'
  } else if (agent.error) {
    icon = <AlertCircle size={13} />
    statusClass = 'error'
  } else {
    icon = <CheckCircle size={13} />
    statusClass = 'done'
  }

  return (
    <div className={`agent-card ${statusClass}`}>
      <div className="agent-card-header">
        <Cpu size={13} className="agent-cpu-icon" />
        <span className="agent-name">{agent.name}</span>
        <span className={`agent-status ${statusClass}`}>{icon}</span>
      </div>
      <div className="agent-file" title={agent.file}>{filename}</div>
      {started && <div className="agent-time">{started}</div>}
      {agent.error && <div className="agent-error">{agent.error}</div>}
    </div>
  )
}
