import type { MachineInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import { useInventory } from '../shared/useInventory'

export type LoadMachines = () => Promise<MachineInventory>

export function useMachines(loadMachines: LoadMachines) {
  const { inventory, loading, error, updatedAt } = useInventory(loadMachines)
  return { machines: inventory?.machines ?? [], host: inventory?.host ?? null, loading, error, updatedAt }
}
