import { useState } from 'react'
import { Settings } from 'lucide-react'
import { exit } from '@tauri-apps/plugin-process'
import logoUrl from '../../assets/logo.svg'
import titleUrl from '../../assets/title.svg'
import { useIDEStore } from '../../store/useIDEStore'
import { DoctorModal } from './DoctorModal'
import { InitModal } from './InitModal'

interface Colors {
  bg: string
  surface: string
  border: string
  text: string
  accent: string
}

interface WelcomeScreenProps {
  version: string
  cwd?: string
  colors: Colors
}

export function WelcomeScreen({ version, cwd, colors }: WelcomeScreenProps) {
  const { setSettingsOpen } = useIDEStore()
  const [doctorOpen, setDoctorOpen] = useState(false)
  const [initOpen, setInitOpen] = useState(false)

  const handleExit = async () => {
    await exit(0)
  }

  const handleInitSuccess = () => {
    setInitOpen(false)
  }

  return (
    <div
      className="fixed inset-0 z-[200] flex flex-col items-center justify-center font-sans"
      style={{ backgroundColor: colors.bg, color: colors.text }}
    >
      <button
        onClick={() => setSettingsOpen(true)}
        className="absolute top-4 right-4 p-1.5 rounded-md transition-colors hover:bg-white/10"
        style={{ color: colors.text, opacity: 0.5 }}
        title="Settings"
      >
        <Settings className="w-4 h-4" />
      </button>

      <div className="flex flex-col items-center max-w-md w-full p-8">

        <div className="mb-6">
          <img src={logoUrl} alt="Polvo logo" width={200} height={140} />
        </div>

        <img src={titleUrl} alt="polvo" width={244} height={44} className="mb-8" />

        <div className="text-center mb-12">
          <p className="text-sm mb-2 opacity-60" style={{ color: colors.text }}>v{version}</p>
          <p className="font-medium opacity-80" style={{ color: colors.text }}>AI agent orchestrator for spec-first projects</p>
        </div>

        <div className="bg-red-500/10 border border-red-500/20 rounded-lg p-4 mb-8 w-full text-center">
          <p className="text-red-400 text-sm font-medium">No polvo.yaml config found</p>
          {cwd && <p className="mt-1 font-mono text-red-400/60 text-xs">{cwd}</p>}
        </div>

        <div className="flex flex-col gap-3 w-full">
          <button
            onClick={() => setInitOpen(true)}
            className="flex items-center justify-between px-4 py-3 rounded-lg transition-colors group hover:bg-white/5"
            style={{ border: `1px solid ${colors.border}` }}
          >
            <div className="flex flex-col items-start">
              <span className="font-medium transition-colors group-hover:text-[#00ffab]" style={{ color: colors.text }}>Init</span>
              <span className="text-xs" style={{ color: colors.text, opacity: 0.5 }}>Initialize project (interactive wizard)</span>
            </div>
            <span className="transition-colors group-hover:text-[#00ffab]" style={{ color: colors.text, opacity: 0.2 }}>→</span>
          </button>

          <button
            onClick={() => setDoctorOpen(true)}
            className="flex items-center justify-between px-4 py-3 rounded-lg transition-colors group hover:bg-white/5"
            style={{ border: `1px solid ${colors.border}` }}
          >
            <div className="flex flex-col items-start">
              <span className="font-medium transition-colors group-hover:text-[#00ffab]" style={{ color: colors.text }}>Doctor</span>
              <span className="text-xs" style={{ color: colors.text, opacity: 0.5 }}>Run environment diagnostics</span>
            </div>
            <span className="transition-colors group-hover:text-[#00ffab]" style={{ color: colors.text, opacity: 0.2 }}>→</span>
          </button>

          <button
            onClick={handleExit}
            className="flex items-center justify-between px-4 py-3 rounded-lg transition-colors group hover:bg-white/5"
            style={{ border: `1px solid ${colors.border}` }}
          >
            <div className="flex flex-col items-start">
              <span className="font-medium transition-colors group-hover:text-red-400" style={{ color: colors.text }}>Exit</span>
              <span className="text-xs" style={{ color: colors.text, opacity: 0.5 }}>Exit polvo</span>
            </div>
            <span className="transition-colors group-hover:text-red-400" style={{ color: colors.text, opacity: 0.2 }}>→</span>
          </button>
        </div>

      </div>

      <DoctorModal open={doctorOpen} onClose={() => setDoctorOpen(false)} colors={colors} />
      <InitModal open={initOpen} onClose={() => setInitOpen(false)} onSuccess={handleInitSuccess} colors={colors} cwd={cwd} />
    </div>
  )
}
