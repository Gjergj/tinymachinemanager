import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { NetworkingInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import type { Domain } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import { NetworkingView } from './NetworkingView'
import { guest, link, network, snapshot } from './testFixtures'

const inventory = snapshot({
  networks: [network({ dnsDomain: 'guests.local', ips: [{ family: 'ipv4', address: '192.168.122.1', prefix: 24,
    dhcp: { ranges: [{ start: '192.168.122.100', end: '192.168.122.200' }], hosts: [{ name: 'reserved', ip: '192.168.122.10' }] } }] }),
    network({ uuid: 'net-lab', name: 'lab', bridge: 'virbr10', active: false, forwardMode: 'isolated', portIsolated: 'yes',
      portGroups: [{ name: 'services', default: true, vlan: { trunk: false, tags: [{ id: 20 }] } }] })],
  interfaces: [link('virbr0', { isBridge: true, addresses: ['192.168.122.1/24'], members: ['enp1s0', 'vnet0'] }),
    link('enp1s0', { master: 'virbr0', mac: '00:11:22:33:44:55', addresses: ['10.0.0.2/24'] }),
    link('vnet0', { master: 'virbr0' }), link('lo', { loopback: true, operState: 'unknown', addresses: ['127.0.0.1/8'] })],
  machines: [guest('atlas-web', [{ type: 'network', source: { network: 'default' }, mac: '52:54:00:00:00:01', target: 'vnet0' }]),
    guest('lab-test', [{ type: 'network', source: { network: 'lab', portGroup: 'services' }, vlan: { trunk: false, tags: [{ id: 20 }] } }], { active: false, state: 'shutoff' as Domain['state'] })],
})

async function show(load = vi.fn().mockResolvedValue(inventory)) {
  const view = render(<NetworkingView loadNetworking={load} />)
  await act(async () => {})
  return { ...view, load }
}
async function advance(milliseconds = 5000) { await act(async () => { await vi.advanceTimersByTimeAsync(milliseconds) }) }
function details() { return screen.getByRole('region', { name: 'Selected network' }) }

describe('Networking explorer', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => { cleanup(); vi.useRealTimers() })

  it('groups local links and libvirt networks and shows configuration and NIC references', async () => {
    await show()
    expect(screen.getByRole('heading', { name: /Libvirt networks/ })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Linux bridges/ })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: /Host interfaces/ })).toBeInTheDocument()
    expect(within(details()).getByRole('heading', { name: 'default' })).toBeInTheDocument()
    expect(within(details()).getByText('192.168.122.1/24')).toBeInTheDocument()
    expect(within(details()).getByText('192.168.122.100 – 192.168.122.200')).toBeInTheDocument()
    expect(within(details()).getByText('guests.local')).toBeInTheDocument()
    const table = screen.getByRole('table', { name: 'Guest network attachments' })
    expect(within(table).getByText('atlas-web')).toBeInTheDocument()
    expect(within(table).getByText('52:54:00:00:00:01')).toBeInTheDocument()
    expect(within(table).getByText('vnet0')).toBeInTheDocument()
    expect(within(table).queryByText('lab-test')).not.toBeInTheDocument()
  })

  it('includes inactive networks and stopped guests, port groups and isolation settings', async () => {
    await show()
    fireEvent.click(screen.getByRole('button', { name: /^lab\b/ }))
    expect(within(details()).getByText('Inactive')).toBeInTheDocument()
    expect(within(details()).getByText('Isolated')).toBeInTheDocument()
    expect(within(details()).getByText('Port isolation').nextElementSibling).toHaveTextContent('Enabled')
    expect(within(details()).getByRole('heading', { name: 'Port groups' })).toBeInTheDocument()
    const table = screen.getByRole('table', { name: 'Guest network attachments' })
    expect(within(table).getByText('lab-test')).toBeInTheDocument()
    expect(within(table).getByText('Shut off')).toBeInTheDocument()
    expect(within(table).getByText('services')).toBeInTheDocument()
    expect(within(table).getByText('VLAN 20')).toBeInTheDocument()
  })

  it('filters host links and follows bridge members and their master without losing the source distinction', async () => {
    await show()
    fireEvent.click(screen.getByRole('tab', { name: /^Host/ }))
    expect(within(screen.getByRole('complementary', { name: 'Network picker' })).queryByRole('button', { name: /^default\b/ })).not.toBeInTheDocument()
    expect(within(details()).getByRole('heading', { name: 'virbr0' })).toBeInTheDocument()
    expect(within(details()).getByText('Network default')).toBeInTheDocument()
    fireEvent.click(within(details()).getByRole('button', { name: 'enp1s0' }))
    expect(within(details()).getByRole('heading', { name: 'enp1s0' })).toBeInTheDocument()
    expect(within(details()).getByText('00:11:22:33:44:55')).toBeInTheDocument()
    expect(within(details()).getByText('10.0.0.2/24')).toBeInTheDocument()
    fireEvent.click(within(details()).getByRole('button', { name: 'virbr0' }))
    expect(within(details()).getByRole('heading', { name: 'virbr0' })).toBeInTheDocument()
    fireEvent.click(within(details()).getByRole('button', { name: 'default' }))
    expect(within(details()).getByRole('heading', { name: 'default' })).toBeInTheDocument()
  })

  it('preserves UUID selection through refresh and falls back when an item disappears', async () => {
    const load = vi.fn().mockResolvedValueOnce(inventory).mockResolvedValueOnce(snapshot({
      networks: [network({ uuid: 'new', name: 'aaa-new' }), network({ name: 'renamed-default' })],
    })).mockResolvedValueOnce(snapshot({ networks: [] }))
    await show(load)
    await advance()
    expect(within(details()).getByRole('heading', { name: 'renamed-default' })).toBeInTheDocument()
    await advance()
    expect(within(details()).getByRole('heading', { name: 'virbr0' })).toBeInTheDocument()
  })

  it('retains failed sources while refreshing healthy sources and clears warnings on recovery', async () => {
    const load = vi.fn().mockResolvedValueOnce(inventory).mockResolvedValueOnce(snapshot({ networks: [], machines: [],
      interfaces: [link('new-uplink')], issues: [{ source: 'networks', message: 'libvirt offline' }, { source: 'machines', message: 'guests offline' }],
    })).mockResolvedValueOnce(snapshot({ networks: [], interfaces: [], machines: [] }))
    await show(load)
    await advance()
    expect(screen.getByRole('alert')).toHaveTextContent('libvirt offline')
    expect(screen.getByRole('button', { name: /^new-uplink\b/ })).toBeInTheDocument()
    expect(within(details()).getByRole('heading', { name: 'default' })).toBeInTheDocument()
    expect(screen.getByRole('table', { name: 'Guest network attachments' })).toHaveTextContent('atlas-web')
    await advance()
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'No networking data' })).toBeInTheDocument()
  })

  it('keeps host interfaces usable on an initial libvirt failure without reporting zero attachments', async () => {
    await show(vi.fn().mockResolvedValue(snapshot({ networks: [], interfaces: [link('br0', { isBridge: true })],
      issues: [{ source: 'networks', message: 'access denied' }, { source: 'machines', message: 'access denied' }],
    })))
    expect(within(details()).getByRole('heading', { name: 'br0' })).toBeInTheDocument()
    expect(within(details()).getByText('Attachment inventory incomplete')).toBeInTheDocument()
    expect(screen.queryByText('No guest attachments')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: /^Libvirt/ }))
    expect(screen.getByRole('heading', { name: 'Libvirt networks unavailable' })).toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'No libvirt networks' })).not.toBeInTheDocument()
  })

  it('marks bridge attachment mappings incomplete when only the network source fails, keeping known direct matches', async () => {
    const data = snapshot({ networks: [], interfaces: [link('virbr0', { isBridge: true })],
      machines: [guest('stopped-reference', [{ type: 'network', source: { network: 'default' } }], { active: false })],
      issues: [{ source: 'networks', message: 'network inventory unavailable' }],
    })
    const load = vi.fn().mockResolvedValueOnce(data).mockResolvedValueOnce({ ...data, machines: [
      ...data.machines, guest('known-bridge', [{ type: 'bridge', source: { bridge: 'virbr0' } }]),
    ] }).mockResolvedValueOnce({ ...data, networks: [network()], issues: [] }).mockResolvedValueOnce(data)
    await show(load)
    expect(within(details()).getByText('Attachment inventory incomplete')).toBeInTheDocument()
    expect(within(details()).getByText('Partial inventory')).toBeInTheDocument()
    expect(screen.queryByText('No guest attachments')).not.toBeInTheDocument()
    await advance()
    expect(screen.getByRole('table', { name: 'Guest network attachments' })).toHaveTextContent('known-bridge')
    expect(within(details()).getByText('Partial inventory')).toBeInTheDocument()
    await advance()
    expect(screen.getByRole('table', { name: 'Guest network attachments' })).toHaveTextContent('stopped-reference')
    expect(within(details()).queryByText('Partial inventory')).not.toBeInTheDocument()
    await advance()
    expect(screen.getByRole('table', { name: 'Guest network attachments' })).toHaveTextContent('stopped-reference')
    expect(within(details()).getByText('Partial inventory')).toBeInTheDocument()
  })

  it('shows loading and unavailable states then retries automatically, and stops polling on unmount', async () => {
    let resolve!: (data: NetworkingInventory) => void
    const load = vi.fn().mockRejectedValueOnce(new Error('Wails disconnected')).mockReturnValueOnce(new Promise<NetworkingInventory>(done => { resolve = done }))
    const { unmount } = await show(load)
    expect(screen.getByRole('alert')).toHaveTextContent('Wails disconnected')
    expect(screen.getByRole('heading', { name: 'Networking unavailable' })).toBeInTheDocument()
    await advance()
    expect(screen.getByRole('heading', { name: 'Loading networking…' })).toBeInTheDocument()
    await advance(15000)
    expect(load).toHaveBeenCalledTimes(2)
    await act(async () => resolve(inventory))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(within(details()).getByRole('heading', { name: 'default' })).toBeInTheDocument()
    unmount()
    await advance(15000)
    expect(load).toHaveBeenCalledTimes(2)
  })
})
