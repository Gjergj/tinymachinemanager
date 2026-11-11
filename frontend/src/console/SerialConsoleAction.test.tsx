import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { SerialConsoleAction } from './SerialConsoleAction'
import { guest } from '../networking/testFixtures'

describe('serial console action', () => {
  it('checks device support and state before opening a window', async () => {
    const open = vi.fn(async () => {})
    const machine = guest('vm', [], { type: 'kvm', devices: { interfaces: [], serials: [], consoles: [] } })
    const view = render(<SerialConsoleAction machine={machine} open={open} />)
    expect(screen.getByRole('button', { name: 'Open serial console' })).toBeDisabled()
    expect(screen.getByText('No serial or text console is configured.')).toBeInTheDocument()
    machine.devices.serials = [{ type: 'pty', alias: 'serial0', target: { type: 'isa-serial', port: 0 } }]
    view.rerender(<SerialConsoleAction machine={{ ...machine, active: false }} open={open} />)
    expect(screen.getByRole('button', { name: 'Open serial console' })).toBeDisabled()
    view.rerender(<SerialConsoleAction machine={machine} open={open} />)
    fireEvent.click(screen.getByRole('button', { name: 'Open serial console' }))
    await waitFor(() => expect(open).toHaveBeenCalledWith('vm'))
  })
})
