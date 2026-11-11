import { act, fireEvent, render, screen, waitFor } from '@testing-library/react'
import { StrictMode } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { ConsoleView } from './ConsoleView'
import type { ConsoleAPI, ConsoleCapabilities } from './api'
import type { TerminalAdapter } from './terminal'
import type { ConsoleStatus } from './connection'

const capabilities: ConsoleCapabilities = { uuid: 'vm', name: 'balboa', active: true, state: 'running', available: true, code: 'available', reason: '', revision: 'revision', devices: [{ key: 'serial', label: 'Serial 1', kind: 'Serial', backend: 'pty', alias: 'serial0', supported: true, reason: '' }] }
function setup() {
  const api: ConsoleAPI = { capabilities: vi.fn(async () => capabilities), create: vi.fn(async () => ({ id: 'session', url: 'ws://127.0.0.1/serial', token: 'secret' })), disconnect: vi.fn(async () => {}) }
  const terminal: TerminalAdapter = { write: vi.fn(), setInput: vi.fn(), focus: vi.fn(), copy: vi.fn(async () => {}), paste: vi.fn(async () => {}), dispose: vi.fn() }
  const makeTerminal = vi.fn(async () => terminal)
  const connect = vi.fn(() => ({ send: vi.fn(), close: vi.fn() }))
  return { api, terminal, makeTerminal, connect }
}
describe('serial console window', () => {
  it('does not send a window-bound cleanup call after backend-owned disconnection', async () => {
    const f = setup()
    let report!: (status: ConsoleStatus) => void
    const close = vi.fn()
    const connect = vi.fn((_credentials, _write, status) => { report = status; return { send: vi.fn(), close } })
    const view = render(<ConsoleView uuid="vm" {...f} connect={connect} />)
    await waitFor(() => expect(connect).toHaveBeenCalledTimes(1))
    act(() => report({ type: 'connected' }))
    await act(async () => report({ type: 'disconnected', backendOwnsCleanup: true }))
    expect(close).toHaveBeenCalled()
    expect(f.api.disconnect).not.toHaveBeenCalled()
    view.unmount()
    await act(async () => {})
    expect(f.api.disconnect).not.toHaveBeenCalled()
  })
  it('does not attach after Disconnect during terminal initialization', async () => {
    const f = setup()
    let finish!: (terminal: TerminalAdapter) => void
    f.makeTerminal = vi.fn(() => new Promise<TerminalAdapter>(resolve => { finish = resolve }))
    render(<ConsoleView uuid="vm" {...f} />)
    fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }))
    await screen.findByText('Disconnected. The guest shell may still be running.')
    await act(async () => finish(f.terminal))
    expect(f.api.create).not.toHaveBeenCalled()
    fireEvent.click(screen.getByRole('button', { name: 'Reconnect' }))
    await waitFor(() => expect(f.api.create).toHaveBeenCalledTimes(1))
  })
  it('initializes before requesting attachment and cancels a late session on close', async () => {
    const f = setup()
    let finish!: (c: { id: string; url: string; token: string }) => void
    f.api.create = vi.fn(() => new Promise<{ id: string; url: string; token: string }>(resolve => { finish = resolve }))
    const view = render(<ConsoleView uuid="vm" {...f} />)
    await waitFor(() => expect(f.api.create).toHaveBeenCalled())
    expect(f.makeTerminal).toHaveBeenCalled()
    view.unmount()
    await act(async () => finish({ id: 'late', url: 'ws://127.0.0.1/serial', token: 'secret' }))
    expect(f.api.disconnect).toHaveBeenCalledWith('late')
    expect(f.connect).not.toHaveBeenCalled()
  })
  it('explains unsupported machines and never creates a session', async () => {
    const f = setup(); f.api.capabilities = vi.fn(async () => ({ ...capabilities, available: false, reason: 'No serial or text console is configured.', devices: [] }))
    render(<ConsoleView uuid="vm" {...f} />)
    expect(await screen.findByText('No serial or text console is configured.')).toBeInTheDocument()
    expect(f.api.create).not.toHaveBeenCalled()
  })
  it('survives StrictMode and reconnects only on an explicit action', async () => {
    const f = setup()
    render(<StrictMode><ConsoleView uuid="vm" {...f} /></StrictMode>)
    await waitFor(() => expect(f.connect).toHaveBeenCalledTimes(1))
    expect(screen.getByRole('heading', { name: 'balboa' })).toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Disconnect' }))
    await waitFor(() => expect(f.api.disconnect).toHaveBeenCalledWith('session'))
    expect(f.connect).toHaveBeenCalledTimes(1)
    fireEvent.click(screen.getByRole('button', { name: 'Reconnect' }))
    await waitFor(() => expect(f.connect).toHaveBeenCalledTimes(2))
  })
})
