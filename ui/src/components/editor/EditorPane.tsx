import { useEffect, useRef } from 'react'
import Editor from '@monaco-editor/react'
import { useIDEStore } from '../../store/useIDEStore'

const EXT_LANG: Record<string, string> = {
  ts: 'typescript',
  tsx: 'typescript',
  js: 'javascript',
  jsx: 'javascript',
  go: 'go',
  py: 'python',
  rs: 'rust',
  md: 'markdown',
  json: 'json',
  yaml: 'yaml',
  yml: 'yaml',
  html: 'html',
  css: 'css',
  sh: 'shell',
  bash: 'shell',
  toml: 'toml',
  sql: 'sql',
  xml: 'xml',
  txt: 'plaintext',
}

function detectLanguage(path: string): string {
  const ext = path.split('.').pop()?.toLowerCase() ?? ''
  return EXT_LANG[ext] ?? 'plaintext'
}

interface EditorPaneProps {
  path: string
  content: string
  onChange: (content: string) => void
  onSave: () => void
}

export function EditorPane({ path, content, onChange, onSave }: EditorPaneProps) {
  const onSaveRef = useRef(onSave)
  onSaveRef.current = onSave
  const { editorFontFamily, editorFontSize } = useIDEStore(s => s.generalSettings)

  const language = detectLanguage(path)

  // Ctrl+S keyboard shortcut is registered via Monaco's addCommand,
  // but we also handle it globally here as a fallback.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 's') {
        e.preventDefault()
        onSaveRef.current()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  return (
    <div className="w-full h-full">
      <Editor
        key={path}
        theme="vs-dark"
        path={path}
        language={language}
        value={content}
        height="100%"
        onChange={(v) => onChange(v ?? '')}
        onMount={(editor, monaco) => {
          editor.addCommand(
            monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS,
            () => onSaveRef.current()
          )
        }}
        options={{
          minimap: { enabled: false },
          fontSize: editorFontSize,
          fontFamily: editorFontFamily,
          padding: { top: 12 },
          scrollBeyondLastLine: false,
          renderLineHighlight: 'line',
          lineNumbers: 'on',
          wordWrap: 'off',
        }}
      />
    </div>
  )
}
