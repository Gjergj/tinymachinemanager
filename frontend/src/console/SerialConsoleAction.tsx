import { useState } from 'react'
import { Button } from '@base-ui/react/button'
import type { Domain } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import { openSerialConsole } from './api'

export function SerialConsoleAction({ machine, open = openSerialConsole }: { machine: Domain; open?: (uuid: string) => Promise<void> }) {
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const devices = [...(machine.devices.serials || []), ...(machine.devices.consoles || [])]
  const reason = !machine.active ? 'Start the virtual machine before connecting.'
    : !devices.length ? 'No serial or text console is configured.'
    : !['kvm', 'qemu'].includes(machine.type) ? 'Serial consoles currently support QEMU/KVM guests only.'
    : !devices.some(d => d.type === 'pty' && d.alias) ? 'No supported PTY serial console is available.' : ''
  const launch = async () => {
    setPending(true); setError('')
    try { await open(machine.uuid) } catch (e) { setError(String(e instanceof Error ? e.message : e)) } finally { setPending(false) }
  }
  return <section className="serial-console-action" aria-label="Serial console">
    <Button disabled={!!reason || pending} onClick={() => void launch()}>{pending ? 'Opening console…' : 'Open serial console'}</Button>
    {reason && <p className="secondary">{reason}</p>}
    {error && <p role="alert">{error}</p>}
  </section>
}
