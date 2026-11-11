import { useCallback, useEffect, useRef, useState } from 'react'
import type { EnableMemoryReporting } from './api'

export type MemoryReportingState = { pending: boolean; error: string | null }

// Action state belongs to the inventory view so selecting another VM cannot
// orphan a pending write or discard its eventual error.
export function useMemoryReporting(action: EnableMemoryReporting | undefined, refresh: () => Promise<void>) {
  const [states, setStates] = useState<Record<string, MemoryReportingState>>({})
  const requests = useRef(new Map<string, Promise<void>>())
  const alive = useRef(true)
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const enable = useCallback((uuid: string): Promise<void> => {
    if (!action || !alive.current) return Promise.resolve()
    const existing = requests.current.get(uuid)
    if (existing) return existing
    setStates(previous => ({ ...previous, [uuid]: { pending: true, error: null } }))
    const request = Promise.resolve().then(async () => { await action(uuid); await refresh() })
      .catch(cause => {
        if (alive.current) setStates(previous => ({ ...previous, [uuid]: { pending: false, error: cause instanceof Error ? cause.message : String(cause) } }))
        throw cause
      }).finally(() => {
        requests.current.delete(uuid)
        if (alive.current) setStates(previous => ({ ...previous, [uuid]: { ...previous[uuid], pending: false } }))
      })
    requests.current.set(uuid, request)
    return request
  }, [action, refresh])
  return { states, enable: action ? enable : undefined }
}
