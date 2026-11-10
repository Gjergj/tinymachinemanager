import { useEffect, useState } from 'react'

const REFRESH_INTERVAL_MS = 5000

export function useInventory<T>(loadInventory: () => Promise<T>, merge?: (next: T, previous: T | null) => T) {
  const [inventory, setInventory] = useState<T | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [updatedAt, setUpdatedAt] = useState<Date | null>(null)

  useEffect(() => {
    let active = true
    let inFlight = false
    const refresh = async () => {
      if (!active || inFlight) return
      inFlight = true
      setLoading(true)
      try {
        const result = await loadInventory()
        if (!active) return
        setInventory(previous => merge ? merge(result, previous) : result)
        setUpdatedAt(new Date())
        setError(null)
      } catch (cause) {
        if (active) setError(cause instanceof Error ? cause.message : String(cause))
      } finally {
        inFlight = false
        if (active) setLoading(false)
      }
    }
    void refresh()
    const timer = setInterval(() => { void refresh() }, REFRESH_INTERVAL_MS)
    return () => {
      clearInterval(timer)
      // Native libvirt calls cannot be interrupted; ignore late results.
      active = false
    }
  }, [loadInventory, merge])
  return { inventory, loading, error, updatedAt }
}
