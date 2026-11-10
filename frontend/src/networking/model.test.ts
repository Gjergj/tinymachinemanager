import { describe, expect, it } from 'vitest'
import { attachmentsFor, explorerItems, mergeNetworkingInventory } from './model'
import { guest, link, network, snapshot } from './testFixtures'

describe('Networking relationships', () => {
  it('keeps overlapping libvirt and host names distinct and filters by source', () => {
    const data = snapshot({ networks: [network({ name: 'br0' })], interfaces: [link('br0', { isBridge: true }), link('lo', { loopback: true })] })
    const items = explorerItems(data, 'all')
    expect(items.map(item => item.key)).toEqual(['network:net-default', 'interface:br0', 'interface:lo'])
    expect(explorerItems(data, 'libvirt').map(item => item.kind)).toEqual(['network'])
    expect(explorerItems(data, 'host').map(item => item.kind)).toEqual(['interface', 'interface'])
  })

  it('lists each configured NIC referencing the selected libvirt network, including stopped guests', () => {
    const data = snapshot({ machines: [guest('multi', [
      { type: 'network', source: { network: 'default', portGroup: 'services' } },
      { type: 'network', source: { network: 'default' } },
      { type: 'bridge', source: { bridge: 'default' } },
    ], { active: false })] })
    const rows = attachmentsFor(explorerItems(data, 'libvirt')[0], data)
    expect(rows.map(row => row.index)).toEqual([0, 1])
    expect(rows[0].nic.source.portGroup).toBe('services')
  })

  it('resolves bridge references while preferring explicit actual devices over network defaults', () => {
    const data = snapshot({ machines: [guest('mixed', [
      { type: 'network', source: { network: 'default' } },
      { type: 'bridge', source: { bridge: 'virbr0' } },
      { type: 'network', source: { network: 'default' }, actual: { type: 'bridge', source: { bridge: 'other' } } },
      { type: 'network', source: { network: 'other' }, actual: { type: 'bridge', source: { bridge: 'virbr0' } } },
    ])] })
    const rows = attachmentsFor(explorerItems(data, 'host')[0], data)
    expect(rows.map(row => row.index)).toEqual([0, 1, 3])
    expect(rows.map(row => row.via)).toEqual(['Network default', 'Bridge reference', 'Resolved bridge'])
  })

  it('matches direct uplinks and live tap targets, but never stopped targets or all bridge members', () => {
    const data = snapshot({ interfaces: [link('enp1s0', { master: 'virbr0' }), link('vnet7')], machines: [
      guest('direct', [{ type: 'direct', source: { device: 'enp1s0', mode: 'bridge' } }]),
      guest('running', [{ type: 'network', source: { network: 'default' }, target: 'vnet7' }]),
      guest('stopped', [{ type: 'network', source: { network: 'default' }, target: 'vnet7' }], { active: false }),
    ] })
    const items = explorerItems(data, 'host')
    expect(attachmentsFor(items[0], data).map(row => row.machine.name)).toEqual(['direct'])
    expect(attachmentsFor(items[1], data).map(row => row.machine.name)).toEqual(['running'])
  })

  it('preserves only failed sources and replaces them on recovery, including successful empty results', () => {
    const previous = snapshot({ machines: [guest('old', [])] })
    const partial = snapshot({ networks: [], machines: [], interfaces: [link('new-link')], issues: [
      { source: 'networks', message: 'offline' }, { source: 'machines', message: 'offline' },
    ] })
    const retained = mergeNetworkingInventory(partial, previous)
    expect(retained.networks).toEqual(previous.networks)
    expect(retained.machines).toEqual(previous.machines)
    expect(retained.interfaces[0].name).toBe('new-link')
    expect(retained.issues).toEqual(partial.issues)
    const recovered = mergeNetworkingInventory(snapshot({ networks: [], machines: [], interfaces: [] }), retained)
    expect(recovered.networks).toEqual([])
    expect(recovered.machines).toEqual([])
    expect(recovered.interfaces).toEqual([])
    expect(mergeNetworkingInventory(partial, null).networks).toEqual([])
  })
})
