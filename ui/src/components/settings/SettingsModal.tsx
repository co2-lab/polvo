import { X, Settings as SettingsIcon, Palette, Keyboard, Search, AlertCircle, Info } from 'lucide-react'
import { useIDEStore } from '../../store/useIDEStore'
import { useState, useEffect } from 'react'
import { useT } from '../../lib/i18n'
import { clsx } from 'clsx'
import type { StatusResponse } from '../../types/api'
import { FloatingModal } from '../common/FloatingModal'

function ThemePreview({ theme }: { theme: { colors: { bg: string; surface: string; border: string; accent: string; text: string } } }) {
  return (
    <div
      className="w-full h-32 rounded-lg border overflow-hidden flex flex-col shadow-sm transition-transform hover:scale-105 cursor-pointer"
      style={{ backgroundColor: theme.colors.bg, borderColor: theme.colors.border }}
    >
      <div className="h-4 w-full flex items-center px-2 gap-1" style={{ backgroundColor: theme.colors.surface }}>
        <div className="w-2 h-2 rounded-full bg-red-500/50" />
        <div className="w-2 h-2 rounded-full bg-yellow-500/50" />
        <div className="w-2 h-2 rounded-full bg-green-500/50" />
      </div>
      <div className="flex-1 flex">
        <div className="w-1/4 h-full border-r" style={{ backgroundColor: theme.colors.surface, borderColor: theme.colors.border }}>
          <div className="w-3/4 h-2 rounded mt-2 ml-2 opacity-50" style={{ backgroundColor: theme.colors.text }} />
          <div className="w-1/2 h-2 rounded mt-2 ml-2 opacity-30" style={{ backgroundColor: theme.colors.text }} />
        </div>
        <div className="flex-1 p-2 flex flex-col gap-2">
          <div className="w-1/3 h-2 rounded opacity-80" style={{ backgroundColor: theme.colors.accent }} />
          <div className="w-full h-2 rounded opacity-40" style={{ backgroundColor: theme.colors.text }} />
          <div className="w-5/6 h-2 rounded opacity-40" style={{ backgroundColor: theme.colors.text }} />
          <div className="w-4/6 h-2 rounded opacity-40" style={{ backgroundColor: theme.colors.text }} />
        </div>
      </div>
      <div className="h-6 w-full flex justify-center items-end pb-1">
        <div
          className="w-1/3 h-4 rounded-full flex justify-center items-center gap-1 px-1 border"
          style={{ backgroundColor: theme.colors.surface, borderColor: theme.colors.border }}
        >
          <div className="w-2 h-2 rounded-sm opacity-60" style={{ backgroundColor: theme.colors.text }} />
          <div className="w-2 h-2 rounded-sm opacity-60" style={{ backgroundColor: theme.colors.text }} />
          <div className="w-2 h-2 rounded-sm opacity-60" style={{ backgroundColor: theme.colors.text }} />
        </div>
      </div>
    </div>
  )
}

export function SettingsModal() {
  const t = useT()
  const {
    isSettingsOpen, setSettingsOpen,
    themes, activeThemeId, setActiveTheme,
    generalSettings, updateGeneralSettings,
    shortcuts, updateShortcut,
    dockPosition, setDockPosition,
  } = useIDEStore()
  const [activeTab, setActiveTab] = useState<'general' | 'themes' | 'shortcuts' | 'info'>('general')
  const [serverInfo, setServerInfo] = useState<StatusResponse | null>(null)

  useEffect(() => {
    if (activeTab !== 'info') return
    fetch('/api/status')
      .then(r => r.ok ? r.json() : null)
      .then(d => setServerInfo(d))
      .catch(() => setServerInfo(null))
  }, [activeTab])
  const [shortcutSearch, setShortcutSearch] = useState('')
  const [recordingShortcutId, setRecordingShortcutId] = useState<string | null>(null)

  const handleKeyDown = (e: React.KeyboardEvent, id: string) => {
    e.preventDefault()
    e.stopPropagation()

    if (e.key === 'Escape') {
      setRecordingShortcutId(null)
      return
    }

    const keys: string[] = []
    if (e.ctrlKey || e.metaKey) keys.push('Ctrl')
    if (e.altKey) keys.push('Alt')
    if (e.shiftKey) keys.push('Shift')

    if (['Control', 'Alt', 'Shift', 'Meta'].includes(e.key)) return

    keys.push(e.key.toUpperCase())
    updateShortcut(id, keys)
    setRecordingShortcutId(null)
  }

  const hasConflict = (keys: string[], currentId: string) => {
    const keyString = keys.join('+')
    return shortcuts.some(s => s.id !== currentId && s.keys.join('+') === keyString)
  }

  const filteredShortcuts = shortcuts.filter(s =>
    s.name.toLowerCase().includes(shortcutSearch.toLowerCase()) ||
    s.keys.join(' ').toLowerCase().includes(shortcutSearch.toLowerCase())
  )

  const groupedShortcuts = filteredShortcuts.reduce((acc, shortcut) => {
    if (!acc[shortcut.context]) acc[shortcut.context] = []
    acc[shortcut.context].push(shortcut)
    return acc
  }, {} as Record<string, typeof shortcuts>)

  return (
    <FloatingModal
      open={isSettingsOpen}
      onClose={() => setSettingsOpen(false)}
      initialWidth={860}
      initialHeight={620}
      minWidth={600}
      minHeight={400}
    >
      {({ dragHandleProps }) => (
        <div className="flex flex-1 overflow-hidden">
          {/* Sidebar */}
          <div className="w-56 bg-black/20 border-r border-white/10 p-4 flex flex-col gap-1 shrink-0">
            <div
              className="text-sm font-medium text-white/90 mb-4 px-2 cursor-grab"
              {...dragHandleProps}
            >
              {t('settings.title')}
            </div>

            {([
              { id: 'general',   label: t('settings.tab.general'),   Icon: SettingsIcon },
              { id: 'shortcuts', label: t('settings.tab.shortcuts'),  Icon: Keyboard },
              { id: 'themes',    label: t('settings.tab.themes'),     Icon: Palette },
              { id: 'info',      label: t('settings.tab.info'),       Icon: Info },
            ] as const).map(({ id, label, Icon }) => (
              <button
                key={id}
                onMouseDown={e => e.stopPropagation()}
                onClick={() => setActiveTab(id)}
                className={clsx(
                  'flex items-center gap-3 px-3 py-2 rounded-lg transition-colors text-sm font-medium',
                  activeTab === id ? 'bg-white/10 text-white' : 'text-white/50 hover:bg-white/5 hover:text-white/90'
                )}
              >
                <Icon className="w-4 h-4" />
                {label}
              </button>
            ))}
          </div>

          {/* Content */}
          <div className="flex-1 flex flex-col overflow-hidden">
            <div
              className="h-12 border-b border-white/10 flex items-center justify-between px-4 shrink-0 cursor-grab"
              {...dragHandleProps}
            >
              <h3 className="text-sm font-medium text-white capitalize">{activeTab}</h3>
              <button
                onMouseDown={e => e.stopPropagation()}
                onClick={() => setSettingsOpen(false)}
                className="p-1.5 text-white/50 hover:text-white hover:bg-white/10 rounded-lg transition-colors"
              >
                <X className="w-4 h-4" />
              </button>
            </div>

            <div className="flex-1 overflow-y-auto p-6">
              {activeTab === 'general' && (
                <div className="space-y-6 max-w-2xl">
                  <div className="space-y-4">
                    <h4 className="text-sm font-medium text-white/90">{t('settings.general.application')}</h4>

                    <div className="flex items-center justify-between py-2 border-b border-white/5">
                      <div>
                        <div className="text-sm text-white/80">{t('settings.general.language')}</div>
                        <div className="text-xs text-white/40">{t('settings.general.language.desc')}</div>
                      </div>
                      <select
                        value={generalSettings.language}
                        onChange={(e) => updateGeneralSettings({ language: e.target.value })}
                        className="bg-black/20 border border-white/10 rounded-lg px-3 py-1.5 text-sm text-white/80 outline-none"
                      >
                        <option value="en-US">English (US)</option>
                        <option value="pt-BR">Português (BR)</option>
                      </select>
                    </div>

                    <div className="flex items-center justify-between py-2 border-b border-white/5">
                      <div>
                        <div className="text-sm text-white/80">{t('settings.general.dockPosition')}</div>
                        <div className="text-xs text-white/40">{t('settings.general.dockPosition.desc')}</div>
                      </div>
                      <select
                        value={dockPosition.edge}
                        onChange={(e) => setDockPosition({ ...dockPosition, edge: e.target.value as 'bottom' | 'top' | 'left' | 'right' })}
                        className="bg-black/20 border border-white/10 rounded-lg px-3 py-1.5 text-sm text-white/80 outline-none"
                      >
                        <option value="bottom">Bottom</option>
                        <option value="left">Left</option>
                        <option value="right">Right</option>
                        <option value="top">Top</option>
                      </select>
                    </div>

                    <div className="flex items-center justify-between py-2 border-b border-white/5">
                      <div>
                        <div className="text-sm text-white/80">{t('settings.general.restoreLayout')}</div>
                        <div className="text-xs text-white/40">{t('settings.general.restoreLayout.desc')}</div>
                      </div>
                      <label className="relative inline-flex items-center cursor-pointer">
                        <input
                          type="checkbox"
                          checked={generalSettings.restoreLastLayout}
                          onChange={(e) => updateGeneralSettings({ restoreLastLayout: e.target.checked })}
                          className="sr-only peer"
                        />
                        <div className="w-9 h-5 bg-white/10 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-white/60" />
                      </label>
                    </div>

                    <div className="flex items-center justify-between py-2 border-b border-white/5">
                      <div>
                        <div className="text-sm text-white/80">{t('settings.general.confirmClose')}</div>
                        <div className="text-xs text-white/40">{t('settings.general.confirmClose.desc')}</div>
                      </div>
                      <label className="relative inline-flex items-center cursor-pointer">
                        <input
                          type="checkbox"
                          checked={generalSettings.confirmOnClose}
                          onChange={(e) => updateGeneralSettings({ confirmOnClose: e.target.checked })}
                          className="sr-only peer"
                        />
                        <div className="w-9 h-5 bg-white/10 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-white/60" />
                      </label>
                    </div>

                    <div className="flex items-center justify-between py-2 border-b border-white/5">
                      <div>
                        <div className="text-sm text-white/80">{t('settings.general.autoDetect')}</div>
                        <div className="text-xs text-white/40">{t('settings.general.autoDetect.desc')}</div>
                      </div>
                      <label className="relative inline-flex items-center cursor-pointer">
                        <input
                          type="checkbox"
                          checked={generalSettings.autoDetectCLIs}
                          onChange={(e) => updateGeneralSettings({ autoDetectCLIs: e.target.checked })}
                          className="sr-only peer"
                        />
                        <div className="w-9 h-5 bg-white/10 peer-focus:outline-none rounded-full peer peer-checked:after:translate-x-full peer-checked:after:border-white after:content-[''] after:absolute after:top-[2px] after:left-[2px] after:bg-white after:border-gray-300 after:border after:rounded-full after:h-4 after:w-4 after:transition-all peer-checked:bg-white/60" />
                      </label>
                    </div>
                  </div>

                  <div className="space-y-4 pt-4">
                    <h4 className="text-sm font-medium text-white/90">{t('settings.general.editor')}</h4>

                    <div className="flex items-center justify-between py-2 border-b border-white/5">
                      <div>
                        <div className="text-sm text-white/80">{t('settings.general.fontFamily')}</div>
                        <div className="text-xs text-white/40">{t('settings.general.fontFamily.desc')}</div>
                      </div>
                      <input
                        type="text"
                        value={generalSettings.editorFontFamily}
                        onChange={(e) => updateGeneralSettings({ editorFontFamily: e.target.value })}
                        className="bg-black/20 border border-white/10 rounded-lg px-3 py-1.5 text-sm text-white/80 outline-none w-48"
                      />
                    </div>

                    <div className="flex items-center justify-between py-2 border-b border-white/5">
                      <div>
                        <div className="text-sm text-white/80">{t('settings.general.fontSize')}</div>
                        <div className="text-xs text-white/40">{t('settings.general.fontSize.desc')}</div>
                      </div>
                      <div className="flex items-center gap-3">
                        <input
                          type="range"
                          min="10"
                          max="24"
                          value={generalSettings.editorFontSize}
                          onChange={(e) => updateGeneralSettings({ editorFontSize: parseInt(e.target.value) })}
                          className="w-24"
                        />
                        <span className="text-sm text-white/60 w-6 text-right">{generalSettings.editorFontSize}</span>
                      </div>
                    </div>
                  </div>
                </div>
              )}

              {activeTab === 'shortcuts' && (
                <div className="space-y-6 max-w-3xl">
                  <div className="relative">
                    <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-4 h-4 text-white/40" />
                    <input
                      type="text"
                      placeholder={t('settings.shortcuts.search')}
                      value={shortcutSearch}
                      onChange={(e) => setShortcutSearch(e.target.value)}
                      className="w-full bg-black/20 border border-white/10 rounded-lg pl-9 pr-4 py-2 text-sm text-white/80 outline-none"
                    />
                  </div>

                  <div className="space-y-8">
                    {Object.entries(groupedShortcuts).map(([context, contextShortcuts]) => (
                      <div key={context} className="space-y-3">
                        <h4 className="text-xs font-medium text-white/40 uppercase tracking-wider">{context}</h4>
                        <div className="bg-black/20 border border-white/5 rounded-xl overflow-hidden">
                          {contextShortcuts.map((shortcut, idx) => {
                            const isRecording = recordingShortcutId === shortcut.id
                            const conflict = !isRecording && hasConflict(shortcut.keys, shortcut.id)

                            return (
                              <div
                                key={shortcut.id}
                                className={clsx(
                                  'flex items-center justify-between p-3',
                                  idx !== contextShortcuts.length - 1 && 'border-b border-white/5',
                                  isRecording && 'bg-white/5'
                                )}
                              >
                                <div className="text-sm text-white/80">{shortcut.name}</div>
                                <div className="flex items-center gap-3">
                                  {conflict && (
                                    <div className="flex items-center gap-1 text-red-400 text-xs bg-red-400/10 px-2 py-1 rounded-md">
                                      <AlertCircle className="w-3 h-3" />
                                      {t('settings.shortcuts.conflict')}
                                    </div>
                                  )}
                                  <button
                                    onClick={() => setRecordingShortcutId(isRecording ? null : shortcut.id)}
                                    onKeyDown={(e) => isRecording && handleKeyDown(e, shortcut.id)}
                                    className={clsx(
                                      'flex items-center gap-1 px-3 py-1.5 rounded-md text-xs font-mono min-w-[100px] justify-center transition-colors border',
                                      isRecording
                                        ? 'bg-white/20 text-white border-white/30 animate-pulse'
                                        : 'bg-black/40 text-white/60 border-white/10 hover:bg-white/10 hover:text-white/90'
                                    )}
                                  >
                                    {isRecording ? t('settings.shortcuts.press') : shortcut.keys.join(' + ')}
                                  </button>
                                </div>
                              </div>
                            )
                          })}
                        </div>
                      </div>
                    ))}

                    {Object.keys(groupedShortcuts).length === 0 && (
                      <div className="text-center py-12 text-white/40 text-sm">
                        {`${t('settings.shortcuts.noResults')} "${shortcutSearch}"`}
                      </div>
                    )}
                  </div>
                </div>
              )}

              {activeTab === 'themes' && (
                <div className="space-y-8">
                  <section>
                    <h4 className="text-xs font-medium text-white/40 uppercase tracking-wider mb-4">{t('settings.themes.predefined')}</h4>
                    <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                      {themes.filter(th => th.type === 'predefined').map(theme => {
                        const isActive = activeThemeId === theme.id
                        return (
                          <div key={theme.id} className="flex flex-col gap-3">
                            <div
                              onClick={() => setActiveTheme(theme.id)}
                              className="rounded-lg transition-all"
                              style={isActive ? {
                                outline: `2px solid ${theme.colors.accent}`,
                                outlineOffset: '3px',
                              } : undefined}
                            >
                              <ThemePreview theme={theme} />
                            </div>
                            <div className="flex items-center justify-between px-1">
                              <span className="text-sm font-medium text-white/80">{theme.name}</span>
                              {isActive && (
                                <span
                                  className="text-xs px-2 py-1 rounded-full"
                                  style={{ backgroundColor: `${theme.colors.accent}22`, color: theme.colors.accent }}
                                >
                                  {t('settings.themes.active')}
                                </span>
                              )}
                            </div>
                          </div>
                        )
                      })}
                    </div>
                  </section>

                  <section>
                    <h4 className="text-xs font-medium text-white/40 uppercase tracking-wider mb-4">{t('settings.themes.marketplace')}</h4>
                    <div className="border border-dashed border-white/10 rounded-xl p-8 flex flex-col items-center justify-center text-center">
                      <Palette className="w-8 h-8 text-white/20 mb-3" />
                      <p className="text-white/50 text-sm">{t('settings.themes.marketplace.desc')}</p>
                      <button className="mt-4 px-4 py-2 bg-white/10 hover:bg-white/20 text-white text-sm font-medium rounded-lg transition-colors">
                        {t('settings.themes.marketplace.browse')}
                      </button>
                    </div>
                  </section>
                </div>
              )}

              {activeTab === 'info' && (
                <div className="space-y-6 max-w-2xl">
                  <div className="space-y-1">
                    <h4 className="text-xs font-medium text-white/40 uppercase tracking-wider mb-4">{t('settings.info.server')}</h4>
                    {serverInfo ? (
                      <div className="bg-black/20 border border-white/5 rounded-xl overflow-hidden">
                        {[
                          { label: t('settings.info.version'),    value: serverInfo.version },
                          { label: t('settings.info.commit'),     value: serverInfo.commit_sha ?? '—' },
                          { label: t('settings.info.buildDate'), value: serverInfo.build_date ?? '—' },
                          { label: t('settings.info.workingDir'), value: serverInfo.cwd },
                        ].map((row, i, arr) => (
                          <div key={row.label} className={clsx('flex items-center justify-between px-4 py-2.5', i < arr.length - 1 && 'border-b border-white/5')}>
                            <span className="text-sm text-white/50">{row.label}</span>
                            <span className="text-xs font-mono text-white/80 truncate max-w-xs text-right">{row.value}</span>
                          </div>
                        ))}
                      </div>
                    ) : (
                      <div className="text-sm text-white/30 py-4">{t('settings.info.unreachable')}</div>
                    )}
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}
    </FloatingModal>
  )
}
