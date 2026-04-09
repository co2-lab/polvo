import { useEffect, useState } from 'react'
import { motion, AnimatePresence } from 'framer-motion'
import { X, CheckCircle, XCircle, Loader, Wrench } from 'lucide-react'
import { API_BASE } from '../../hooks/useSSE'

// In Tauri dev mode the WebView loads from localhost:5173 (Vite),
// so relative paths go through the Vite proxy to :7373.
// In production Tauri the WebView loads from tauri://localhost,
// so we need the absolute API_BASE.
const DOCTOR_BASE = window.location.protocol === 'tauri:' || window.location.hostname !== 'localhost'
  ? API_BASE
  : ''

interface Diagnosis {
  category: string
  label: string
  ok: boolean
  detail?: string
  fix?: string
  fixable?: boolean
}

interface Colors {
  bg: string
  surface: string
  border: string
  text: string
  accent: string
}

interface DoctorModalProps {
  open: boolean
  onClose: () => void
  colors: Colors
}

export function DoctorModal({ open, onClose, colors }: DoctorModalProps) {
  const [diags, setDiags] = useState<Diagnosis[]>([])
  const [loading, setLoading] = useState(false)
  const [fixing, setFixing] = useState<string | null>(null)

  const fetchDiags = () => {
    setLoading(true)
    setDiags([])
    fetch(`${DOCTOR_BASE}/api/doctor`)
      .then((r) => r.json())
      .then((data) => setDiags(data))
      .catch(() => setDiags([{ category: 'error', label: 'Failed to connect', ok: false, detail: 'Could not reach polvo server', fix: 'Make sure polvo is running' }]))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (!open) return
    fetchDiags()
  }, [open])

  const handleFix = async (label: string) => {
    setFixing(label)
    await fetch(`${DOCTOR_BASE}/api/doctor/fix`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ label }),
    }).catch(() => {})
    setFixing(null)
    fetchDiags()
  }

  const categories = [...new Set(diags.map((d) => d.category))]
  const allOk = diags.length > 0 && diags.every((d) => d.ok)

  return (
    <AnimatePresence>
      {open && (
        <div className="fixed inset-0 z-[400] flex items-center justify-center">
          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            onClick={onClose}
            className="absolute inset-0 bg-black/30 backdrop-blur-[2px]"
          />
          <motion.div
            initial={{ opacity: 0, scale: 0.95 }}
            animate={{ opacity: 1, scale: 1 }}
            exit={{ opacity: 0, scale: 0.95 }}
            transition={{ duration: 0.15 }}
            className="relative w-full max-w-lg max-h-[80vh] flex flex-col rounded-xl shadow-2xl overflow-hidden"
            style={{ backgroundColor: colors.bg, border: `1px solid ${colors.border}` }}
            onClick={e => e.stopPropagation()}
          >
            {/* Header */}
            <div className="flex items-center justify-between px-6 py-4" style={{ borderBottom: `1px solid ${colors.border}` }}>
              <div className="flex items-center gap-3">
                <span className="font-semibold text-sm" style={{ color: colors.text }}>Doctor</span>
                {!loading && diags.length > 0 && (
                  <span className={`text-xs px-2 py-0.5 rounded-full ${allOk ? 'bg-green-500/10 text-green-400' : 'bg-red-500/10 text-red-400'}`}>
                    {allOk ? 'All checks passed' : `${diags.filter((d) => !d.ok).length} issue(s) found`}
                  </span>
                )}
              </div>
              <button
                onClick={onClose}
                className="p-1 rounded-md transition-colors hover:bg-white/10"
                style={{ color: colors.text, opacity: 0.5 }}
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            {/* Body */}
            <div className="flex-1 overflow-y-auto px-6 py-4 space-y-6">
              {loading && (
                <div className="flex items-center gap-3 py-8 justify-center" style={{ color: colors.text, opacity: 0.5 }}>
                  <Loader className="w-4 h-4 animate-spin" />
                  <span className="text-sm">Running diagnostics…</span>
                </div>
              )}

              {!loading && categories.map((cat) => (
                <div key={cat}>
                  <p className="text-xs font-semibold uppercase tracking-wider mb-2" style={{ color: colors.text, opacity: 0.4 }}>{cat}</p>
                  <div className="space-y-2">
                    {diags.filter((d) => d.category === cat).map((d, i) => (
                      <div
                        key={i}
                        className="rounded-lg p-3"
                        style={{ backgroundColor: colors.surface, border: `1px solid ${colors.border}` }}
                      >
                        <div className="flex items-start gap-2">
                          {d.ok
                            ? <CheckCircle className="w-4 h-4 text-green-400 mt-0.5 shrink-0" />
                            : <XCircle className="w-4 h-4 text-red-400 mt-0.5 shrink-0" />
                          }
                          <div className="flex-1 min-w-0">
                            <p className="text-sm font-medium" style={{ color: colors.text }}>{d.label}</p>
                            {d.detail && (
                              <p className="text-xs mt-0.5" style={{ color: colors.text, opacity: 0.5 }}>{d.detail}</p>
                            )}
                            {!d.ok && d.fix && !d.fixable && (
                              <p className="text-xs mt-1.5 px-2 py-1 rounded bg-yellow-500/10 text-yellow-400 font-mono">{d.fix}</p>
                            )}
                            {!d.ok && d.fixable && d.fix && (
                              <button
                                onClick={() => handleFix(d.label)}
                                disabled={fixing === d.label}
                                className="mt-1.5 flex items-center gap-1.5 px-2 py-1 rounded text-xs font-medium transition-colors bg-yellow-500/10 text-yellow-400 hover:bg-yellow-500/20 disabled:opacity-50"
                              >
                                {fixing === d.label
                                  ? <Loader className="w-3 h-3 animate-spin" />
                                  : <Wrench className="w-3 h-3" />
                                }
                                {d.fix}
                              </button>
                            )}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              ))}
            </div>
          </motion.div>
        </div>
      )}
    </AnimatePresence>
  )
}
