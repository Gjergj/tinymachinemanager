import { useCallback, useEffect, useRef, useState } from 'react'

const REFRESH_INTERVAL_MS = 5000
const noRefresh = async () => {}

export function useInventory<T>(loadInventory: () => Promise<T>, merge?: (next: T, previous: T | null) => T) {
  const [inventory, setInventory] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)
  const refreshRef = useRef<() => Promise<void>>(noRefresh)
  const refreshNow = useCallback(() => refreshRef.current(), [])

  useEffect(() => {
    let active = true
    let inFlight: Promise<void> | null = null
    const refresh = (): Promise<void> => {
      if (!active) return Promise.resolve()
      if (inFlight) return inFlight
      setLoading(true)
      // Start in a microtask so even a synchronous loader error cannot leave
      // an already-finished promise registered as the in-flight request.
      inFlight = Promise.resolve().then(async () => {
        if (!active) return
        try {
          const result = await loadInventory()
          if (!active) return
          setInventory(previous => merge ? merge(result, previous) : result)
          setUpdatedAt(new Date())
          setError(null)
        } catch (cause) {
          if (active) setError(cause instanceof Error ? cause.message : String(cause))
        } finally {
          if (active) setLoading(false)
        }
      }).finally(() => { inFlight = null })
      return inFlight
    }
    refreshRef.current = async () => {
      // A configuration change needs a read started after the write. Wait for
      // any older read, then refresh; ordinary polling ticks never queue work.
      if (inFlight) await inFlight
      if (active) await refresh()
    }
    void refresh()
    const timer = setInterval(() => { void refresh() }, REFRESH_INTERVAL_MS)
    return () => {
      clearInterval(timer)
      // Native libvirt calls cannot be interrupted; ignore late results.
      active = false
      refreshRef.current = noRefresh
    }
  }, [loadInventory, merge])
  return { inventory, loading, error, updatedAt, refresh: refreshNow }
}
