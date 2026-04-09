import { useEffect, useState } from 'react'
import { useIDEStore } from '../../store/useIDEStore'

const SUPPORTED_LOCALES = ['en-US', 'pt-BR'] as const
type Locale = typeof SUPPORTED_LOCALES[number]

type Dict = Record<string, string>

const cache: Partial<Record<Locale, Dict>> = {}

async function loadLocale(locale: Locale): Promise<Dict> {
  if (cache[locale]) return cache[locale]!
  const mod = await import(`./locales/${locale}.json`)
  cache[locale] = mod.default as Dict
  return cache[locale]!
}

// Preload en-US synchronously so the first render never flickers
import enUS from './locales/en-US.json'
cache['en-US'] = enUS as Dict

export function useT() {
  const language = useIDEStore(s => s.generalSettings.language)
  const locale: Locale = SUPPORTED_LOCALES.includes(language as Locale)
    ? (language as Locale)
    : 'en-US'

  const [dict, setDict] = useState<Dict>(() => cache[locale] ?? cache['en-US']!)

  useEffect(() => {
    if (cache[locale]) {
      setDict(cache[locale]!)
      return
    }
    loadLocale(locale).then(setDict)
  }, [locale])

  return (key: string) => dict[key] ?? cache['en-US']![key] ?? key
}
