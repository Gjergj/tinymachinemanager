import { act, cleanup, fireEvent, render, screen } from '@testing-library/react'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import App from './App'
import { loadMachines } from './machines/api'
import { loadNetworking } from './networking/api'
import { snapshot } from './networking/testFixtures'

vi.mock('./machines/api', () => ({ loadMachines: vi.fn() }))
vi.mock('./networking/api', () => ({ loadNetworking: vi.fn() }))
beforeEach(() => vi.useFakeTimers())
afterEach(() => { cleanup(); vi.useRealTimers(); vi.clearAllMocks() })

it('navigates between Machines and Networking, polling only the mounted page', async () => {
  vi.mocked(loadMachines).mockResolvedValue({ machines: [], host: { memoryKiB: 25165824, logicalCPUs: 12 } })
  vi.mocked(loadNetworking).mockResolvedValue(snapshot())
  render(<App />)
  await act(async () => {})
  expect(screen.getByRole('heading', { name: 'Virtual machines' })).toBeInTheDocument()
  fireEvent.click(screen.getByRole('button', { name: 'Networking' }))
  await act(async () => {})
  expect(screen.getByRole('heading', { name: 'Networking' })).toBeInTheDocument()
  expect(screen.getByRole('button', { name: 'Networking' })).toHaveAttribute('aria-current', 'page')
  await act(async () => { await vi.advanceTimersByTimeAsync(5000) })
  expect(loadMachines).toHaveBeenCalledTimes(1)
  expect(loadNetworking).toHaveBeenCalledTimes(2)
  fireEvent.click(screen.getByRole('button', { name: 'Machines' }))
  await act(async () => {})
  expect(screen.getByRole('heading', { name: 'Virtual machines' })).toBeInTheDocument()
  expect(loadMachines).toHaveBeenCalledTimes(2)
})
