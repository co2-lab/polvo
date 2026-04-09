import { useState, useEffect, useCallback } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { X, Plus, Trash2, ChevronRight, ChevronLeft, Loader, Check, SkipForward } from 'lucide-react'
import { API_BASE } from '../../hooks/useSSE'

const INIT_BASE = window.location.protocol === 'tauri:' || window.location.hostname !== 'localhost'
  ? API_BASE
  : ''

interface Colors {
  bg: string
  surface: string
  border: string
  text: string
  accent: string
}

// Shared guide config — same schema as GuideConfig in Go
interface GuideConfig {
  name: string       // instance name (e.g. "go-lint", "spec")
  base: string       // builtin to extend/replace (e.g. "lint") — empty if name matches builtin
  mode: string       // extend | replace | ''
  file: string
  provider: string
  model: string
  prompt: string
  role: string       // author | reviewer | ''
  useTools: boolean
}

interface InterfaceGroup {
  name: string
  patterns: string   // one per line
  specPath: string   // derived.spec template
  provider: string
  model: string
}

interface Provider {
  name: string
  type: string
  envVar: string      // e.g. ANTHROPIC_API_KEY — goes into polvo.yaml as ${ENV_VAR}
  apiKeyValue: string // actual secret — goes to .env only
  baseURL: string
  defaultModel: string
}

interface InitModalProps {
  open: boolean
  onClose: () => void
  onSuccess: () => void
  colors: Colors
  cwd?: string
}

const PROVIDER_TYPES = ['claude', 'openai', 'ollama', 'gemini', 'openai-compatible']


const PROVIDER_NEEDS_KEY: Record<string, boolean> = {
  claude: true, openai: true, gemini: true, ollama: false, 'openai-compatible': false,
}

const PROVIDER_DEFAULT_ENV: Record<string, string> = {
  claude: 'ANTHROPIC_API_KEY',
  openai: 'OPENAI_API_KEY',
  gemini: 'GEMINI_API_KEY',
  ollama: '',
  'openai-compatible': '',
}

const PROVIDER_NEEDS_URL: Record<string, boolean> = {
  ollama: true, 'openai-compatible': true, claude: false, openai: false, gemini: false,
}

const BUILTIN_GUIDES = ['spec', 'features', 'tests', 'review', 'lint', 'best-practices', 'docs']

const GUIDE_DESC: Record<string, string> = {
  spec:             'Generates specification documents from interfaces',
  features:         'Breaks specs into implementable feature stories',
  tests:            'Writes test cases from feature stories',
  review:           'Reviews generated code for quality',
  lint:             'Checks code style and formatting',
  'best-practices': 'Enforces architectural best practices',
  docs:             'Generates documentation',
}

function emptyGuide(name: string): GuideConfig {
  return { name, base: '', mode: '', file: '', provider: '', model: '', prompt: '', role: '', useTools: false }
}


function emptyInterface(): InterfaceGroup {
  return { name: '', patterns: '', specPath: '{{.Dir}}/{{.Name}}.spec.md', provider: '', model: '' }
}

// ── shared sub-components ────────────────────────────────────────────────────

function TextInput({ label, value, onChange, placeholder, type = 'text', mono = false }: {
  label: string; value: string; onChange: (v: string) => void
  placeholder?: string; type?: string; mono?: boolean
}) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs font-medium opacity-60">{label}</label>
      <input
        type={type} value={value} onChange={e => onChange(e.target.value)}
        placeholder={placeholder}
        className={`px-3 py-2 h-[38px] rounded-lg text-sm bg-black/20 border border-white/10 outline-none focus:border-[#00ffab]/50 transition-colors placeholder:opacity-30 ${mono ? 'font-mono' : ''}`}
      />
    </div>
  )
}

type SelectOption = { value: string; label: string }
type SelectGroup = { group: string; options: SelectOption[] }
type SelectItems = SelectOption[] | (SelectOption | SelectGroup)[]

function isGroup(item: SelectOption | SelectGroup): item is SelectGroup {
  return 'group' in item
}

function SelectInput({ label, value, onChange, options }: {
  label: string; value: string; onChange: (v: string) => void
  options: SelectItems
}) {
  return (
    <div className="flex flex-col gap-1">
      <label className="text-xs font-medium opacity-60">{label}</label>
      <select value={value} onChange={e => onChange(e.target.value)}
        className="px-3 py-2 h-[38px] rounded-lg text-sm bg-black/20 border border-white/10 outline-none focus:border-[#00ffab]/50 transition-colors">
        {options.map((item, i) =>
          isGroup(item)
            ? <optgroup key={i} label={item.group}>
                {item.options.map(o => <option key={o.value} value={o.value}>{o.label}</option>)}
              </optgroup>
            : <option key={item.value} value={item.value}>{item.label}</option>
        )}
      </select>
    </div>
  )
}

function groupModels(models: string[], providerType: string): SelectItems {
  if (models.length === 0) return []

  const families: Record<string, string[]> = {}
  const ungrouped: string[] = []

  for (const m of models) {
    let family: string | null = null

    if (providerType === 'claude') {
      const match = m.match(/^claude-(opus|sonnet|haiku)/i)
      if (match) family = match[1].charAt(0).toUpperCase() + match[1].slice(1)
    } else if (providerType === 'openai') {
      if (m.startsWith('o1') || m.startsWith('o3') || m.startsWith('o4')) family = m.split('-')[0].toUpperCase()
      else if (m.startsWith('gpt-4o')) family = 'GPT-4o'
      else if (m.startsWith('gpt-4')) family = 'GPT-4'
      else if (m.startsWith('gpt-3.5')) family = 'GPT-3.5'
    } else if (providerType === 'gemini') {
      const match = m.match(/^(gemini-[^-]+)/i)
      if (match) family = match[1].replace(/-/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
    }

    if (family) {
      if (!families[family]) families[family] = []
      families[family].push(m)
    } else {
      ungrouped.push(m)
    }
  }

  const hasGroups = Object.keys(families).length > 0
  if (!hasGroups) return models.map(m => ({ value: m, label: m }))

  const result: SelectItems = []
  for (const [group, opts] of Object.entries(families)) {
    result.push({ group, options: opts.map(m => ({ value: m, label: m })) })
  }
  if (ungrouped.length > 0) {
    result.push({ group: 'Other', options: ungrouped.map(m => ({ value: m, label: m })) })
  }
  return result
}

function StepDot({ active, done, num }: { active: boolean; done: boolean; num: number }) {
  return (
    <div className={`w-6 h-6 rounded-full flex items-center justify-center text-xs font-semibold transition-all shrink-0 ${
      done ? 'bg-[#00ffab] text-black' : active ? 'bg-[#00ffab]/20 border border-[#00ffab] text-[#00ffab]' : 'bg-white/5 border border-white/10 text-white/30'
    }`}>
      {done ? <Check className="w-3 h-3" /> : num}
    </div>
  )
}

function GuideCard({ g, idx, colors, onChange, onRemove, providerOptions, modelOptions }: {
  g: GuideConfig; idx: number; colors: Colors
  onChange: (updated: GuideConfig) => void
  onRemove: () => void
  providerOptions: SelectItems
  modelOptions: SelectItems
}) {
  const up = (field: keyof GuideConfig, value: unknown) => onChange({ ...g, [field]: value })
  const kindLabel = g.base || g.name || '—'

  return (
    <div className="rounded-lg p-4 space-y-3" style={{ backgroundColor: colors.surface, border: `1px solid ${colors.border}` }}>
      <div className="flex items-center justify-between">
        <span className="text-xs font-semibold opacity-40" style={{ color: colors.text }}>Guide {idx + 1}</span>
        <button onClick={onRemove} className="p-1 rounded hover:bg-red-500/10 text-red-400 opacity-60 hover:opacity-100 transition-all">
          <Trash2 className="w-3.5 h-3.5" />
        </button>
      </div>

      <div className="grid grid-cols-2 gap-3">
        <TextInput label="Name" value={g.name} onChange={v => up('name', v)} placeholder="go-lint" mono />
        <SelectInput label="Kind" value={g.base} onChange={v => up('base', v)} options={[
          { value: '', label: '— select kind —' },
          ...BUILTIN_GUIDES.map(n => ({ value: n, label: n })),
        ]} />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <SelectInput label="Mode" value={g.mode} onChange={v => up('mode', v)} options={[
          { value: '', label: '— default (extend) —' },
          { value: 'extend', label: 'extend' },
          { value: 'replace', label: 'replace' },
        ]} />
        <SelectInput label="Role" value={g.role} onChange={v => up('role', v)} options={[
          { value: '', label: '— default —' },
          { value: 'author', label: 'author' },
          { value: 'reviewer', label: 'reviewer' },
        ]} />
      </div>

      <div className="grid grid-cols-2 gap-3">
        <SelectInput label="Provider (optional)" value={g.provider} onChange={v => up('provider', v)} options={providerOptions} />
        <SelectInput label="Model (optional)" value={g.model} onChange={v => up('model', v)} options={modelOptions} />
      </div>

      <TextInput label="Guide file" value={g.file} onChange={v => up('file', v)} placeholder={`guides/${kindLabel}.md`} mono />

      <div className="flex flex-col gap-1">
        <label className="text-xs font-medium opacity-60">Inline prompt override</label>
        <textarea value={g.prompt} onChange={e => up('prompt', e.target.value)}
          placeholder="Leave empty to use the built-in or file content"
          rows={2}
          className="px-3 py-2 rounded-lg text-sm bg-black/20 border border-white/10 outline-none focus:border-[#00ffab]/50 transition-colors placeholder:opacity-30 resize-none" />
      </div>

      <label className="flex items-center gap-2 cursor-pointer">
        <input type="checkbox" checked={g.useTools} onChange={e => up('useTools', e.target.checked)}
          className="w-3.5 h-3.5 accent-[#00ffab]" />
        <span className="text-xs opacity-60" style={{ color: colors.text }}>Enable tool use</span>
      </label>
    </div>
  )
}

function cwdBasename(cwd?: string): string {
  if (!cwd) return ''
  const parts = cwd.replace(/\\/g, '/').split('/').filter(Boolean)
  return parts[parts.length - 1] ?? ''
}

const STEPS = ['Project', 'Providers', 'Interfaces', 'Guides']

export function InitModal({ open, onClose, onSuccess, colors, cwd }: InitModalProps) {
  const [step, setStep] = useState(0)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Step 0
  const [projectName, setProjectName] = useState('')
  const [nameSuggestions, setNameSuggestions] = useState<string[]>([])

  // Step 1
  const [providers, setProviders] = useState<Provider[]>([
    { name: 'main', type: 'claude', envVar: 'ANTHROPIC_API_KEY', apiKeyValue: '', baseURL: '', defaultModel: '' },
  ])
  const [providerModels, setProviderModels] = useState<Record<number, string[]>>({})
  const [providerModelsLoading, setProviderModelsLoading] = useState<Record<number, boolean>>({})

  const fetchModels = useCallback(async (idx: number, p: Provider) => {
    const needsKey = PROVIDER_NEEDS_KEY[p.type]
    const needsUrl = PROVIDER_NEEDS_URL[p.type]
    if (needsKey && !p.apiKeyValue) return
    if (needsUrl && !p.baseURL) return
    setProviderModelsLoading(prev => ({ ...prev, [idx]: true }))
    try {
      const res = await fetch(`${INIT_BASE}/api/models`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ type: p.type, api_key: p.apiKeyValue, base_url: p.baseURL }),
      })
      if (res.ok) {
        const models = await res.json() as string[]
        setProviderModels(prev => ({ ...prev, [idx]: models }))
      } else {
        setProviderModels(prev => ({ ...prev, [idx]: [] }))
      }
    } catch {
      setProviderModels(prev => ({ ...prev, [idx]: [] }))
    } finally {
      setProviderModelsLoading(prev => ({ ...prev, [idx]: false }))
    }
  }, [])

  // Auto-fetch models when entering the Providers step for any provider that already has enough info
  useEffect(() => {
    if (step !== 1) return
    providers.forEach((p, i) => {
      const needsKey = PROVIDER_NEEDS_KEY[p.type]
      const needsUrl = PROVIDER_NEEDS_URL[p.type]
      const canFetch = (!needsKey || p.apiKey) && (!needsUrl || p.baseURL)
      if (canFetch) fetchModels(i, p)
    })
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step])

  // Step 2
  const [interfaces, setInterfaces] = useState<InterfaceGroup[]>([emptyInterface()])

  // Step 3 — start empty, user adds guides explicitly
  const [guides, setGuides] = useState<GuideConfig[]>([])

  useEffect(() => {
    if (open) {
      const folder = cwdBasename(cwd)
      const clean = folder.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-|-$/g, '')
      setNameSuggestions(folder ? (clean && clean !== folder ? [folder, clean] : [folder]) : [])
      setProjectName('')
      setStep(0)
      setError(null)
    }
  }, [open, cwd])

  // Auto-fetch models for providers that don't need a key (e.g. ollama) when step becomes visible
  useEffect(() => {
    if (step !== 1) return
    providers.forEach((p, i) => {
      const needsKey = PROVIDER_NEEDS_KEY[p.type]
      const needsUrl = PROVIDER_NEEDS_URL[p.type]
      if (!needsKey && !needsUrl) {
        fetchModels(i, p)
      }
    })
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [step])

  // Providers
  const addProvider = () =>
    setProviders(p => [...p, { name: '', type: 'claude', envVar: 'ANTHROPIC_API_KEY', apiKeyValue: '', baseURL: '', defaultModel: '' }])
  const removeProvider = (i: number) => setProviders(p => p.filter((_, idx) => idx !== i))
  const updateProvider = (i: number, field: keyof Provider, value: string) => {
    setProviders(p => {
      const next = p.map((item, idx) => {
        if (idx !== i) return item
        const u = { ...item, [field]: value }
        if (field === 'type') {
          u.defaultModel = ''
          u.envVar = PROVIDER_DEFAULT_ENV[value] ?? ''
        }
        return u
      })
      if (field === 'type') {
        setProviderModels(prev => ({ ...prev, [i]: [] }))
      }
      const updated = next[i]
      if (field === 'type' || field === 'apiKeyValue' || field === 'baseURL') {
        setTimeout(() => fetchModels(i, updated), 0)
      }
      return next
    })
  }

  // Interfaces
  const addInterface = () => setInterfaces(p => [...p, emptyInterface()])
  const removeInterface = (i: number) => setInterfaces(p => p.filter((_, idx) => idx !== i))
  const updateInterface = (i: number, field: keyof InterfaceGroup, value: unknown) =>
    setInterfaces(p => p.map((item, idx) => idx === i ? { ...item, [field]: value } : item))
  // Global guides
  const updateGuide = (i: number, updated: GuideConfig) =>
    setGuides(g => g.map((item, idx) => idx === i ? updated : item))
  const removeGuide = (i: number) =>
    setGuides(g => g.filter((_, idx) => idx !== i))
  const addGuide = () =>
    setGuides(g => [...g, emptyGuide('')])

  // Aggregate all fetched models across providers for use in Interface/Guide dropdowns.
  // Groups by provider name so the user knows where each model comes from.
  const allModelOptions = (selectedProvider?: string): SelectItems => {
    // Find the effective default model: from selected provider, or first provider with a default
    let defaultModel = ''
    if (selectedProvider) {
      const p = providers.find(p => p.name.trim() === selectedProvider)
      defaultModel = p?.defaultModel ?? ''
    }
    if (!defaultModel) {
      defaultModel = providers.find(p => p.defaultModel)?.defaultModel ?? ''
    }

    const defaultLabel = defaultModel
      ? `— use default (${defaultModel}) —`
      : '— use default —'

    const result: SelectItems = [{ value: '', label: defaultLabel }]
    providers.forEach((p, i) => {
      const models = providerModels[i]
      if (!models || models.length === 0) return
      const providerName = p.name.trim() || `Provider ${i + 1}`
      result.push({ group: providerName, options: models.map(m => ({ value: m, label: m })) })
    })
    return result
  }

  const canNext = () => step === 0 ? projectName.trim() !== '' : true

  const handleSubmit = async () => {
    setSubmitting(true)
    setError(null)
    try {
      const body = {
        project_name: projectName.trim(),
        providers: providers.filter(p => p.name.trim()).map(p => ({
          name: p.name.trim(), type: p.type,
          env_var: p.envVar, api_key_value: p.apiKeyValue,
          base_url: p.baseURL, default_model: p.defaultModel,
        })),
        interfaces: interfaces.filter(i => i.name.trim() && i.patterns.trim()).map(i => ({
          name: i.name.trim(),
          patterns: i.patterns.split('\n').map(s => s.trim()).filter(Boolean),
          spec_path: i.specPath,
          provider: i.provider,
          model: i.model,
        })),
        guides: guides.filter(g => g.name.trim()).map(g => ({
          name: g.name.trim(), base: g.base, mode: g.mode, file: g.file,
          provider: g.provider, model: g.model,
          prompt: g.prompt, role: g.role, use_tools: g.useTools,
        })),
      }
      const res = await fetch(`${INIT_BASE}/api/init`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      if (!res.ok) {
        setError(await res.text() || 'Failed to initialize project')
        return
      }
      onSuccess()
    } catch {
      setError('Could not reach polvo server')
    } finally {
      setSubmitting(false)
    }
  }

  const isLastStep = step === STEPS.length - 1

  return (
    <AnimatePresence>
      {open && (
        <div className="fixed inset-0 z-[400] flex items-center justify-center">
          <motion.div initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}
            onClick={onClose} className="absolute inset-0 bg-black/30 backdrop-blur-[2px]" />
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }} animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95 }} transition={{ duration: 0.15 }}
            className="relative w-full max-w-2xl max-h-[90vh] flex flex-col rounded-xl shadow-2xl overflow-hidden"
            style={{ backgroundColor: colors.bg, border: `1px solid ${colors.border}` }}
            onClick={e => e.stopPropagation()}
          >
            {/* Header */}
            <div className="flex items-center justify-between px-6 py-4" style={{ borderBottom: `1px solid ${colors.border}` }}>
              <div className="flex items-center gap-3">
                {STEPS.map((label, i) => (
                  <div key={i} className="flex items-center gap-2">
                    <StepDot active={step === i} done={step > i} num={i + 1} />
                    <span className="text-xs transition-colors" style={{
                      color: step === i ? '#00ffab' : colors.text,
                      opacity: step === i ? 1 : step > i ? 0.35 : 0.25,
                    }}>{label}</span>
                    {i < STEPS.length - 1 && <div className="w-6 h-px bg-white/10" />}
                  </div>
                ))}
              </div>
              <button onClick={onClose} className="p-1 rounded-md transition-colors hover:bg-white/10 ml-4" style={{ color: colors.text, opacity: 0.5 }}>
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Body */}
            <div className="flex-1 overflow-y-auto px-6 py-5" style={{ backgroundColor: colors.bg }}>
              <AnimatePresence mode="wait">

                {/* Step 0: Project */}
                {step === 0 && (
                  <motion.div key="s0" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.1 }} className="space-y-4">
                    <p className="text-sm opacity-50" style={{ color: colors.text }}>Give your project a name. This will appear in reports and logs.</p>
                    <div className="flex flex-col gap-2">
                      <TextInput label="Project name" value={projectName} onChange={setProjectName} placeholder="my-project" />
                      {nameSuggestions.length > 0 && (
                        <div className="flex items-center gap-2 flex-wrap">
                          <span className="text-xs opacity-30" style={{ color: colors.text }}>Suggestions:</span>
                          {nameSuggestions.map(s => (
                            <button key={s} onClick={() => setProjectName(s)}
                              className={`text-xs px-2.5 py-0.5 rounded-full font-mono transition-all border ${projectName === s ? 'border-[#00ffab]/50 bg-[#00ffab]/10 text-[#00ffab]' : 'border-white/10 bg-white/5 hover:bg-white/10'}`}
                              style={{ color: projectName === s ? '#00ffab' : colors.text }}>
                              {s}
                            </button>
                          ))}
                        </div>
                      )}
                    </div>
                    {cwd && <p className="text-xs opacity-25 font-mono" style={{ color: colors.text }}>{cwd}</p>}
                  </motion.div>
                )}

                {/* Step 1: Providers */}
                {step === 1 && (
                  <motion.div key="s1" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.1 }} className="space-y-4">
                    <p className="text-sm opacity-50" style={{ color: colors.text }}>Configure the LLM providers polvo will use. You can skip and add them later in polvo.yaml.</p>
                    {providers.map((p, i) => (
                      <div key={i} className="rounded-lg p-4 space-y-3" style={{ backgroundColor: colors.surface, border: `1px solid ${colors.border}` }}>
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-semibold opacity-40" style={{ color: colors.text }}>Provider {i + 1}</span>
                          {providers.length > 1 && (
                            <button onClick={() => removeProvider(i)} className="p-1 rounded hover:bg-red-500/10 text-red-400 opacity-60 hover:opacity-100 transition-all">
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          )}
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                          <TextInput label="Name" value={p.name} onChange={v => updateProvider(i, 'name', v)} placeholder="main" mono />
                          <SelectInput label="Type" value={p.type} onChange={v => updateProvider(i, 'type', v)}
                            options={PROVIDER_TYPES.map(t => ({ value: t, label: t }))} />
                        </div>
                        {PROVIDER_NEEDS_KEY[p.type] && (
                          <div className="space-y-2">
                            <div className="grid grid-cols-2 gap-3">
                              <TextInput label="Env var name" value={p.envVar} onChange={v => updateProvider(i, 'envVar', v)} placeholder="ANTHROPIC_API_KEY" mono />
                              <TextInput label="API Key (saved to .env)" value={p.apiKeyValue} onChange={v => updateProvider(i, 'apiKeyValue', v)} placeholder="sk-..." type="password" mono />
                            </div>
                            <p className="text-xs opacity-30 font-mono" style={{ color: 'white' }}>
                              polvo.yaml will reference <span className="opacity-70">${'{'}{ p.envVar || 'ENV_VAR' }{'}'}</span> — the actual key stays in <span className="opacity-70">.env</span> (git-ignored)
                            </p>
                          </div>
                        )}
                        {PROVIDER_NEEDS_URL[p.type] && (
                          <TextInput label="Base URL" value={p.baseURL} onChange={v => updateProvider(i, 'baseURL', v)} placeholder="http://localhost:11434" mono />
                        )}
                        {(() => {
                          const models = providerModels[i]
                          const loading = providerModelsLoading[i]
                          const needsKey = PROVIDER_NEEDS_KEY[p.type]
                          const needsUrl = PROVIDER_NEEDS_URL[p.type]
                          const canFetch = (!needsKey || p.apiKeyValue) && (!needsUrl || p.baseURL)
                          if (!canFetch) return null
                          if (loading) return (
                            <div className="flex flex-col gap-1">
                              <label className="text-xs font-medium opacity-60">Default model</label>
                              <div className="px-3 h-[38px] rounded-lg text-sm bg-black/20 border border-white/10 flex items-center gap-2 opacity-50" style={{ color: colors.text }}>
                                <Loader className="w-3 h-3 animate-spin" /> Loading models…
                              </div>
                            </div>
                          )
                          if (models && models.length > 0) return (
                            <SelectInput label="Default model" value={p.defaultModel} onChange={v => updateProvider(i, 'defaultModel', v)}
                              options={[
                                { value: '', label: '— use provider default —' },
                                ...groupModels(models, p.type),
                              ]} />
                          )
                          return (
                            <TextInput label="Default model" value={p.defaultModel} onChange={v => updateProvider(i, 'defaultModel', v)} placeholder="model-name" mono />
                          )
                        })()}
                      </div>
                    ))}
                    <button onClick={addProvider} className="flex items-center gap-2 text-xs opacity-50 hover:opacity-100 transition-opacity px-2 py-1" style={{ color: colors.text }}>
                      <Plus className="w-3.5 h-3.5" /> Add provider
                    </button>
                  </motion.div>
                )}

                {/* Step 2: Interfaces */}
                {step === 2 && (
                  <motion.div key="s2" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.1 }} className="space-y-4">
                    <p className="text-sm opacity-50" style={{ color: colors.text }}>Define file groups polvo will watch. You can skip and configure later.</p>
                    {interfaces.map((iface, i) => (
                      <div key={i} className="rounded-lg p-4 space-y-3" style={{ backgroundColor: colors.surface, border: `1px solid ${colors.border}` }}>
                        <div className="flex items-center justify-between">
                          <span className="text-xs font-semibold opacity-40" style={{ color: colors.text }}>Interface {i + 1}</span>
                          {interfaces.length > 1 && (
                            <button onClick={() => removeInterface(i)} className="p-1 rounded hover:bg-red-500/10 text-red-400 opacity-60 hover:opacity-100 transition-all">
                              <Trash2 className="w-3.5 h-3.5" />
                            </button>
                          )}
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                          <TextInput label="Name" value={iface.name} onChange={v => updateInterface(i, 'name', v)} placeholder="api" mono />
                          <SelectInput label="Provider (optional)" value={iface.provider} onChange={v => updateInterface(i, 'provider', v)}
                            options={[{ value: '', label: '— use default —' }, ...providers.filter(p => p.name.trim()).map(p => ({ value: p.name, label: p.name }))]} />
                        </div>
                        <div className="grid grid-cols-2 gap-3">
                          <div className="flex flex-col gap-1">
                            <label className="text-xs font-medium opacity-60">Glob patterns (one per line)</label>
                            <textarea value={iface.patterns} onChange={e => updateInterface(i, 'patterns', e.target.value)}
                              placeholder={"**/*.go\n**/*.ts"} rows={3}
                              className="px-3 py-2 rounded-lg text-sm bg-black/20 border border-white/10 outline-none focus:border-[#00ffab]/50 transition-colors placeholder:opacity-30 font-mono resize-none" />
                          </div>
                          <div className="space-y-3">
                            <TextInput label="Spec path template" value={iface.specPath} onChange={v => updateInterface(i, 'specPath', v)} placeholder="{{dir}}/{{name}}.spec.md" mono />
                            <SelectInput label="Model (optional)" value={iface.model} onChange={v => updateInterface(i, 'model', v)} options={allModelOptions(iface.provider)} />
                          </div>
                        </div>
                      </div>
                    ))}
                    <button onClick={addInterface} className="flex items-center gap-2 text-xs opacity-50 hover:opacity-100 transition-opacity px-2 py-1" style={{ color: colors.text }}>
                      <Plus className="w-3.5 h-3.5" /> Add interface group
                    </button>
                  </motion.div>
                )}

                {/* Step 3: Guides */}
                {step === 3 && (
                  <motion.div key="s3" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} transition={{ duration: 0.1 }} className="space-y-4">
                    <p className="text-sm opacity-50" style={{ color: colors.text }}>
                      Add guide instances. Each guide has a <span className="font-mono opacity-80">Kind</span> (the built-in type it extends) and a <span className="font-mono opacity-80">Name</span> (your instance name). You can have multiple instances of the same kind for different contexts.
                    </p>
                    {guides.map((g, i) => (
                      <GuideCard key={i} g={g} idx={i} colors={colors}
                        onChange={updated => updateGuide(i, updated)}
                        onRemove={() => removeGuide(i)}
                        providerOptions={[{ value: '', label: '— use default —' }, ...providers.filter(p => p.name.trim()).map(p => ({ value: p.name, label: p.name }))]}
                        modelOptions={allModelOptions(g.provider)}
                      />
                    ))}
                    <button onClick={addGuide}
                      className="flex items-center gap-2 text-xs opacity-50 hover:opacity-100 transition-opacity px-2 py-1"
                      style={{ color: colors.text }}>
                      <Plus className="w-3.5 h-3.5" /> Add guide
                    </button>
                  </motion.div>
                )}

              </AnimatePresence>

              {error && (
                <div className="mt-4 px-3 py-2 rounded-lg bg-red-500/10 border border-red-500/20">
                  <p className="text-xs text-red-400">{error}</p>
                </div>
              )}
            </div>

            {/* Footer */}
            <div className="flex items-center justify-between px-6 py-4" style={{ borderTop: `1px solid ${colors.border}` }}>
              <button onClick={() => setStep(s => s - 1)} disabled={step === 0}
                className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors hover:bg-white/5 disabled:opacity-20 disabled:pointer-events-none"
                style={{ color: colors.text }}>
                <ChevronLeft className="w-4 h-4" /> Back
              </button>

              <div className="flex items-center gap-2">
                {step > 0 && !isLastStep && (
                  <button onClick={() => setStep(s => s + 1)}
                    className="flex items-center gap-1.5 px-3 py-1.5 rounded-lg text-sm transition-colors hover:bg-white/5"
                    style={{ color: colors.text, opacity: 0.5 }}>
                    <SkipForward className="w-3.5 h-3.5" /> Skip
                  </button>
                )}
                {!isLastStep ? (
                  <button onClick={() => setStep(s => s + 1)} disabled={!canNext()}
                    className="flex items-center gap-1.5 px-4 py-1.5 rounded-lg text-sm font-medium transition-all bg-[#00ffab]/10 text-[#00ffab] hover:bg-[#00ffab]/20 disabled:opacity-30 disabled:pointer-events-none">
                    Next <ChevronRight className="w-4 h-4" />
                  </button>
                ) : (
                  <button onClick={handleSubmit} disabled={submitting}
                    className="flex items-center gap-1.5 px-4 py-1.5 rounded-lg text-sm font-medium transition-all bg-[#00ffab]/10 text-[#00ffab] hover:bg-[#00ffab]/20 disabled:opacity-50">
                    {submitting ? <Loader className="w-4 h-4 animate-spin" /> : <Check className="w-4 h-4" />}
                    {submitting ? 'Creating…' : 'Create polvo.yaml'}
                  </button>
                )}
              </div>
            </div>
          </motion.div>
        </div>
      )}
    </AnimatePresence>
  )
}
