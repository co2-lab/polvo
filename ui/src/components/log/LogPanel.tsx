import { useEffect, useRef } from 'react'

const MAX_DISPLAY = 200

interface LogPanelProps {
  lines: string[]
}

export function LogPanel({ lines }: LogPanelProps) {
  const endRef = useRef<HTMLDivElement>(null)
  const containerRef = useRef<HTMLDivElement>(null)

  // Auto-scroll to bottom when new lines arrive, unless user has scrolled up.
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    if (atBottom) {
      endRef.current?.scrollIntoView({ behavior: 'smooth' })
    }
  }, [lines])

  const visible = lines.slice(-MAX_DISPLAY)

  if (visible.length === 0) {
    return (
      <div className="log-panel" ref={containerRef}>
        <div className="panel-empty">No log output</div>
      </div>
    )
  }

  return (
    <div className="log-panel" ref={containerRef}>
      {visible.map((line, i) => (
        <div key={i} className="log-line">{line}</div>
      ))}
      <div ref={endRef} />
    </div>
  )
}
