import { MachinesService } from '../../bindings/github.com/Gjergj/tinymachinemanager'
import type { LoadMachines } from './useMachines'

export const loadMachines: LoadMachines = async () => {
  const inventory = await MachinesService.GetInventory()
  if (!inventory) throw new Error('Libvirt returned no inventory.')
  return inventory
}

export type EnableMemoryReporting = (uuid: string) => Promise<void>
export const enableMemoryReporting: EnableMemoryReporting = uuid => MachinesService.EnableMemoryReporting(uuid)
