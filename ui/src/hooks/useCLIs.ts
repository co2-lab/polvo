import { useState, useEffect } from 'react'

export interface CLIInfo {
  id: string
  label: string
  command: string
}

export function useCLIs(): CLIInfo[] {
  const [clis, setCLIs] = useState<CLIInfo[]>([])

  useEffect(() => {
    fetch('/api/clis')
      .then((r) => r.ok ? r.json() : [])
      .then((data) => setCLIs(Array.isArray(data) ? data : []))
      .catch(() => setCLIs([]))
  }, [])

  return clis
}
