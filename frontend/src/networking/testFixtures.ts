import type { NetworkingInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import type { Domain, Interface as GuestInterface } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import type { Interface } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/hostnet/models'
import type { Network } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/networks/models'

export function network(extra: Partial<Network> = {}): Network {
  return { uuid: 'net-default', name: 'default', active: true, persistent: true, autostart: true,
    bridge: 'virbr0', forwardMode: 'nat', forwardInterfaces: [], portGroups: [], ips: [], ...extra }
}
export function link(name: string, extra: Partial<Interface> = {}): Interface {
  return { name, index: 1, mtu: 1500, up: true, running: true, loopback: false,
    operState: 'up', isBridge: false, members: [], addresses: [], ...extra }
}
export function guest(uuid: string, interfaces: GuestInterface[], extra: Partial<Domain> = {}): Domain {
  return { uuid, name: uuid, type: '', os: { type: {} }, devices: { serials: [], consoles: [], interfaces }, active: true, state: 'running' as Domain['state'], persistent: true,
    vcpus: 2, memoryKiB: 4194304, maxMemoryKiB: 4194304, ...extra }
}
export function snapshot(extra: Partial<NetworkingInventory> = {}): NetworkingInventory {
  return { networks: [network()], interfaces: [link('virbr0', { isBridge: true })], machines: [], issues: [], ...extra }
}
