import type { Theme } from '../../themes/types'

interface ThemeThumbnailProps {
  theme: Theme
}

export function ThemeThumbnail({ theme }: ThemeThumbnailProps) {
  const v = theme.variables
  // Support both new token names and backward-compat aliases
  const bg        = v['--surface-base']    ?? v['--bg']         ?? '#0d1210'
  const bgDark    = v['--surface-low']     ?? v['--bg-dark']    ?? '#0a0e0c'
  const titleBar  = v['--surface-low']     ?? v['--title-bar']  ?? '#0a0e0c'
  const border    = v['--border']                               ?? '#1a3a2a'
  const text      = v['--text-primary']    ?? v['--text']       ?? '#e0e0e0'
  const muted     = v['--text-muted']      ?? v['--muted']      ?? '#4a6a5a'
  const primary   = v['--mint']            ?? v['--primary']    ?? '#00ffab'
  const codeBg    = v['--code-bg']                              ?? '#0f1a14'
  const accent    = v['--rose']            ?? v['--accent']     ?? '#ff5e8e'
  const success   = v['--status-success']  ?? v['--success']    ?? '#7fd88f'

  return (
    <svg
      width="200"
      height="96"
      viewBox="0 0 200 96"
      xmlns="http://www.w3.org/2000/svg"
      className="theme-card-thumbnail"
      style={{ display: 'block' }}
    >
      {/* Background */}
      <rect width="200" height="96" fill={bg} />

      {/* Title bar */}
      <rect x="0" y="0" width="200" height="14" fill={titleBar} />
      <circle cx="12" cy="7" r="3" fill={primary} opacity="0.8" />
      <text x="22" y="10" fill={primary} fontSize="6" fontFamily="monospace" fontWeight="600">polvo</text>
      {/* Workspace tabs */}
      <rect x="60" y="2" width="36" height="10" rx="2" fill={bg} />
      <rect x="60" y="2" width="36" height="2" fill={primary} />
      <text x="64" y="10" fill={text} fontSize="5" fontFamily="monospace">Workspace 1</text>
      <rect x="100" y="2" width="28" height="10" rx="2" fill="transparent" />
      <text x="103" y="10" fill={muted} fontSize="5" fontFamily="monospace">Work 2</text>

      {/* Left sidebar */}
      <rect x="0" y="14" width="38" height="68" fill={bgDark} />
      <rect x="38" y="14" width="0.5" height="68" fill={border} />
      <text x="5" y="24" fill={muted} fontSize="4.5" fontFamily="monospace" textAnchor="start">EXPLORER</text>
      <text x="7" y="34" fill={muted} fontSize="4" fontFamily="monospace">▶ /src</text>
      <text x="10" y="43" fill={text} fontSize="4" fontFamily="monospace">app.go</text>
      <text x="10" y="51" fill={muted} fontSize="4" fontFamily="monospace">main.go</text>
      <text x="10" y="59" fill={muted} fontSize="4" fontFamily="monospace">utils.go</text>

      {/* Editor panel */}
      <rect x="38" y="14" width="162" height="68" fill={codeBg} />
      {/* Editor tab bar */}
      <rect x="38" y="14" width="162" height="12" fill={titleBar} />
      <rect x="38" y="14" width="40" height="12" fill={bg} />
      <rect x="38" y="14" width="40" height="2" fill={primary} />
      <text x="42" y="23" fill={text} fontSize="5" fontFamily="monospace">app.go</text>
      <text x="82" y="23" fill={muted} fontSize="5" fontFamily="monospace">main.go ×</text>
      {/* Code lines */}
      <text x="44" y="38" fill={muted} fontSize="4" fontFamily="monospace">1</text>
      <text x="52" y="38" fill={success} fontSize="4" fontFamily="monospace">func</text>
      <text x="68" y="38" fill={text} fontSize="4" fontFamily="monospace">main() {'{'}</text>
      <text x="44" y="47" fill={muted} fontSize="4" fontFamily="monospace">2</text>
      <text x="56" y="47" fill={primary} fontSize="4" fontFamily="monospace">fmt</text>
      <text x="68" y="47" fill={text} fontSize="4" fontFamily="monospace">.Println(...)</text>
      <text x="44" y="56" fill={muted} fontSize="4" fontFamily="monospace">3</text>
      <text x="52" y="56" fill={text} fontSize="4" fontFamily="monospace">{'}'}</text>
      {/* Cursor */}
      <rect x="56" y="49" width="1.5" height="7" fill={primary} opacity="0.8" />

      {/* Status bar */}
      <rect x="0" y="82" width="200" height="7" fill={bgDark} opacity="0.8" />
      <text x="4" y="88" fill={muted} fontSize="4" fontFamily="monospace">idle</text>
      <text x="160" y="88" fill={text} fontSize="4" fontFamily="monospace">project</text>

      {/* Dock */}
      <rect x="0" y="89" width="200" height="7" fill={bgDark} />
      {[5, 14, 23, 32, 41, 50].map((x, i) => (
        <rect
          key={i}
          x={x}
          y="91"
          width="6"
          height="4"
          rx="1"
          fill={i === 0 ? primary : border}
          opacity={i === 0 ? 0.9 : 0.5}
        />
      ))}

      {/* Use accent to avoid unused variable warning */}
      <rect width="0" height="0" fill={accent} />
    </svg>
  )
}
