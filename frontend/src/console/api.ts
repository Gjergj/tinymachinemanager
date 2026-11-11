import { ConsoleService } from '../../bindings/github.com/Gjergj/tinymachinemanager'
import type { Credentials } from './connection'

export interface ConsoleDevice { key: string; label: string; kind: string; backend: string; alias: string; supported: boolean; reason: string }
export interface ConsoleCapabilities { uuid: string; name: string; active: boolean; state: string; available: boolean; code: string; reason: string; revision: string; devices: ConsoleDevice[] }
export interface ConsoleAPI {
  capabilities(uuid: string): Promise<ConsoleCapabilities>
  create(uuid: string, key: string, revision: string): Promise<Credentials>
  disconnect(id: string): Promise<void>
}
export const consoleAPI: ConsoleAPI = {
  capabilities: uuid => ConsoleService.GetCapabilities(uuid),
  create: (uuid, key, revision) => ConsoleService.CreateSession(uuid, key, revision),
  disconnect: id => ConsoleService.Disconnect(id),
}
export const openSerialConsole = (uuid: string) => ConsoleService.OpenWindow(uuid)
