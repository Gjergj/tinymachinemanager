import { useEffect, useState } from 'react'
import { Button } from '@base-ui/react/button'
import { Tabs } from '@base-ui/react/tabs'
import type { NetworkingInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import { InfoIcon, InterfaceIcon, NetworkIcon } from '../machines/Icons'
import { AppShell, type Page } from '../shared/AppShell'
import { AttachedMachines } from './AttachedMachines'
import { InterfaceDetails } from './InterfaceDetails'
import { NetworkDetails } from './NetworkDetails'
import { attachmentsFor, explorerItems, linkKind, modeLabel, type ExplorerItem, type Scope } from './model'
import { useNetworking, type LoadNetworking } from './useNetworking'

const scopes: { value: Scope; label: string }[] = [{ value: 'all', label: 'All' }, { value: 'libvirt', label: 'Libvirt' }, { value: 'host', label: 'Host' }]
const empty: NetworkingInventory = { networks: [], interfaces: [], machines: [], issues: [] }
const sourceLabels: Record<string, string> = { networks: 'Libvirt networks', interfaces: 'Host interfaces', machines: 'Guest attachments' }

function EmptyState({ title, message }: { title: string; message: string }) {
  return <div className="inventory-empty" role="status"><span className="empty-icon"><NetworkIcon /></span><h2>{title}</h2><p>{message}</p></div>
}

function PickerGroup({ title, items, unavailable, selectedKey, onSelect }: {
  title: string; items: ExplorerItem[]; unavailable: boolean; selectedKey?: string; onSelect: (key: string) => void
}) {
  return <section className="network-picker-group"><h2 className="eyebrow">{title}</h2>
    {items.map(item => <Button className="network-picker-item" key={item.key} aria-pressed={selectedKey === item.key} onClick={() => onSelect(item.key)}>
      {item.kind === 'network' ? <NetworkIcon /> : <InterfaceIcon />}<span>
        <strong>{item.kind === 'network' ? item.network.name : item.link.name}</strong>{' '}
        <small>{item.kind === 'network' ? modeLabel(item.network.forwardMode) + ' · ' + (item.network.active ? 'Active' : 'Inactive') : linkKind(item.link) + ' · ' + (item.link.up ? 'Up' : 'Down')}</small>
      </span>
    </Button>)}
    {!items.length && <p className="picker-empty">{unavailable ? 'Unavailable' : 'None found'}</p>}
  </section>
}

export function NetworkingView({ loadNetworking, onNavigate }: { loadNetworking: LoadNetworking; onNavigate?: (page: Page) => void }) {
  const { inventory, loading, error, updatedAt } = useNetworking(loadNetworking)
  const data = inventory ?? empty
  const [scope, setScope] = useState<Scope>('all')
  const [selectedKey, setSelectedKey] = useState<string | null>(null)
  const items = explorerItems(data, scope)
  const selected = items.find(item => item.key === selectedKey) ?? items[0]
  const visibleSelection = selected?.key ?? null
  useEffect(() => { setSelectedKey(visibleSelection) }, [visibleSelection])
  const unavailable = new Set(data.issues.map(issue => issue.source))
  const scopeUnavailable = scope === 'libvirt' ? unavailable.has('networks') : scope === 'host' ? unavailable.has('interfaces') : unavailable.has('networks') || unavailable.has('interfaces')
  const emptyTitle = scopeUnavailable
    ? (scope === 'libvirt' ? 'Libvirt networks unavailable' : scope === 'host' ? 'Host interfaces unavailable' : 'Networking unavailable')
    : (scope === 'libvirt' ? 'No libvirt networks' : scope === 'host' ? 'No host interfaces' : 'No networking data')
  const stale = Boolean(error && updatedAt) || Boolean(data.issues.length)
  const counts: Record<Scope, number | null> = {
    all: unavailable.has('networks') || unavailable.has('interfaces') ? null : data.networks.length + data.interfaces.length,
    libvirt: unavailable.has('networks') ? null : data.networks.length,
    host: unavailable.has('interfaces') ? null : data.interfaces.length,
  }
  const onSelect = (key: string) => {
    setSelectedKey(key)
    if (key.startsWith('network:') && scope === 'host') setScope('libvirt')
    if (key.startsWith('interface:') && scope === 'libvirt') setScope('host')
  }
  const attachments = selected ? attachmentsFor(selected, data) : []
  // Host associations can require the network-to-bridge mapping even when the
  // domain inventory itself succeeded. Known explicit matches remain visible.
  const attachmentDataIncomplete = unavailable.has('machines') || (selected?.kind === 'interface' && unavailable.has('networks'))
  const guestAttachments = <AttachedMachines attachments={attachments} unavailable={attachmentDataIncomplete} />
  return <AppShell page="networking" onNavigate={onNavigate} loading={loading} stale={stale} updatedAt={updatedAt} status={data.issues.length ? 'Partial inventory · retrying' : undefined}>
    <header className="page-heading"><h1>Networking</h1></header>
    {(error || data.issues.length > 0) && <div className="connection-error" role="alert"><InfoIcon /><div>
      <h2>{error ? "Couldn't refresh networking" : 'Some networking data is unavailable'}</h2>
      <p>Available sources remain visible. Failed sources keep their last loaded data and retry automatically.</p>
      {error && <p className="error-message">{error}</p>}
      {!!data.issues.length && <ul className="source-issues">{data.issues.map(issue => <li key={issue.source}><strong>{sourceLabels[issue.source] ?? issue.source}</strong><span className="error-message">{issue.message}</span></li>)}</ul>}
    </div></div>}
    <Tabs.Root value={scope} onValueChange={value => { if (value === 'all' || value === 'libvirt' || value === 'host') setScope(value) }}>
      <div className="inventory-toolbar"><Tabs.List className="inventory-tabs" aria-label="Network source">
        {scopes.map(item => <Tabs.Tab key={item.value} className="inventory-tab" value={item.value}>{item.label}<span className="tab-count">{inventory ? counts[item.value] ?? '—' : '—'}</span></Tabs.Tab>)}
      </Tabs.List></div>
      {scopes.map(item => <Tabs.Panel className="inventory-panel" key={item.value} value={item.value}>
        {loading && !inventory ? <EmptyState title="Loading networking…" message="Reading libvirt networks and local host interfaces." /> :
          !inventory ? <EmptyState title="Networking unavailable" message="The connection will be retried automatically." /> :
            <div className="network-explorer" aria-busy={loading}>
              <aside className="network-picker" aria-label="Network picker">
                {scope !== 'host' && <PickerGroup title="Libvirt networks" items={items.filter(entry => entry.kind === 'network')} unavailable={unavailable.has('networks')} selectedKey={selected?.key} onSelect={onSelect} />}
                {scope !== 'libvirt' && <>
                  <PickerGroup title="Linux bridges" items={items.filter(entry => entry.kind === 'interface' && entry.link.isBridge)} unavailable={unavailable.has('interfaces')} selectedKey={selected?.key} onSelect={onSelect} />
                  <PickerGroup title="Host interfaces" items={items.filter(entry => entry.kind === 'interface' && !entry.link.isBridge)} unavailable={unavailable.has('interfaces')} selectedKey={selected?.key} onSelect={onSelect} />
                </>}
              </aside>
              <section className="network-detail" aria-label="Selected network">
                {selected ? <>
                  {selected.kind === 'network' ? <NetworkDetails network={selected.network} data={data} attachments={attachments} onSelect={onSelect}>{guestAttachments}</NetworkDetails> : <InterfaceDetails link={selected.link} data={data} onSelect={onSelect}>{guestAttachments}</InterfaceDetails>}
                </> : <EmptyState title={emptyTitle} message={scopeUnavailable ? 'Some sources are unavailable. The inventory will retry automatically.' : 'No networks or host interfaces were reported in this view.'} />}
              </section>
            </div>}
      </Tabs.Panel>)}
    </Tabs.Root>
  </AppShell>
}
