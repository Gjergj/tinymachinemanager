import { useEffect, useState } from 'react'
import { Button } from '@base-ui/react/button'
import { Tabs } from '@base-ui/react/tabs'
import type { Domain } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import { InfoIcon, MachineIcon } from './Icons'
import { AppShell, type Page } from '../shared/AppShell'
import { MachineDetails } from './MachineDetails'
import { ResourceSummary } from './ResourceSummary'
import { memoryLabel, stateLabel } from './format'
import { useMachines, type LoadMachines } from './useMachines'
import type { EnableMemoryReporting } from './api'
import { useMemoryReporting } from './useMemoryReporting'

type Filter = 'all' | 'active' | 'stopped'
const filters: { value: Filter; label: string }[] = [
  { value: 'all', label: 'All' }, { value: 'active', label: 'Active' }, { value: 'stopped', label: 'Stopped' },
]
function MachineTable({ machines, selectedUUID, onSelect }: { machines: Domain[]; selectedUUID: string | undefined; onSelect: (uuid: string) => void }) {
  return <div className="machine-table-scroll"><table className="machine-table">
    <caption className="sr-only">Virtual machines on the local system connection</caption>
    <thead><tr><th scope="col">Virtual machine</th><th scope="col">State</th><th scope="col" className="numeric">vCPU</th><th scope="col" className="numeric">Memory</th><th scope="col" className="numeric">NICs</th></tr></thead>
    <tbody>{machines.map(machine => <tr key={machine.uuid} data-selected={selectedUUID === machine.uuid || undefined}>
      <th scope="row"><Button className="machine-name" onClick={() => onSelect(machine.uuid)} aria-pressed={selectedUUID === machine.uuid}><MachineIcon /><span>{machine.name}</span></Button></th>
      <td><span className="machine-state" data-state={machine.state}>{stateLabel(machine.state)}</span></td>
      <td className="numeric">{machine.vcpus}</td><td className="numeric">{machine.memoryKiB ? memoryLabel(machine.memoryKiB) : '—'}</td><td className="numeric">{machine.devices.interfaces.length}</td>
    </tr>)}</tbody>
  </table></div>
}
export function MachinesView({ loadMachines, onNavigate, enableMemoryReporting }: { loadMachines: LoadMachines; onNavigate?: (page: Page) => void; enableMemoryReporting?: EnableMemoryReporting }) {
  const { machines, host, usage, loading, error, updatedAt, refresh } = useMachines(loadMachines)
  const reporting = useMemoryReporting(enableMemoryReporting, refresh)
  const [filter, setFilter] = useState<Filter>('all')
  const [selectedUUID, setSelectedUUID] = useState<string | null>(null)
  const visible = machines.filter(machine => filter === 'all' || (filter === 'active' ? machine.active : !machine.active))
  const selected = visible.find(machine => machine.uuid === selectedUUID) ?? visible[0]
  const visibleSelection = selected?.uuid ?? null
  useEffect(() => {
    setSelectedUUID(visibleSelection)
  }, [visibleSelection])
  const counts: Record<Filter, number> = { all: machines.length, active: machines.filter(machine => machine.active).length, stopped: machines.filter(machine => !machine.active).length }
  const stale = Boolean(error && updatedAt)
  const firstLoad = loading && !updatedAt
  return <AppShell page="machines" onNavigate={onNavigate} loading={loading} stale={stale} updatedAt={updatedAt}>
      <header className="page-heading"><div><h1>Virtual machines</h1></div>
        {/* <span className="auto-refresh-label">Auto-refresh · 5s</span> */}
      </header>
      {error && <div className="connection-error" role="alert"><InfoIcon /><div><h2>Couldn't load virtual machines</h2><p>Check that libvirt is running and your user can access the system connection.</p><p className="error-message">{error}</p>{stale && <strong>Showing the last loaded inventory. It may be out of date.</strong>}</div></div>}
      <Tabs.Root value={filter} onValueChange={value => { if (value === 'all' || value === 'active' || value === 'stopped') setFilter(value) }}>
        <div className="inventory-toolbar"><Tabs.List className="inventory-tabs" aria-label="Machine state">
          {filters.map(item => <Tabs.Tab key={item.value} className="inventory-tab" value={item.value}>{item.label}<span className="tab-count">{updatedAt ? counts[item.value] : '—'}</span></Tabs.Tab>)}
        </Tabs.List></div>
        {filters.map(item => <Tabs.Panel key={item.value} value={item.value} className="inventory-panel">
          {firstLoad ? <div className="inventory-empty" role="status"><span className="empty-icon"><MachineIcon /></span><h2>Loading virtual machines…</h2><p>Reading the local libvirt inventory.</p></div> :
            error && !machines.length ? <div className="inventory-empty"><span className="empty-icon"><MachineIcon /></span><h2>Inventory unavailable</h2><p>The connection will be retried automatically.</p></div> :
            <div className="machines-layout" data-empty={!visible.length || undefined}><section className="machine-inventory" aria-label="Machine inventory" aria-busy={loading}>
              {visible.length ? <MachineTable machines={visible} selectedUUID={selected?.uuid} onSelect={setSelectedUUID} /> :
                <div className="inventory-empty" role="status"><span className="empty-icon"><MachineIcon /></span><h2>{machines.length ? 'No ' + (item.value === 'active' ? 'active' : 'stopped') + ' machines' : 'No virtual machines yet'}</h2><p>{machines.length ? 'Choose another state to see your other guests.' : 'This libvirt connection has no virtual machines to display.'}</p></div>}
              {host && <ResourceSummary machines={machines} shown={visible.length} host={host} />}
            </section>{selected && <MachineDetails machine={selected} usage={usage.find(item => item.uuid === selected.uuid)} usageStale={stale} onEnableMemoryReporting={reporting.enable} reportingState={reporting.states[selected.uuid]} />}</div>}
        </Tabs.Panel>)}
      </Tabs.Root>
  </AppShell>
}
