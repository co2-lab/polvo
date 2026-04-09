const cache = new Map<string, string>()

const IGNORE = new Set(['none', 'transparent', 'inherit', 'currentColor', '#000', '#000000', '#fff', '#ffffff'])

export async function getIconColor(iconUrl: string): Promise<string | null> {
  if (cache.has(iconUrl)) return cache.get(iconUrl)!

  try {
    const res = await fetch(iconUrl)
    const svg = await res.text()
    // Extract all fill/stroke color values
    const matches = svg.matchAll(/(?:fill|stroke)="(#[0-9a-fA-F]{3,8}|[a-z]+)"/g)
    for (const [, color] of matches) {
      if (!IGNORE.has(color.toLowerCase())) {
        cache.set(iconUrl, color)
        return color
      }
    }
  } catch {
    // ignore
  }

  cache.set(iconUrl, '')
  return null
}
