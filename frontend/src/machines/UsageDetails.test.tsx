import { act, cleanup, fireEvent, render, screen, within } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import type { GuestUsage } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/usage/models'
import { guest } from '../networking/testFixtures'
import { UsageDetails } from './UsageDetails'

afterEach(cleanup)
const machine = guest('one', [], { devices: { interfaces: [], memballoon: { model: 'virtio' } } })
const measured: GuestUsage = { uuid: 'one', cpu: { state: 'ok', percent: 50 }, memory: { state: 'ok', usedKiB: 1048576, totalKiB: 4194304 } }

it('shows normalized CPU and guest RAM separately from allocation', () => {
  render(<UsageDetails machine={machine} usage={measured} />)
  expect(screen.getByText('50%')).toBeInTheDocument()
  expect(screen.getByText('1 GiB / 4 GiB')).toBeInTheDocument()
  expect(screen.getByText(/of 2 vCPUs/)).toBeInTheDocument()
})

it('preserves measured zero values and does not clamp CPU overhead', () => {
  render(<UsageDetails machine={machine} usage={{ ...measured, cpu: { state: 'ok', percent: 150 }, memory: { state: 'ok', usedKiB: 0, totalKiB: 4194304 } }} />)
  expect(screen.getByText('150%')).toBeInTheDocument()
  expect(screen.getByText('0 MiB / 4 GiB')).toBeInTheDocument()
})

it('suppresses retained values for stopped guests and stale inventories', () => {
  const { rerender } = render(<UsageDetails machine={{ ...machine, active: false }} usage={measured} />)
  expect(screen.queryByText('50%')).not.toBeInTheDocument()
  expect(screen.queryByText('1 GiB / 4 GiB')).not.toBeInTheDocument()
  expect(screen.getAllByText('Stopped')).toHaveLength(2)
  rerender(<UsageDetails machine={machine} usage={measured} stale />)
  expect(screen.queryByText('50%')).not.toBeInTheDocument()
  expect(screen.queryByText('1 GiB / 4 GiB')).not.toBeInTheDocument()
  expect(screen.getAllByText('Stale')).toHaveLength(2)
})

it('shows collecting, unavailable and stale statistics without fake numbers', () => {
  const { rerender } = render(<UsageDetails machine={machine} usage={{ ...measured, cpu: { state: 'collecting' }, memory: { state: 'stale', message: 'Waiting for a fresh guest report.' } }} />)
  expect(screen.getByText('Collecting…')).toBeInTheDocument()
  expect(screen.getByText('Stale')).toBeInTheDocument()
  expect(screen.queryByText('0%')).not.toBeInTheDocument()
  expect(screen.queryByText('1 GiB / 4 GiB')).not.toBeInTheDocument()
  rerender(<UsageDetails machine={machine} />)
  expect(screen.getAllByText('Unavailable')).toHaveLength(2)
})

it('enables memory reporting explicitly for the selected UUID and prevents duplicate requests', async () => {
  let finish!: () => void
  const enable = vi.fn(() => new Promise<void>(resolve => { finish = resolve }))
  render(<UsageDetails machine={machine} usage={{ ...measured, memory: { state: 'disabled' } }} onEnableMemoryReporting={enable} />)
  const button = screen.getByRole('button', { name: 'Enable memory reporting' })
  fireEvent.click(button)
  expect(enable).toHaveBeenCalledWith('one')
  expect(screen.getByRole('button', { name: 'Enabling…' })).toBeDisabled()
  fireEvent.click(screen.getByRole('button', { name: 'Enabling…' }))
  expect(enable).toHaveBeenCalledTimes(1)
  await act(async () => finish())
})

it('surfaces configuration errors and limits the action to unconfigured supported VMs', async () => {
  const enable = vi.fn().mockRejectedValue(new Error('Not authorized to configure this VM'))
  const { rerender } = render(<UsageDetails machine={machine} usage={{ ...measured, memory: { state: 'disabled' } }} onEnableMemoryReporting={enable} />)
  fireEvent.click(screen.getByRole('button', { name: 'Enable memory reporting' }))
  await act(async () => {})
  expect(screen.getByRole('alert')).toHaveTextContent('Not authorized')
  const configured = { ...machine, devices: { interfaces: [], memballoon: { model: 'virtio', stats: { period: 5 } } } }
  rerender(<UsageDetails machine={configured} usage={{ ...measured, memory: { state: 'waiting' } }} onEnableMemoryReporting={enable} />)
  expect(screen.queryByRole('button', { name: 'Enable memory reporting' })).not.toBeInTheDocument()
  rerender(<UsageDetails machine={{ ...machine, devices: { interfaces: [] } }} onEnableMemoryReporting={enable} />)
  expect(screen.queryByRole('button', { name: 'Enable memory reporting' })).not.toBeInTheDocument()
})

it('can configure a persistent stopped VM before its next start', async () => {
  const enable = vi.fn().mockResolvedValue(undefined)
  render(<UsageDetails machine={{ ...machine, active: false, persistent: true }} onEnableMemoryReporting={enable} />)
  fireEvent.click(screen.getByRole('button', { name: 'Enable memory reporting' }))
  await act(async () => {})
  expect(enable).toHaveBeenCalledWith('one')
  expect(within(screen.getByRole('region', { name: 'Runtime usage' })).getAllByText('Stopped')).toHaveLength(2)
})
