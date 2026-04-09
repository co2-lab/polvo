import iconMap from 'material-icon-theme/dist/material-icons.json'

const BASE = '/file-icons'

type IconMap = typeof iconMap

function iconUrl(name: string): string {
  return `${BASE}/${name}.svg`
}

export function getFileIconUrl(filename: string, isDir: boolean, isOpen = false): string {
  const lower = filename.toLowerCase()

  if (isDir) {
    const folderMap = isOpen
      ? (iconMap as IconMap).folderNamesExpanded
      : (iconMap as IconMap).folderNames
    const name = (folderMap as Record<string, string>)[lower]
      ?? (folderMap as Record<string, string>)[lower.replace(/^[._-]/, '')]
    if (name) return iconUrl(name)
    return iconUrl(isOpen ? (iconMap.folderExpanded ?? 'folder-open') : (iconMap.folder ?? 'folder'))
  }

  // Check full filename first (e.g. "Makefile", ".gitignore")
  const byName = (iconMap.fileNames as Record<string, string>)[lower]
  if (byName) return iconUrl(byName)

  // Then by extension — try longest match first (e.g. "spec.ts" before "ts")
  const parts = lower.split('.')
  for (let i = 1; i < parts.length; i++) {
    const ext = parts.slice(i).join('.')
    const byExt = (iconMap.fileExtensions as Record<string, string>)[ext]
    if (byExt) return iconUrl(byExt)
  }

  return iconUrl(iconMap.file ?? 'file')
}
