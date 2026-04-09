import { useState, useRef, useEffect, useCallback } from 'react'
import { Paperclip, Send } from 'lucide-react'
import { API_BASE } from '../../hooks/useSSE'
import type { OpenFile } from '../../hooks/useFiles'

interface ChatMessage {
  id: number
  role: 'user' | 'assistant'
  content: string
  streaming?: boolean
}

interface ChatPanelProps {
  openFiles: OpenFile[]
  activeTab?: string | null
}

let msgIdCounter = 0

export function ChatPanel({ openFiles }: ChatPanelProps) {
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [input, setInput] = useState('')
  const [contextFiles, setContextFiles] = useState<string[]>([])
  const [showFilePicker, setShowFilePicker] = useState(false)
  const [streaming, setStreaming] = useState(false)
  const listRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  // Scroll to bottom when messages change
  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight
    }
  }, [messages])

  const toggleContextFile = useCallback((path: string) => {
    setContextFiles((prev) =>
      prev.includes(path) ? prev.filter((p) => p !== path) : [...prev, path]
    )
  }, [])

  const sendMessage = useCallback(async () => {
    const text = input.trim()
    if (!text || streaming) return

    setInput('')
    setShowFilePicker(false)

    const userMsg: ChatMessage = { id: ++msgIdCounter, role: 'user', content: text }
    const assistantId = ++msgIdCounter
    const assistantMsg: ChatMessage = { id: assistantId, role: 'assistant', content: '', streaming: true }

    setMessages((prev) => [...prev, userMsg, assistantMsg])
    setStreaming(true)

    try {
      const resp = await fetch(`${API_BASE}/api/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message: text, context_files: contextFiles }),
      })

      if (!resp.ok || !resp.body) {
        throw new Error(`HTTP ${resp.status}`)
      }

      const reader = resp.body.getReader()
      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const parts = buffer.split('\n\n')
        buffer = parts.pop() ?? ''

        for (const part of parts) {
          const line = part.trim()
          if (!line.startsWith('data: ')) continue
          try {
            const event = JSON.parse(line.slice(6)) as { kind: string; payload: { token?: string; error?: string } }
            if (event.kind === 'chat_token' && event.payload.token) {
              setMessages((prev) =>
                prev.map((m) =>
                  m.id === assistantId
                    ? { ...m, content: m.content + event.payload.token }
                    : m
                )
              )
            } else if (event.kind === 'chat_done') {
              setMessages((prev) =>
                prev.map((m) => (m.id === assistantId ? { ...m, streaming: false } : m))
              )
            } else if (event.kind === 'chat_error') {
              setMessages((prev) =>
                prev.map((m) =>
                  m.id === assistantId
                    ? { ...m, content: `Error: ${event.payload.error ?? 'unknown'}`, streaming: false }
                    : m
                )
              )
            }
          } catch {
            // ignore parse errors
          }
        }
      }
    } catch (err) {
      setMessages((prev) =>
        prev.map((m) =>
          m.id === assistantId
            ? { ...m, content: `Error: ${String(err)}`, streaming: false }
            : m
        )
      )
    } finally {
      setStreaming(false)
      setMessages((prev) =>
        prev.map((m) => (m.id === assistantId ? { ...m, streaming: false } : m))
      )
    }
  }, [input, streaming, contextFiles])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLTextAreaElement>) => {
      if (e.key === 'Enter' && !e.shiftKey) {
        e.preventDefault()
        sendMessage()
      }
    },
    [sendMessage]
  )

  return (
    <div className="chat-panel">
      {/* Message list */}
      <div className="chat-messages" ref={listRef}>
        {messages.length === 0 && (
          <div className="panel-empty">Ask anything about your code</div>
        )}
        {messages.map((msg) => (
          <div
            key={msg.id}
            className={`chat-message ${msg.role === 'user' ? 'chat-message-user' : 'chat-message-assistant'}`}
          >
            <div className="chat-bubble">
              {msg.content || (msg.streaming ? '' : '')}
              {msg.streaming && <span className="chat-cursor" />}
            </div>
          </div>
        ))}
      </div>

      {/* Context files badge row */}
      {contextFiles.length > 0 && (
        <div className="chat-context-row">
          {contextFiles.map((f) => (
            <span key={f} className="chat-context-badge" onClick={() => toggleContextFile(f)}>
              {f.split('/').pop()} ×
            </span>
          ))}
        </div>
      )}

      {/* File picker overlay */}
      {showFilePicker && openFiles.length > 0 && (
        <div className="chat-filepicker">
          <div className="chat-filepicker-header">Attach context files</div>
          {openFiles.map((f) => (
            <label key={f.path} className="chat-filepicker-item">
              <input
                type="checkbox"
                checked={contextFiles.includes(f.path)}
                onChange={() => toggleContextFile(f.path)}
              />
              <span>{f.path}</span>
            </label>
          ))}
        </div>
      )}

      {/* Input row */}
      <div className="chat-input-row">
        <button
          className={`chat-attach-btn${showFilePicker ? ' active' : ''}`}
          onClick={() => setShowFilePicker((v) => !v)}
          title="Attach context files"
          disabled={openFiles.length === 0}
        >
          <Paperclip size={13} />
        </button>
        <textarea
          ref={textareaRef}
          className="chat-textarea"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Ask about your code… (Enter to send, Shift+Enter for newline)"
          rows={2}
          disabled={streaming}
        />
        <button
          className="chat-send-btn"
          onClick={sendMessage}
          disabled={!input.trim() || streaming}
          title="Send"
        >
          <Send size={13} />
        </button>
      </div>
    </div>
  )
}
