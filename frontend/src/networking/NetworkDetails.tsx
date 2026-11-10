import { Button } from '@base-ui/react/button'
import type { NetworkingInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import type { Network } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/networks/models'
import { NetworkIcon } from '../machines/Icons'
import { addressLabel, modeLabel, vlanLabel, type GuestAttachment } from './model'

export function NetworkDetails({ network, data, attachments, onSelect, children }: {
  network: Network; data: NetworkingInventory; attachments: GuestAttachment[]; onSelect: (key: string) => void; children: ReactNode
}) {
  const hostBridge = data.interfaces.find(link => link.name === network.bridge)
  const guests = new Set(attachments.map(row => row.machine.uuid))
  const active = new Set(attachments.filter(row => row.machine.active).map(row => row.machine.uuid))
  const uplinks = [...new Set([network.forwardDevice, ...network.forwardInterfaces].filter(Boolean))]
  const attachmentsUnavailable = data.issues.some(issue => issue.source === 'machines')
  return <>
    <header className="network-detail-heading"><span className="machine-mark"><NetworkIcon /></span><div><span className="eyebrow">Libvirt network</span><h2>{network.name}</h2></div><span className="network-status" data-active={network.active}>{network.active ? 'Active' : 'Inactive'}</span><span className="badge">{modeLabel(network.forwardMode)}</span></header>
    <dl className="network-metrics">
      <div><dt>Host bridge</dt><dd>{hostBridge ? <Button className="text-link mono" onClick={() => onSelect('interface:' + hostBridge.name)}>{hostBridge.name}</Button> : <span className="mono">{network.bridge || 'Not configured'}</span>}</dd></div>
      <div><dt>Address configuration</dt><dd>{network.ips.length ? network.ips.map((ip, i) => <span className="mono" key={i}>{addressLabel(ip)}</span>) : 'None configured'}</dd></div>
      <div><dt>Assigned guests</dt><dd>{attachmentsUnavailable ? 'Not available' : `${guests.size} total · ${active.size} active`}</dd></div>
    </dl>
    {children}
    <section className="network-section"><h3>Configuration</h3><dl className="network-properties">
      <dt>Uplink interfaces</dt><dd>{uplinks.length ? uplinks.join(', ') : 'Not specified'}</dd>
      <dt>Definition</dt><dd>{network.persistent ? 'Persistent' : 'Transient'}</dd><dt>Autostart</dt><dd>{network.autostart ? 'Enabled' : 'Disabled'}</dd>
      <dt>Port isolation</dt><dd>{network.portIsolated === 'yes' ? 'Enabled' : network.portIsolated === 'no' ? 'Disabled' : 'Not specified'}</dd>
      <dt>VLAN</dt><dd>{vlanLabel(network.vlan)}</dd>
      {network.dnsDomain && <><dt>DNS domain</dt><dd>{network.dnsDomain}</dd></>}
      {network.virtualPortType && <><dt>Virtual port</dt><dd>{network.virtualPortType}</dd></>}
    </dl></section>
    {network.ips.some(ip => ip.dhcp) && <section className="network-section"><h3>DHCP configuration</h3>
      {network.ips.filter(ip => ip.dhcp).map((ip, i) => <div className="address-config" key={i}>
        <h4>{ip.family.toUpperCase()}</h4>
        {ip.dhcp!.ranges.map((range, j) => <p className="mono" key={j}>{range.start} – {range.end}</p>)}
        {!!ip.dhcp!.hosts.length && <><h4>Reservations</h4><ul className="plain-list">{ip.dhcp!.hosts.map((host, j) => <li key={j}><code>{host.ip}</code><span>{host.name || host.mac || host.id || 'Unnamed reservation'}</span></li>)}</ul></>}
        {!ip.dhcp!.ranges.length && !ip.dhcp!.hosts.length && <p className="secondary">No ranges or reservations configured.</p>}
      </div>)}
    </section>}
    {!!network.portGroups.length && <section className="network-section"><h3>Port groups</h3><div className="port-group-list">{network.portGroups.map(group => <div className="port-group" key={group.name}>
      <div><strong>{group.name}</strong>{group.default && <span className="badge">Default</span>}</div><span className="secondary">{vlanLabel(group.vlan)}</span>{group.virtualPortType && <span className="secondary">{group.virtualPortType}</span>}
    </div>)}</div></section>}
    <div className="network-identity"><span className="eyebrow">UUID</span><code>{network.uuid}</code></div>
  </>
}
import type { ReactNode } from 'react'
