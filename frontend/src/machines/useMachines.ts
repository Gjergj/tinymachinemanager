import { useEffect, useState } from 'react'
import type { MachineInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'

export type LoadMachines = () => Promise<MachineInventory>
const REFRESH_INTERVAL_MS = 5000

export function useMachines(loadMachines: LoadMachines) {
  const [inventory, setInventory] = useState<MachineInventory | null>(null)
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
        const result = await loadMachines()
        if (!active) return
        setInventory(result)
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
  }, [loadMachines])
  return { machines: inventory?.machines ?? [], host: inventory?.host ?? null, loading, error, updatedAt }
}
