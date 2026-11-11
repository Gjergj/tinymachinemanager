import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Domain } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import type { MachineInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import { MachinesView } from './MachinesView'

function guest(uuid: string, name: string, extra: Partial<Domain> = {}): Domain {
  return { uuid, name, type: '', os: { type: {} }, devices: { serials: [], consoles: [], interfaces: [] }, state: 'running' as Domain['state'], active: true, persistent: true,
    vcpus: 2, memoryKiB: 4194304, maxMemoryKiB: 8388608, ...extra }
}

const inventory = [
  guest('one', 'atlas-web'),
  guest('two', 'atlas-db', { state: 'paused' as Domain['state'], devices: { serials: [], consoles: [], interfaces: [
    { type: 'network', source: { network: 'lab', portGroup: 'services' }, model: 'virtio', mac: '52:54:00:00:00:02',
      vlan: { trunk: false, tags: [{ id: 20 }] }, actual: { type: 'bridge', source: { bridge: 'virbr10' } } },
  ] } }),
  guest('three', 'ubuntu-test', { active: false, state: 'shutoff' as Domain['state'], memoryKiB: 0 }),
]

function snapshot(machines: Domain[], host = { memoryKiB: 25165824, logicalCPUs: 12 }): MachineInventory {
  return { machines, host, usage: [] }
}

async function showMachines(loadMachines: () => Promise<MachineInventory>) {
  const view = render(<MachinesView loadMachines={loadMachines} />)
  await act(async () => {})
  return view
}

async function advance(milliseconds = 5000) {
  await act(async () => { await vi.advanceTimersByTimeAsync(milliseconds) })
}

describe('Machines view', () => {
  beforeEach(() => vi.useFakeTimers())
  afterEach(() => { cleanup(); vi.useRealTimers() })

  it('lists guests and shows configured and resolved NIC information on selection', async () => {
    await showMachines(vi.fn().mockResolvedValue(snapshot(inventory)))
    fireEvent.click(screen.getByRole('button', { name: 'atlas-db' }))
    const details = screen.getByRole('complementary', { name: 'Selected machine' })
    expect(within(details).getByRole('heading', { name: 'atlas-db' })).toBeInTheDocument()
    expect(within(details).getByText('lab')).toBeInTheDocument()
    expect(within(details).getByText('virbr10')).toBeInTheDocument()
    expect(within(details).getByText('52:54:00:00:00:02')).toBeInTheDocument()
    expect(within(details).getByText('VLAN 20')).toBeInTheDocument()
    expect(within(details).getByText('4 GiB')).toBeInTheDocument()
    expect(within(details).getByText('8 GiB')).toBeInTheDocument()
  })

  it('includes paused guests in Active and changes selection to a visible stopped guest', async () => {
    await showMachines(vi.fn().mockResolvedValue(snapshot(inventory)))
    fireEvent.click(screen.getByRole('tab', { name: /Active/ }))
    expect(screen.getByRole('button', { name: 'atlas-db' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'ubuntu-test' })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: /Stopped/ }))
    const details = screen.getByRole('complementary', { name: 'Selected machine' })
    expect(within(details).getByRole('heading', { name: 'ubuntu-test' })).toBeInTheDocument()
    expect(within(details).getByText('Memory').nextElementSibling).toHaveTextContent('Not reported')
  })

  it('refreshes at five-second intervals without a manual Refresh button', async () => {
    const load = vi.fn().mockResolvedValueOnce(snapshot([inventory[0]])).mockResolvedValue(snapshot([guest('one', 'updated-web')]))
    await showMachines(load)
    expect(screen.queryByRole('button', { name: 'Refresh machines' })).not.toBeInTheDocument()
    await advance(4999)
    expect(screen.getByRole('button', { name: 'atlas-web' })).toBeInTheDocument()
    await advance(1)
    expect(screen.getByRole('button', { name: 'updated-web' })).toBeInTheDocument()
  })

  it('distinguishes an empty inventory from a failed connection', async () => {
    const load = vi.fn().mockResolvedValueOnce(snapshot([])).mockRejectedValueOnce(new Error('Permission denied on qemu:///system'))
    await showMachines(load)
    expect(screen.getByText('No virtual machines yet')).toBeInTheDocument()
    await advance()
    expect(screen.getByRole('alert')).toHaveTextContent('Permission denied')
    expect(screen.queryByText('No virtual machines yet')).not.toBeInTheDocument()
  })

  it('preserves UUID selection on refresh and replaces the selected machine data', async () => {
    const load = vi.fn().mockResolvedValueOnce(snapshot(inventory)).mockResolvedValueOnce(snapshot([
      guest('two', 'renamed-db', { memoryKiB: 2097152 }), inventory[0],
    ]))
    await showMachines(load)
    fireEvent.click(screen.getByRole('button', { name: 'atlas-db' }))
    await advance()
    const details = screen.getByRole('complementary', { name: 'Selected machine' })
    expect(within(details).getByRole('heading', { name: 'renamed-db' })).toBeInTheDocument()
    expect(within(details).getByText('2 GiB')).toBeInTheDocument()
  })

  it('keeps the automatically selected machine when a new row sorts before it', async () => {
    const load = vi.fn().mockResolvedValueOnce(snapshot(inventory)).mockResolvedValueOnce(snapshot([
      guest('new', 'aaa-new'), ...inventory,
    ]))
    await showMachines(load)
    await advance()
    expect(screen.getByRole('button', { name: 'aaa-new' })).toBeInTheDocument()
    const details = screen.getByRole('complementary', { name: 'Selected machine' })
    expect(within(details).getByRole('heading', { name: 'atlas-web' })).toBeInTheDocument()
  })

  it('keeps stale data labelled during a retry and recovers automatically', async () => {
    let resolveRetry!: (value: MachineInventory) => void
    const retry = new Promise<MachineInventory>(resolve => { resolveRetry = resolve })
    const load = vi.fn().mockResolvedValueOnce(snapshot(inventory))
      .mockRejectedValueOnce(new Error('Libvirt disconnected')).mockReturnValueOnce(retry)
    await showMachines(load)
    await advance()
    expect(screen.getByRole('alert')).toHaveTextContent('Showing the last loaded inventory')
    await advance()
    expect(screen.getByRole('alert')).toHaveTextContent('Showing the last loaded inventory')
    expect(screen.getByRole('button', { name: 'atlas-web' })).toBeInTheDocument()
    await act(async () => resolveRetry(snapshot([inventory[2]])))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'atlas-web' })).not.toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'ubuntu-test' })).toBeInTheDocument()
  })

  it('does not replace a new request with an older response', async () => {
    let resolveOld!: (value: MachineInventory) => void
    const oldLoader = () => new Promise<MachineInventory>(resolve => { resolveOld = resolve })
    const { rerender } = await showMachines(oldLoader)
    const newLoader = vi.fn().mockResolvedValue(snapshot([guest('new', 'new-machine')]))
    rerender(<MachinesView loadMachines={newLoader} />)
    await act(async () => {})
    await act(async () => resolveOld(snapshot(inventory)))
    expect(screen.getByRole('button', { name: 'new-machine' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'atlas-web' })).not.toBeInTheDocument()
  })

  it('skips polling ticks while a native request is still pending', async () => {
    let resolveFirst!: (value: MachineInventory) => void
    const pending = new Promise<MachineInventory>(resolve => { resolveFirst = resolve })
    const load = vi.fn().mockReturnValueOnce(pending).mockResolvedValue(snapshot([guest('new', 'new-machine')]))
    await showMachines(load)
    await advance(15000)
    expect(load).toHaveBeenCalledTimes(1)
    await act(async () => resolveFirst(snapshot(inventory)))
    expect(screen.getByRole('button', { name: 'atlas-web' })).toBeInTheDocument()
    await advance()
    expect(screen.getByRole('button', { name: 'new-machine' })).toBeInTheDocument()
  })

  it('stops polling after the view unmounts', async () => {
    const load = vi.fn().mockResolvedValue(snapshot(inventory))
    const { unmount } = await showMachines(load)
    unmount()
    await advance(20000)
    expect(load).toHaveBeenCalledTimes(1)
  })

  it('compares all guest assignments with host capacity independently of the state filter', async () => {
    const machines = inventory.map(machine => ({ ...machine, memoryKiB: 4194304 }))
    await showMachines(vi.fn().mockResolvedValue(snapshot(machines)))
    expect(screen.getByText('12 / 24 GiB')).toBeInTheDocument()
    expect(screen.getByText('6 / 12')).toBeInTheDocument()
    fireEvent.click(screen.getByRole('tab', { name: /Stopped/ }))
    expect(screen.getByText('12 / 24 GiB')).toBeInTheDocument()
    expect(screen.getByText('6 / 12')).toBeInTheDocument()
    expect(screen.getByText(/machine shown/)).toHaveTextContent('1 machine shown')
  })

  it('shows capacity with zero assignments for an empty host and an empty filter', async () => {
    const load = vi.fn().mockResolvedValueOnce(snapshot([])).mockResolvedValueOnce(snapshot([inventory[0]]))
    await showMachines(load)
    expect(screen.getByText('0 / 24 GiB')).toBeInTheDocument()
    expect(screen.getByText('0 / 12')).toBeInTheDocument()
    await advance()
    fireEvent.click(screen.getByRole('tab', { name: /Stopped/ }))
    expect(screen.getByText('No stopped machines')).toBeInTheDocument()
    expect(screen.getByText('4 / 24 GiB')).toBeInTheDocument()
    expect(screen.getByText('2 / 12')).toBeInTheDocument()
  })

  it('shows overcommit without clamping and updates capacity with guest assignments', async () => {
    const load = vi.fn().mockResolvedValueOnce(snapshot([guest('one', 'atlas-web', { vcpus: 16 })]))
      .mockResolvedValueOnce(snapshot([guest('one', 'atlas-web', { vcpus: 20, memoryKiB: 33554432 })], { memoryKiB: 16777216, logicalCPUs: 8 }))
    await showMachines(load)
    expect(screen.getByText('16 / 12')).toBeInTheDocument()
    await advance()
    expect(screen.getByText('20 / 8')).toBeInTheDocument()
    expect(screen.getByText('32 / 16 GiB')).toBeInTheDocument()
  })

  it('does not silently count unreported guest memory as zero', async () => {
    await showMachines(vi.fn().mockResolvedValue(snapshot(inventory)))
    expect(screen.getByText('— / 24 GiB')).toBeInTheDocument()
    expect(screen.queryByText('8 / 24 GiB')).not.toBeInTheDocument()
    expect(screen.getByText('6 / 12')).toBeInTheDocument()
  })

  it('leaves unavailable host capacity unknown instead of displaying zero', async () => {
    await showMachines(vi.fn().mockResolvedValue(snapshot([inventory[0]], { memoryKiB: 0, logicalCPUs: 0 })))
    expect(screen.getByText('4 / — GiB')).toBeInTheDocument()
    expect(screen.getByText('2 / —')).toBeInTheDocument()
  })

  it('shows nested XML platform metadata and updates it with the selected guest on refresh', async () => {
    const load = vi.fn().mockResolvedValueOnce(snapshot([guest('one', 'platform-vm', {
      type: 'kvm', os: { type: { value: 'hvm', arch: 'x86_64', machine: 'pc-q35-10.2' }, firmware: 'efi',
        loader: { type: 'pflash', path: '/usr/share/OVMF/OVMF_CODE_4M.ms.fd', secure: 'yes' } },
      devices: { serials: [], consoles: [], emulator: '/usr/bin/qemu-system-x86_64', interfaces: [] },
    })])).mockResolvedValueOnce(snapshot([guest('one', 'platform-vm', {
      type: 'qemu', os: { type: { arch: 'aarch64' }, firmware: 'bios' },
      devices: { serials: [], consoles: [], emulator: '/usr/bin/qemu-system-aarch64', interfaces: [] },
    })]))
    await showMachines(load)
    let platform = screen.getByRole('region', { name: 'Platform' })
    expect(within(platform).getByText('Hypervisor').nextElementSibling).toHaveTextContent('KVM')
    expect(within(platform).getByText('Architecture').nextElementSibling).toHaveTextContent('x86_64')
    expect(within(platform).getByText('Emulator').nextElementSibling).toHaveTextContent('/usr/bin/qemu-system-x86_64')
    expect(within(platform).getByText('Firmware').nextElementSibling).toHaveTextContent('UEFI')
    expect(within(platform).getByText('/usr/share/OVMF/OVMF_CODE_4M.ms.fd')).toBeInTheDocument()
    await advance()
    platform = screen.getByRole('region', { name: 'Platform' })
    expect(within(platform).getByText('Hypervisor').nextElementSibling).toHaveTextContent('QEMU')
    expect(within(platform).getByText('Architecture').nextElementSibling).toHaveTextContent('aarch64')
    expect(within(platform).getByText('Emulator').nextElementSibling).toHaveTextContent('/usr/bin/qemu-system-aarch64')
    expect(within(platform).getByText('Firmware').nextElementSibling).toHaveTextContent('BIOS')
    expect(screen.queryByText('/usr/share/OVMF/OVMF_CODE_4M.ms.fd')).not.toBeInTheDocument()
  })

  it('shows unreported platform fields without inventing a hypervisor or BIOS default', async () => {
    await showMachines(vi.fn().mockResolvedValue(snapshot([inventory[0]])))
    const platform = screen.getByRole('region', { name: 'Platform' })
    expect(within(platform).getAllByText('Not reported')).toHaveLength(4)
    expect(within(platform).queryByText('BIOS')).not.toBeInTheDocument()
    expect(within(platform).queryByText('KVM')).not.toBeInTheDocument()
  })

  it.each<{ os: Domain['os']; expected: string }>([
    { os: { type: {}, firmware: 'bios' }, expected: 'BIOS' },
    { os: { type: {}, loader: { type: 'pflash' } }, expected: 'UEFI' },
    { os: { type: {}, loader: { type: 'rom', path: '/custom/firmware.fd' } }, expected: 'Not reported' },
    { os: { type: {}, firmware: 'future-firmware', loader: { type: 'pflash' } }, expected: 'future-firmware' },
  ])('uses explicit firmware or a pflash loader without classifying ROM-only firmware ($expected)', async ({ os, expected }) => {
    await showMachines(vi.fn().mockResolvedValue(snapshot([guest('one', 'platform-vm', { type: 'future-hv', os })])))
    const platform = screen.getByRole('region', { name: 'Platform' })
    expect(within(platform).getByText('Firmware').nextElementSibling).toHaveTextContent(expected)
    expect(within(platform).getByText('Hypervisor').nextElementSibling).toHaveTextContent('future-hv')
  })

  it('shows IPv4 and IPv6 DHCP addresses under their matching NICs', async () => {
    await showMachines(vi.fn().mockResolvedValue(snapshot([guest('one', 'leased-vm', {
      devices: { serials: [], consoles: [], interfaces: [
        { type: 'network', source: { network: 'default' }, mac: '00:1A:2B:3C:4D:5E' },
        { type: 'network', source: { network: 'lab' }, mac: '00:1a:2b:3c:4d:5a' },
      ] }, dhcp: { interfaces: [
        { index: 1, mac: '00:1a:2b:3c:4d:5a', addresses: [{ family: 'ipv4', address: '10.0.0.8', prefix: 24 }] },
        { index: 0, mac: '00:1a:2b:3c:4d:5e', addresses: [
          { family: 'ipv4', address: '192.168.122.41', prefix: 24 },
          { family: 'ipv6', address: '2001:db8::41', prefix: 64 },
        ] },
      ] },
    })])))
    const first = screen.getByRole('region', { name: 'Network interface 1' })
    const second = screen.getByRole('region', { name: 'Network interface 2' })
    expect(within(first).getByText('192.168.122.41/24')).toBeInTheDocument()
    expect(within(first).getByText('2001:db8::41/64')).toBeInTheDocument()
    expect(within(first).queryByText('10.0.0.8/24')).not.toBeInTheDocument()
    expect(within(second).getByText('10.0.0.8/24')).toBeInTheDocument()
  })

  it.each<{ nic: Domain['devices']['interfaces'][number]; dhcp?: Domain['dhcp']; expected: string }>([
    { nic: { type: 'network', source: { network: 'default' }, mac: '00:1a:2b:3c:4d:5e' }, dhcp: { interfaces: [{ index: 0, mac: '00:1a:2b:3c:4d:5e', addresses: [] }] }, expected: 'No lease reported' },
    { nic: { type: 'network', source: { network: 'default' }, mac: '00:1a:2b:3c:4d:5e' }, dhcp: { interfaces: [], error: 'Lease permission denied' }, expected: 'Lease data unavailable' },
    { nic: { type: 'network', source: { network: 'default' }, mac: '00:1a:2b:3c:4d:5e' }, expected: 'Not reported' },
    { nic: { type: 'bridge', source: { bridge: 'external' }, mac: '00:1a:2b:3c:4d:5e' }, expected: 'Not available' },
    { nic: { type: 'network', source: { network: 'default' } }, expected: 'Not available' },
  ])('keeps missing and unavailable DHCP data distinct ($expected)', async ({ nic, dhcp, expected }) => {
    await showMachines(vi.fn().mockResolvedValue(snapshot([guest('one', 'leased-vm', { devices: { serials: [], consoles: [], interfaces: [nic] }, dhcp })])))
    const details = screen.getByRole('region', { name: 'Network interface 1' })
    expect(within(details).getByText('DHCP addresses').nextElementSibling).toHaveTextContent(expected)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('clears lease addresses after refresh and hides old observations when stopped or a lookup fails', async () => {
    const machine = guest('one', 'leased-vm', { devices: { serials: [], consoles: [], interfaces: [{ type: 'network', source: { network: 'default' }, mac: '00:1a:2b:3c:4d:5e' }] },
      dhcp: { interfaces: [{ index: 0, mac: '00:1a:2b:3c:4d:5e', addresses: [{ family: 'ipv4', address: '192.168.122.41', prefix: 24 }] }] },
    })
    const load = vi.fn().mockResolvedValueOnce(snapshot([machine]))
      .mockResolvedValueOnce(snapshot([{ ...machine, dhcp: { interfaces: [{ index: 0, mac: '00:1a:2b:3c:4d:5e', addresses: [] }] } }]))
      .mockResolvedValueOnce(snapshot([{ ...machine, active: false, state: 'shutoff' as Domain['state'] }]))
      .mockResolvedValueOnce(snapshot([{ ...machine, dhcp: { ...machine.dhcp!, error: 'Lease access denied' } }]))
    await showMachines(load)
    expect(screen.getByText('192.168.122.41/24')).toBeInTheDocument()
    await advance()
    expect(screen.queryByText('192.168.122.41/24')).not.toBeInTheDocument()
    expect(screen.getByText('No lease reported')).toBeInTheDocument()
    await advance()
    expect(screen.getByText('VM is stopped')).toBeInTheDocument()
    expect(screen.queryByText('192.168.122.41/24')).not.toBeInTheDocument()
    await advance()
    expect(screen.getByText('Lease data unavailable')).toHaveAttribute('title', 'Lease access denied')
    expect(screen.queryByText('192.168.122.41/24')).not.toBeInTheDocument()
  })

  it('does not display a lease observation for a different MAC at the same NIC index', async () => {
    await showMachines(vi.fn().mockResolvedValue(snapshot([guest('one', 'leased-vm', {
      devices: { serials: [], consoles: [], interfaces: [{ type: 'network', source: { network: 'default' }, mac: '00:1a:2b:3c:4d:5e' }] },
      dhcp: { interfaces: [{ index: 0, mac: '00:1a:2b:3c:4d:5a', addresses: [{ family: 'ipv4', address: '10.0.0.8', prefix: 24 }] }] },
    })])))
    expect(screen.queryByText('10.0.0.8/24')).not.toBeInTheDocument()
    const details = screen.getByRole('region', { name: 'Network interface 1' })
    expect(within(details).getByText('DHCP addresses').nextElementSibling).toHaveTextContent('Not reported')
  })

  it('refreshes after explicit memory enablement even when an older poll is pending', async () => {
    const machine = guest('one', 'reporting-vm', { devices: { serials: [], consoles: [], interfaces: [], memballoon: { model: 'virtio' } } })
    const configured = { ...machine, devices: { serials: [], consoles: [], interfaces: [], memballoon: { model: 'virtio', stats: { period: 5 } } } }
    let finishPoll!: (value: MachineInventory) => void
    const olderPoll = new Promise<MachineInventory>(resolve => { finishPoll = resolve })
    const load = vi.fn().mockResolvedValueOnce(snapshot([machine])).mockReturnValueOnce(olderPoll).mockResolvedValueOnce({
      ...snapshot([configured]), usage: [{ uuid: 'one', cpu: { state: 'ok', percent: 20 }, memory: { state: 'waiting' } }],
    })
    const enable = vi.fn().mockResolvedValue(undefined)
    render(<MachinesView loadMachines={load} enableMemoryReporting={enable} />)
    await act(async () => {})
    await advance()
    fireEvent.click(screen.getByRole('button', { name: 'Enable memory reporting' }))
    await act(async () => {})
    expect(enable).toHaveBeenCalledWith('one')
    expect(load).toHaveBeenCalledTimes(2)
    expect(screen.getByRole('button', { name: 'Enabling…' })).toBeDisabled()
    await act(async () => finishPoll(snapshot([machine])))
    expect(load).toHaveBeenCalledTimes(3)
    expect(screen.queryByRole('button', { name: 'Enable memory reporting' })).not.toBeInTheDocument()
    expect(screen.getByText('20%')).toBeInTheDocument()
    expect(screen.getByText('Waiting…')).toBeInTheDocument()
  })

  it('suppresses live usage when refreshing the overall inventory fails', async () => {
    const measured = { ...snapshot([inventory[0]]), usage: [{ uuid: 'one', cpu: { state: 'ok', percent: 40 }, memory: { state: 'ok', usedKiB: 1048576, totalKiB: 4194304 } }] }
    await showMachines(vi.fn().mockResolvedValueOnce(measured).mockRejectedValueOnce(new Error('Inventory unavailable')))
    expect(screen.getByText('40%')).toBeInTheDocument()
    await advance()
    const details = screen.getByRole('region', { name: 'Runtime usage' })
    expect(within(details).queryByText('40%')).not.toBeInTheDocument()
    expect(within(details).queryByText('1 GiB / 4 GiB')).not.toBeInTheDocument()
    expect(within(details).getAllByText('Stale')).toHaveLength(2)
  })

  it('keeps enable requests and errors attached to their VM across selection changes', async () => {
    const machines = [guest('one', 'vm-one', { devices: { serials: [], consoles: [], interfaces: [], memballoon: { model: 'virtio' } } }),
      guest('two', 'vm-two', { devices: { serials: [], consoles: [], interfaces: [], memballoon: { model: 'virtio' } } })]
    let reject!: (reason: Error) => void
    const enable = vi.fn(() => new Promise<void>((_resolve, fail) => { reject = fail }))
    render(<MachinesView loadMachines={vi.fn().mockResolvedValue(snapshot(machines))} enableMemoryReporting={enable} />)
    await act(async () => {})
    fireEvent.click(screen.getByRole('button', { name: 'Enable memory reporting' }))
    await act(async () => {})
    fireEvent.click(screen.getByRole('button', { name: 'vm-two' }))
    fireEvent.click(screen.getByRole('button', { name: 'vm-one' }))
    expect(screen.getByRole('button', { name: 'Enabling…' })).toBeDisabled()
    expect(enable).toHaveBeenCalledTimes(1)
    await act(async () => reject(new Error('VM one configuration denied')))
    expect(screen.getByRole('alert')).toHaveTextContent('VM one configuration denied')
    fireEvent.click(screen.getByRole('button', { name: 'vm-two' }))
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'vm-one' }))
    expect(screen.getByRole('alert')).toHaveTextContent('VM one configuration denied')
  })
})
