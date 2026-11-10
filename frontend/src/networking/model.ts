import type { NetworkingInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import type { Domain, Interface as GuestInterface } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import type { Interface } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/hostnet/models'
import type { IPConfig, Network, VLAN } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/networks/models'

export type Scope = 'all' | 'libvirt' | 'host'
export type ExplorerItem = { key: string; kind: 'network'; network: Network }
  | { key: string; kind: 'interface'; link: Interface }
export type GuestAttachment = { machine: Domain; nic: GuestInterface; index: number; via: string }

export function explorerItems(data: NetworkingInventory, scope: Scope): ExplorerItem[] {
  const networks: ExplorerItem[] = scope === 'host' ? [] : data.networks.map(network => ({ kind: 'network', key: 'network:' + network.uuid, network }))
  const orderedLinks = [...data.interfaces.filter(link => link.isBridge), ...data.interfaces.filter(link => !link.isBridge)]
  const links: ExplorerItem[] = scope === 'libvirt' ? [] : orderedLinks.map(link => ({ kind: 'interface', key: 'interface:' + link.name, link }))
  return [...networks, ...links]
}

function hostAssociation(link: Interface, machine: Domain, nic: GuestInterface, networks: Network[]): string | null {
  if (machine.active && nic.target === link.name) return 'Live host device'
  // Resolved device references describe the live attachment more precisely than
  // the bridge in a network definition (which may have changed independently).
  const actual = machine.active ? nic.actual?.source : undefined
  if (actual?.bridge || actual?.device) {
    if (actual.bridge === link.name) return 'Resolved bridge'
    if (actual.device === link.name) return 'Resolved device'
    return null
  }
  if (nic.source.bridge === link.name) return 'Bridge reference'
  if (nic.source.device === link.name) return 'Device reference'
  if (nic.type === 'network' && networks.some(network => network.name === nic.source.network && network.bridge === link.name)) {
    return 'Network ' + nic.source.network
  }
  return null
}

export function attachmentsFor(item: ExplorerItem, data: NetworkingInventory): GuestAttachment[] {
  return data.machines.flatMap(machine => machine.devices.interfaces.flatMap((nic, index) => {
    const via = item.kind === 'network'
      ? (nic.type === 'network' && nic.source.network === item.network.name ? 'Network reference' : null)
      : hostAssociation(item.link, machine, nic, data.networks)
    return via ? [{ machine, nic, index, via }] : []
  }))
}

export function mergeNetworkingInventory(next: NetworkingInventory, previous: NetworkingInventory | null): NetworkingInventory {
  if (!previous) return next
  const failed = new Set(next.issues.map(issue => issue.source))
  return { ...next,
    networks: failed.has('networks') ? previous.networks : next.networks,
    interfaces: failed.has('interfaces') ? previous.interfaces : next.interfaces,
    machines: failed.has('machines') ? previous.machines : next.machines,
  }
}

const modeLabels: Record<string, string> = { nat: 'NAT', isolated: 'Isolated', route: 'Routed', open: 'Open', bridge: 'Bridge', private: 'Private', vepa: 'VEPA', passthrough: 'Passthrough', hostdev: 'Host device' }
export function modeLabel(mode: string) { return modeLabels[mode] ?? (mode || 'Not reported') }
export function linkKind(link: Interface) { return link.isBridge ? 'Linux bridge' : link.loopback ? 'Loopback' : 'Host interface' }
export function addressLabel(ip: IPConfig) {
  return ip.address + (ip.prefix != null ? '/' + ip.prefix : ip.netmask ? ' (' + ip.netmask + ')' : '')
}
export function vlanLabel(vlan: VLAN | null | undefined) {
  if (!vlan) return 'Not specified'
  return vlan.tags.map(tag => 'VLAN ' + tag.id + (tag.nativeMode ? ' (' + tag.nativeMode + ')' : '')).join(', ') + (vlan.trunk ? ' · trunk' : '')
}
