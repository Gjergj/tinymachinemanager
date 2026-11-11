import { useEffect, useRef, useState } from 'react'
import { Button } from '@base-ui/react/button'
import type { Domain } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import type { GuestUsage } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/usage/models'
import type { EnableMemoryReporting } from './api'
import { memoryQuantityLabel } from './format'
import type { MemoryReportingState } from './useMemoryReporting'

export function UsageDetails({ machine, usage, stale = false, onEnableMemoryReporting, reportingState }: {
  machine: Domain; usage?: GuestUsage; stale?: boolean; onEnableMemoryReporting?: EnableMemoryReporting; reportingState?: MemoryReportingState
}) {
  const [enabling, setEnabling] = useState(false)
  const [requestError, setRequestError] = useState<string | null>(null)
  const alive = useRef(true)
  const pending = useRef(false)
  const busy = enabling || Boolean(reportingState?.pending)
  const errorMessage = reportingState?.error ?? requestError
  useEffect(() => { alive.current = true; return () => { alive.current = false } }, [])
  const observed = usage?.uuid === machine.uuid ? usage : undefined
  const cpu = observed?.cpu
  const memory = observed?.memory
  const number = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 })
  const cpuLabel = !machine.active ? 'Stopped' : stale ? 'Stale' : cpu?.state === 'ok' && cpu.percent != null
    ? number.format(cpu.percent) + '%' : cpu?.state === 'collecting' ? 'Collecting…' : cpu?.state === 'stopped' ? 'Stopped' : 'Unavailable'
  const memoryLabel = !machine.active ? 'Stopped' : stale ? 'Stale' : memory?.state === 'ok' && memory.usedKiB != null && memory.totalKiB != null
    ? memoryQuantityLabel(memory.usedKiB) + ' / ' + memoryQuantityLabel(memory.totalKiB)
    : memory?.state === 'stopped' ? 'Stopped' : memory?.state === 'stale' ? 'Stale' : memory?.state === 'waiting' ? 'Waiting…' : 'Unavailable'
  const balloon = machine.devices.memballoon
  const supported = balloon && ['virtio', 'virtio-transitional', 'virtio-non-transitional'].includes(balloon.model)
  const canEnable = onEnableMemoryReporting && !stale && supported && !balloon.stats?.period && (machine.active || machine.persistent)
  const enable = async () => {
    if (!onEnableMemoryReporting || pending.current || reportingState?.pending) return
    pending.current = true
    setEnabling(true)
    setRequestError(null)
    try { await onEnableMemoryReporting(machine.uuid) }
    catch (cause) { if (alive.current) setRequestError(cause instanceof Error ? cause.message : String(cause)) }
    finally { pending.current = false; if (alive.current) setEnabling(false) }
  }
  return <section className="usage-details" aria-label="Runtime usage"><h3 className="eyebrow">Runtime usage</h3>
    <dl className="detail-list"><dt>CPU usage</dt><dd title={cpu?.message}>{cpuLabel}</dd><dt>Guest RAM</dt><dd title={memory?.message}>{memoryLabel}</dd></dl>
    {machine.active && !stale && <p className="usage-help secondary">CPU usage as a share of {machine.vcpus} vCPUs.</p>}
    {machine.active && !stale && memory?.state !== 'ok' && memory?.message && <p className="usage-help secondary">{memory.message}</p>}
    {canEnable && <div className="memory-reporting-action"><Button className="secondary-button" disabled={busy} onClick={() => { void enable() }}>{busy ? 'Enabling…' : 'Enable memory reporting'}</Button>
      <p className="usage-help secondary">Collects guest statistics every 5 seconds. {machine.persistent ? 'Saved for future starts.' : 'Applies while this VM runs.'}</p>
    </div>}
    {errorMessage && <p className="usage-error" role="alert">{errorMessage}</p>}
  </section>
}
