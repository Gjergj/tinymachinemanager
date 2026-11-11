import { render, screen } from '@testing-library/react'
import { afterEach, expect, it, vi } from 'vitest'
import App from '../App'
import { loadMachines } from '../machines/api'
import { loadNetworking } from '../networking/api'

vi.mock('../machines/api', () => ({ loadMachines: vi.fn(), enableMemoryReporting: vi.fn() }))
vi.mock('../networking/api', () => ({ loadNetworking: vi.fn() }))
vi.mock('./terminal', () => ({ createTerminal: async () => ({ write() {}, setInput() {}, focus() {}, async copy() {}, async paste() {}, dispose() {} }) }))
vi.mock('./api', () => ({ consoleAPI: { capabilities: async () => ({ uuid: 'vm', name: 'Console guest', available: false, reason: 'No serial port.', devices: [] }), create: vi.fn(), disconnect: vi.fn() }, openSerialConsole: vi.fn() }))
afterEach(() => window.history.replaceState(null, '', '/'))

it('routes console windows without mounting inventory or networking polling', async () => {
  window.history.replaceState(null, '', '/?console=vm')
  render(<App />)
  expect(await screen.findByRole('heading', { name: 'Console guest' })).toBeInTheDocument()
  expect(loadMachines).not.toHaveBeenCalled()
  expect(loadNetworking).not.toHaveBeenCalled()
})
