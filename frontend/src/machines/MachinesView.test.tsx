import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Domain } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import type { MachineInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import { MachinesView } from './MachinesView'

function guest(uuid: string, name: string, extra: Partial<Domain> = {}): Domain {
  return { uuid, name, state: 'running' as Domain['state'], active: true, persistent: true,
    vcpus: 2, memoryKiB: 4194304, maxMemoryKiB: 8388608, interfaces: [], ...extra }
}

const inventory = [
  guest('one', 'atlas-web'),
  guest('two', 'atlas-db', { state: 'paused' as Domain['state'], interfaces: [
    { type: 'network', source: { network: 'lab', portGroup: 'services' }, model: 'virtio', mac: '52:54:00:00:00:02',
      vlan: { trunk: false, tags: [{ id: 20 }] }, actual: { type: 'bridge', source: { bridge: 'virbr10' } } },
  ] }),
  guest('three', 'ubuntu-test', { active: false, state: 'shutoff' as Domain['state'], memoryKiB: 0 }),
]

function snapshot(machines: Domain[], host = { memoryKiB: 25165824, logicalCPUs: 12 }): MachineInventory {
  return { machines, host }
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
    expect(within(details).getByText('Not reported')).toBeInTheDocument()
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
})
