import { useEffect, useState, useRef } from 'react'
import { useIDEStore } from '../../store/useIDEStore'
import { motion, AnimatePresence } from 'framer-motion'
import { X, Save, CheckCircle, AlertCircle, Plus, Trash2, ChevronDown, ChevronRight, Folder, Globe, Cpu, Code2, Database, Lock, Zap, Layers, Box, Rocket, Star, Heart, Leaf, Flame, Cloud, Terminal, Package, Blocks, Wrench, FlaskConical } from 'lucide-react'
import * as yaml from 'js-yaml'
import { API_BASE } from '../../hooks/useSSE'
import { clsx } from 'clsx'

// ── types mirroring Go config structs ──────────────────────────────────────

interface ProviderConfig {
  type: string
  api_key?: string
  base_url?: string
  default_model?: string
}

interface DerivedConfig {
  spec?: string
  features?: string
  tests?: string
}

interface InterfaceGroupConfig {
  patterns: string[]
  provider?: string
  model?: string
  derived?: DerivedConfig
}

interface ReviewConfig {
  gates?: string[]
  max_retries?: number
  auto_merge?: boolean
}

interface GitConfig {
  branch_prefix?: string
  pr_labels?: string[]
  target_branch?: string
}

interface SettingsConfig {
  debounce_ms?: number
  report_dir?: string
  log_level?: string
  max_parallel?: number
}

interface ProjectConfigData {
  project?: { name?: string; color?: string; icon?: string }
  providers?: Record<string, ProviderConfig>
  interfaces?: Record<string, InterfaceGroupConfig>
  review?: ReviewConfig
  git?: GitConfig
  settings?: SettingsConfig
}

// ── helpers ────────────────────────────────────────────────────────────────

function SectionHeader({ label, open, onToggle }: { label: string; open: boolean; onToggle: () => void }) {
  return (
    <button
      onClick={onToggle}
      className="w-full flex items-center gap-2 py-2 text-left text-xs font-semibold text-white/40 uppercase tracking-widest hover:text-white/60 transition-colors"
    >
      {open ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
      {label}
    </button>
  )
}

function Field({ label, hint, children }: { label: string; hint?: string; children: React.ReactNode }) {
  return (
    <div className="flex items-start justify-between gap-4 py-2.5 border-b border-white/5">
      <div className="min-w-0">
        <div className="text-sm text-white/80">{label}</div>
        {hint && <div className="text-xs text-white/35 mt-0.5">{hint}</div>}
      </div>
      <div className="shrink-0">{children}</div>
    </div>
  )
}

function TextInput({ value, onChange, placeholder, mono }: { value: string; onChange: (v: string) => void; placeholder?: string; mono?: boolean }) {
  return (
    <input
      type="text"
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className={clsx(
        'bg-black/30 border border-white/10 rounded-lg px-3 py-1.5 text-sm text-white/80 outline-none w-52',
        mono && 'font-mono text-xs'
      )}
    />
  )
}

function NumberInput({ value, onChange, min, max }: { value: number; onChange: (v: number) => void; min?: number; max?: number }) {
  return (
    <input
      type="number"
      value={value}
      min={min}
      max={max}
      onChange={(e) => onChange(parseInt(e.target.value) || 0)}
      className="bg-black/30 border border-white/10 rounded-lg px-3 py-1.5 text-sm text-white/80 outline-none w-28 font-mono"
    />
  )
}

function Toggle({ checked, onChange }: { checked: boolean; onChange: (v: boolean) => void }) {
  return (
    <label className="relative inline-flex items-center cursor-pointer">
      <input type="checkbox" checked={checked} onChange={(e) => onChange(e.target.checked)} className="sr-only peer" />
      <div className="w-9 h-5 bg-white/10 rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-white/60" />
    </label>
  )
}

function SelectInput({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[] }) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      className="bg-black/30 border border-white/10 rounded-lg px-3 py-1.5 text-sm text-white/80 outline-none w-52"
    >
      {options.map((o) => (
        <option key={o.value} value={o.value}>{o.label}</option>
      ))}
    </select>
  )
}

function TagList({ values, onChange, placeholder }: { values: string[]; onChange: (v: string[]) => void; placeholder?: string }) {
  const [input, setInput] = useState('')
  const add = () => {
    const v = input.trim()
    if (v && !values.includes(v)) onChange([...values, v])
    setInput('')
  }
  return (
    <div className="flex flex-col gap-1.5 w-52">
      <div className="flex flex-wrap gap-1">
        {values.map((v) => (
          <span key={v} className="flex items-center gap-1 px-2 py-0.5 bg-white/8 border border-white/10 rounded-md text-xs text-white/70 font-mono">
            {v}
            <button onClick={() => onChange(values.filter((x) => x !== v))} className="text-white/30 hover:text-white/70">
              <X className="w-3 h-3" />
            </button>
          </span>
        ))}
      </div>
      <div className="flex gap-1">
        <input
          type="text"
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && add()}
          placeholder={placeholder ?? 'Add…'}
          className="flex-1 bg-black/30 border border-white/10 rounded-lg px-2 py-1 text-xs text-white/80 outline-none font-mono"
        />
        <button onClick={add} className="px-2 py-1 bg-white/5 hover:bg-white/10 border border-white/10 rounded-lg text-white/50 hover:text-white/80 transition-colors">
          <Plus className="w-3 h-3" />
        </button>
      </div>
    </div>
  )
}

// ── Color picker ───────────────────────────────────────────────────────────

const COLOR_PRESETS = [
  '#6366f1', '#8b5cf6', '#ec4899', '#ef4444',
  '#f97316', '#eab308', '#22c55e', '#10b981',
  '#06b6d4', '#3b82f6', '#ffffff', '#94a3b8',
]

function ColorPicker({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const inputRef = useRef<HTMLInputElement>(null)
  const active = value || ''

  return (
    <div className="flex items-center gap-2 flex-wrap">
      {COLOR_PRESETS.map((c) => (
        <button
          key={c}
          title={c}
          onClick={() => onChange(c)}
          className="w-5 h-5 rounded-full border-2 transition-transform hover:scale-110 shrink-0"
          style={{
            backgroundColor: c,
            borderColor: active === c ? 'white' : 'transparent',
          }}
        />
      ))}
      {/* Custom color via native picker */}
      <button
        title="Custom color"
        onClick={() => inputRef.current?.click()}
        className="w-5 h-5 rounded-full border-2 border-dashed border-white/30 hover:border-white/60 transition-colors shrink-0 flex items-center justify-center text-white/40 hover:text-white/70 text-[9px] font-bold"
      >
        +
      </button>
      <input
        ref={inputRef}
        type="color"
        value={active || '#6366f1'}
        onChange={(e) => onChange(e.target.value)}
        className="sr-only"
      />
      {active && (
        <div className="flex items-center gap-1.5 ml-1">
          <div className="w-4 h-4 rounded-full shrink-0 border border-white/20" style={{ backgroundColor: active }} />
          <span className="text-xs text-white/40 font-mono">{active}</span>
          <button onClick={() => onChange('')} className="text-white/25 hover:text-white/60 transition-colors">
            <X className="w-3 h-3" />
          </button>
        </div>
      )}
    </div>
  )
}

// ── Icon picker ─────────────────────────────────────────────────────────────

const ICON_OPTIONS: { name: string; el: React.ReactNode }[] = [
  { name: 'folder', el: <Folder className="w-4 h-4" /> },
  { name: 'globe', el: <Globe className="w-4 h-4" /> },
  { name: 'cpu', el: <Cpu className="w-4 h-4" /> },
  { name: 'code', el: <Code2 className="w-4 h-4" /> },
  { name: 'database', el: <Database className="w-4 h-4" /> },
  { name: 'lock', el: <Lock className="w-4 h-4" /> },
  { name: 'zap', el: <Zap className="w-4 h-4" /> },
  { name: 'layers', el: <Layers className="w-4 h-4" /> },
  { name: 'box', el: <Box className="w-4 h-4" /> },
  { name: 'rocket', el: <Rocket className="w-4 h-4" /> },
  { name: 'star', el: <Star className="w-4 h-4" /> },
  { name: 'heart', el: <Heart className="w-4 h-4" /> },
  { name: 'leaf', el: <Leaf className="w-4 h-4" /> },
  { name: 'flame', el: <Flame className="w-4 h-4" /> },
  { name: 'cloud', el: <Cloud className="w-4 h-4" /> },
  { name: 'terminal', el: <Terminal className="w-4 h-4" /> },
  { name: 'package', el: <Package className="w-4 h-4" /> },
  { name: 'blocks', el: <Blocks className="w-4 h-4" /> },
  { name: 'wrench', el: <Wrench className="w-4 h-4" /> },
  { name: 'flask', el: <FlaskConical className="w-4 h-4" /> },
]

function IconPicker({ value, color, onChange }: { value: string; color: string; onChange: (v: string) => void }) {
  return (
    <div className="flex items-center gap-1 flex-wrap">
      {ICON_OPTIONS.map(({ name, el }) => (
        <button
          key={name}
          title={name}
          onClick={() => onChange(value === name ? '' : name)}
          className="w-7 h-7 flex items-center justify-center rounded-md border transition-all hover:scale-110"
          style={{
            borderColor: value === name ? (color || 'rgba(255,255,255,0.4)') : 'transparent',
            backgroundColor: value === name
              ? color ? `color-mix(in srgb, ${color} 20%, transparent)` : 'rgba(255,255,255,0.10)'
              : 'rgba(255,255,255,0.04)',
            color: value === name ? (color || 'white') : 'rgba(255,255,255,0.4)',
          }}
        >
          {el}
        </button>
      ))}
    </div>
  )
}

// ── Provider section ───────────────────────────────────────────────────────

function ProviderSection({ providers, onChange }: {
  providers: Record<string, ProviderConfig>
  onChange: (p: Record<string, ProviderConfig>) => void
}) {
  const [open, setOpen] = useState(true)
  const [newName, setNewName] = useState('')

  const update = (name: string, patch: Partial<ProviderConfig>) =>
    onChange({ ...providers, [name]: { ...providers[name], ...patch } })

  const rename = (oldName: string, newName: string) => {
    const n = newName.trim()
    if (!n || n === oldName || providers[n]) return
    const next: Record<string, ProviderConfig> = {}
    for (const [k, v] of Object.entries(providers)) {
      next[k === oldName ? n : k] = v
    }
    onChange(next)
  }

  const remove = (name: string) => {
    const next = { ...providers }
    delete next[name]
    onChange(next)
  }

  const add = () => {
    const n = newName.trim()
    if (!n || providers[n]) return
    onChange({ ...providers, [n]: { type: 'claude' } })
    setNewName('')
  }

  const providerTypes = [
    { value: 'claude', label: 'Claude' },
    { value: 'openai', label: 'OpenAI' },
    { value: 'gemini', label: 'Gemini' },
    { value: 'ollama', label: 'Ollama' },
    { value: 'openai-compatible', label: 'OpenAI-compatible' },
  ]

  return (
    <div>
      <SectionHeader label="Providers" open={open} onToggle={() => setOpen(!open)} />
      {open && (
        <div className="space-y-4 mb-4">
          {Object.entries(providers).map(([name, cfg]) => (
            <div key={name} className="bg-black/20 border border-white/5 rounded-xl p-4">
              <div className="flex items-center justify-between mb-3">
                <input
                  defaultValue={name}
                  onBlur={(e) => rename(name, e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && e.currentTarget.blur()}
                  className="text-sm font-medium text-white/80 font-mono bg-transparent border-b border-transparent hover:border-white/20 focus:border-white/40 outline-none px-0.5 w-40 transition-colors"
                  title="Click to rename"
                />
                <button onClick={() => remove(name)} className="text-white/25 hover:text-red-400 transition-colors">
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
              <div className="space-y-0">
                <Field label="Type">
                  <SelectInput value={cfg.type} onChange={(v) => update(name, { type: v })} options={providerTypes} />
                </Field>
                <Field label="API Key" hint="Supports ${ENV_VAR} syntax">
                  <TextInput mono value={cfg.api_key ?? ''} onChange={(v) => update(name, { api_key: v })} placeholder="${ANTHROPIC_API_KEY}" />
                </Field>
                <Field label="Default Model">
                  <TextInput mono value={cfg.default_model ?? ''} onChange={(v) => update(name, { default_model: v })} placeholder="claude-sonnet-4-5" />
                </Field>
                {(cfg.type === 'ollama' || cfg.type === 'openai-compatible') && (
                  <Field label="Base URL">
                    <TextInput mono value={cfg.base_url ?? ''} onChange={(v) => update(name, { base_url: v })} placeholder="http://localhost:11434" />
                  </Field>
                )}
              </div>
            </div>
          ))}
          <div className="flex gap-2">
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && add()}
              placeholder="Provider name…"
              className="flex-1 bg-black/30 border border-white/10 rounded-lg px-3 py-1.5 text-sm text-white/70 outline-none font-mono"
            />
            <button onClick={add} className="flex items-center gap-1.5 px-3 py-1.5 bg-white/5 hover:bg-white/10 border border-white/10 rounded-lg text-xs text-white/60 hover:text-white/90 transition-colors">
              <Plus className="w-3.5 h-3.5" /> Add
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// ── Interfaces section ─────────────────────────────────────────────────────

function InterfacesSection({ interfaces, onChange }: {
  interfaces: Record<string, InterfaceGroupConfig>
  onChange: (i: Record<string, InterfaceGroupConfig>) => void
}) {
  const [open, setOpen] = useState(true)
  const [newName, setNewName] = useState('')

  const update = (name: string, patch: Partial<InterfaceGroupConfig>) =>
    onChange({ ...interfaces, [name]: { ...interfaces[name], ...patch } })

  const remove = (name: string) => {
    const next = { ...interfaces }
    delete next[name]
    onChange(next)
  }

  const add = () => {
    const n = newName.trim()
    if (!n || interfaces[n]) return
    onChange({ ...interfaces, [n]: { patterns: [] } })
    setNewName('')
  }

  return (
    <div>
      <SectionHeader label="Interfaces" open={open} onToggle={() => setOpen(!open)} />
      {open && (
        <div className="space-y-4 mb-4">
          {Object.entries(interfaces).map(([name, cfg]) => (
            <div key={name} className="bg-black/20 border border-white/5 rounded-xl p-4">
              <div className="flex items-center justify-between mb-3">
                <span className="text-sm font-medium text-white/80 font-mono">{name}</span>
                <button onClick={() => remove(name)} className="text-white/25 hover:text-red-400 transition-colors">
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
              <Field label="Patterns" hint="Glob patterns, one per entry">
                <TagList
                  values={cfg.patterns ?? []}
                  onChange={(v) => update(name, { patterns: v })}
                  placeholder="src/**/*.ts"
                />
              </Field>
              <Field label="Provider">
                <TextInput mono value={cfg.provider ?? ''} onChange={(v) => update(name, { provider: v })} placeholder="claude" />
              </Field>
              <Field label="Model">
                <TextInput mono value={cfg.model ?? ''} onChange={(v) => update(name, { model: v })} placeholder="(default)" />
              </Field>
            </div>
          ))}
          <div className="flex gap-2">
            <input
              type="text"
              value={newName}
              onChange={(e) => setNewName(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && add()}
              placeholder="Interface group name…"
              className="flex-1 bg-black/30 border border-white/10 rounded-lg px-3 py-1.5 text-sm text-white/70 outline-none font-mono"
            />
            <button onClick={add} className="flex items-center gap-1.5 px-3 py-1.5 bg-white/5 hover:bg-white/10 border border-white/10 rounded-lg text-xs text-white/60 hover:text-white/90 transition-colors">
              <Plus className="w-3.5 h-3.5" /> Add
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

// ── Review section ─────────────────────────────────────────────────────────

function ReviewSection({ review, onChange }: { review: ReviewConfig; onChange: (r: ReviewConfig) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <div>
      <SectionHeader label="Review" open={open} onToggle={() => setOpen(!open)} />
      {open && (
        <div className="mb-4">
          <Field label="Gates" hint="Quality gates that must pass">
            <TagList values={review.gates ?? []} onChange={(v) => onChange({ ...review, gates: v })} placeholder="lint" />
          </Field>
          <Field label="Max Retries">
            <NumberInput value={review.max_retries ?? 3} onChange={(v) => onChange({ ...review, max_retries: v })} min={1} max={10} />
          </Field>
          <Field label="Auto Merge" hint="Merge PR automatically on success">
            <Toggle checked={review.auto_merge ?? true} onChange={(v) => onChange({ ...review, auto_merge: v })} />
          </Field>
        </div>
      )}
    </div>
  )
}

// ── Git section ────────────────────────────────────────────────────────────

function GitSection({ git, onChange }: { git: GitConfig; onChange: (g: GitConfig) => void }) {
  const [open, setOpen] = useState(false)
  return (
    <div>
      <SectionHeader label="Git" open={open} onToggle={() => setOpen(!open)} />
      {open && (
        <div className="mb-4">
          <Field label="Branch Prefix">
            <TextInput mono value={git.branch_prefix ?? 'polvo/'} onChange={(v) => onChange({ ...git, branch_prefix: v })} />
          </Field>
          <Field label="Target Branch">
            <TextInput mono value={git.target_branch ?? 'main'} onChange={(v) => onChange({ ...git, target_branch: v })} />
          </Field>
          <Field label="PR Labels">
            <TagList values={git.pr_labels ?? []} onChange={(v) => onChange({ ...git, pr_labels: v })} placeholder="polvo" />
          </Field>
        </div>
      )}
    </div>
  )
}

// ── Settings section ───────────────────────────────────────────────────────

function SettingsSection({ settings, onChange }: { settings: SettingsConfig; onChange: (s: SettingsConfig) => void }) {
  const [open, setOpen] = useState(false)
  const logLevels = [
    { value: 'debug', label: 'Debug' },
    { value: 'info', label: 'Info' },
    { value: 'warn', label: 'Warn' },
    { value: 'error', label: 'Error' },
  ]
  return (
    <div>
      <SectionHeader label="Settings" open={open} onToggle={() => setOpen(!open)} />
      {open && (
        <div className="mb-4">
          <Field label="Log Level">
            <SelectInput value={settings.log_level ?? 'info'} onChange={(v) => onChange({ ...settings, log_level: v })} options={logLevels} />
          </Field>
          <Field label="Debounce (ms)" hint="File watch debounce">
            <NumberInput value={settings.debounce_ms ?? 500} onChange={(v) => onChange({ ...settings, debounce_ms: v })} min={100} max={5000} />
          </Field>
          <Field label="Max Parallel Agents">
            <NumberInput value={settings.max_parallel ?? 2} onChange={(v) => onChange({ ...settings, max_parallel: v })} min={1} max={16} />
          </Field>
          <Field label="Report Dir">
            <TextInput mono value={settings.report_dir ?? '.polvo/reports'} onChange={(v) => onChange({ ...settings, report_dir: v })} />
          </Field>
        </div>
      )}
    </div>
  )
}

// ── Main modal ─────────────────────────────────────────────────────────────

interface Props {
  projectName: string
  onClose: () => void
  onSaved?: () => void
}

type SaveState = 'idle' | 'saving' | 'ok' | 'error'

export function ProjectConfigModal({ projectName, onClose, onSaved }: Props) {
  const { loadProjects } = useIDEStore()
  const [cfg, setCfg] = useState<ProjectConfigData>({})
  const [original, setOriginal] = useState('')
  const [loading, setLoading] = useState(true)
  const [saveState, setSaveState] = useState<SaveState>('idle')
  const [errorMsg, setErrorMsg] = useState('')

  useEffect(() => {
    fetch(`${API_BASE}/api/config`)
      .then((r) => r.json())
      .then((d: { yaml: string }) => {
        const parsed = (yaml.load(d.yaml) ?? {}) as ProjectConfigData
        setCfg(parsed)
        setOriginal(d.yaml)
      })
      .catch(() => setErrorMsg('Failed to load polvo.yaml'))
      .finally(() => setLoading(false))
  }, [])

  const currentYaml = yaml.dump(cfg, { indent: 2, lineWidth: -1 })
  const isDirty = currentYaml !== original

  const handleSave = async () => {
    setSaveState('saving')
    setErrorMsg('')
    try {
      const res = await fetch(`${API_BASE}/api/config`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ yaml: currentYaml }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text)
      }
      setOriginal(currentYaml)
      setSaveState('ok')
      onSaved?.()
      void loadProjects()
      setTimeout(() => setSaveState('idle'), 2000)
    } catch (e) {
      setErrorMsg(e instanceof Error ? e.message : 'Save failed')
      setSaveState('error')
    }
  }

  const patch = (partial: Partial<ProjectConfigData>) => {
    setCfg((prev) => ({ ...prev, ...partial }))
    setSaveState('idle')
    setErrorMsg('')
  }

  return (
    <AnimatePresence>
      <div className="fixed inset-0 z-[110] flex items-center justify-center">
        <motion.div
          initial={{ opacity: 0 }}
          animate={{ opacity: 1 }}
          exit={{ opacity: 0 }}
          onClick={onClose}
          className="absolute inset-0 bg-black/30 backdrop-blur-[2px]"
        />

        <motion.div
          initial={{ opacity: 0, scale: 0.95, y: 20 }}
          animate={{ opacity: 1, scale: 1, y: 0 }}
          exit={{ opacity: 0, scale: 0.95, y: 20 }}
          className="relative w-full max-w-2xl h-[82vh] bg-[#0d0d0d]/96 backdrop-blur-2xl border border-white/10 rounded-2xl shadow-2xl flex flex-col overflow-hidden"
        >
          {/* Header */}
          <div className="h-14 border-b border-white/10 flex items-center justify-between px-5 shrink-0">
            <div className="flex items-center gap-3">
              <span className="text-sm font-medium text-white/90">Project Config</span>
              <span className="text-xs text-white/30 font-mono">{projectName} / polvo.yaml</span>
              {isDirty && (
                <span className="text-xs px-1.5 py-0.5 bg-white/5 text-white/40 rounded border border-white/10">unsaved</span>
              )}
            </div>
            <div className="flex items-center gap-2">
              <button
                onClick={handleSave}
                disabled={!isDirty || saveState === 'saving'}
                className="flex items-center gap-1.5 px-3 py-1.5 text-xs font-medium rounded-lg transition-colors disabled:opacity-40 disabled:cursor-not-allowed bg-white/10 text-white hover:bg-white/20"
              >
                {saveState === 'saving' ? (
                  <div className="w-3.5 h-3.5 border border-white/30 border-t-white/80 rounded-full animate-spin" />
                ) : saveState === 'ok' ? (
                  <CheckCircle className="w-3.5 h-3.5 text-green-400" />
                ) : (
                  <Save className="w-3.5 h-3.5" />
                )}
                {saveState === 'ok' ? 'Saved' : 'Save'}
              </button>
              <button onClick={onClose} className="p-1.5 text-white/50 hover:text-white hover:bg-white/10 rounded-lg transition-colors">
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>

          {/* Error */}
          {saveState === 'error' && errorMsg && (
            <div className="flex items-start gap-2 px-5 py-2.5 bg-red-500/10 border-b border-red-500/20 text-red-400 text-xs shrink-0">
              <AlertCircle className="w-3.5 h-3.5 shrink-0 mt-0.5" />
              <span className="font-mono">{errorMsg}</span>
            </div>
          )}

          {/* Body */}
          <div className="flex-1 overflow-y-auto px-5 py-2">
            {loading ? (
              <div className="w-full h-full flex items-center justify-center">
                <div className="w-6 h-6 border border-white/20 border-t-white/60 rounded-full animate-spin" />
              </div>
            ) : (
              <div className="space-y-1">
                {/* Project identity */}
                <div className="py-2.5 border-b border-white/5 flex items-center justify-between">
                  <span className="text-sm text-white/80">Project Name</span>
                  <TextInput
                    value={cfg.project?.name ?? ''}
                    onChange={(v) => patch({ project: { ...cfg.project, name: v } })}
                    placeholder="my-project"
                  />
                </div>
                <div className="py-2.5 border-b border-white/5 flex items-start justify-between gap-4">
                  <div>
                    <div className="text-sm text-white/80">Color</div>
                    <div className="text-xs text-white/35 mt-0.5">Accent color for this project</div>
                  </div>
                  <ColorPicker
                    value={cfg.project?.color ?? ''}
                    onChange={(v) => patch({ project: { ...cfg.project, color: v || undefined } })}
                  />
                </div>
                <div className="py-2.5 border-b border-white/5 flex items-start justify-between gap-4">
                  <div>
                    <div className="text-sm text-white/80">Icon</div>
                    <div className="text-xs text-white/35 mt-0.5">Icon shown in the project list</div>
                  </div>
                  <IconPicker
                    value={cfg.project?.icon ?? ''}
                    color={cfg.project?.color ?? ''}
                    onChange={(v) => patch({ project: { ...cfg.project, icon: v || undefined } })}
                  />
                </div>

                <ProviderSection
                  providers={cfg.providers ?? {}}
                  onChange={(p) => patch({ providers: p })}
                />

                <InterfacesSection
                  interfaces={cfg.interfaces ?? {}}
                  onChange={(i) => patch({ interfaces: i })}
                />

                <ReviewSection
                  review={cfg.review ?? {}}
                  onChange={(r) => patch({ review: r })}
                />

                <GitSection
                  git={cfg.git ?? {}}
                  onChange={(g) => patch({ git: g })}
                />

                <SettingsSection
                  settings={cfg.settings ?? {}}
                  onChange={(s) => patch({ settings: s })}
                />
              </div>
            )}
          </div>
        </motion.div>
      </div>
    </AnimatePresence>
  )
}
