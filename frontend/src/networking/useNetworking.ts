import type { NetworkingInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import { useInventory } from '../shared/useInventory'
import { mergeNetworkingInventory } from './model'

export type LoadNetworking = () => Promise<NetworkingInventory>
export function useNetworking(loadNetworking: LoadNetworking) {
  return useInventory(loadNetworking, mergeNetworkingInventory)
}
