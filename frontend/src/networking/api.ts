import { NetworkingService } from '../../bindings/github.com/Gjergj/tinymachinemanager'
import type { LoadNetworking } from './useNetworking'

export const loadNetworking: LoadNetworking = () => NetworkingService.GetInventory()
