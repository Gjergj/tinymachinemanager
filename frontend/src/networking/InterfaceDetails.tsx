import { Button } from '@base-ui/react/button'
import type { NetworkingInventory } from '../../bindings/github.com/Gjergj/tinymachinemanager/models'
import type { Interface } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/hostnet/models'
import { InterfaceIcon } from '../machines/Icons'
import { linkKind } from './model'

export function InterfaceDetails({ link, data, onSelect, children }: { link: Interface; data: NetworkingInventory; onSelect: (key: string) => void; children: ReactNode }) {
  const relatedNetworks = data.networks.filter(network => network.bridge === link.name)
  const reference = (name: string) => data.interfaces.some(iface => iface.name === name)
    ? <Button className="text-link mono" onClick={() => onSelect('interface:' + name)}>{name}</Button> : <code>{name}</code>
  return <>
    <header className="network-detail-heading"><span className="machine-mark"><InterfaceIcon /></span><div><span className="eyebrow">{linkKind(link)}</span><h2>{link.name}</h2></div><span className="network-status" data-active={link.up}>{link.up ? 'Up' : 'Down'}</span></header>
    <dl className="network-metrics">
      <div><dt>Host addresses</dt><dd>{link.addresses.length ? link.addresses.map(address => <span className="mono" key={address}>{address}</span>) : 'None assigned'}</dd></div>
      <div><dt>Operational state</dt><dd>{link.operState || 'Not reported'}</dd></div>
      <div><dt>Member interfaces</dt><dd>{link.members.length}</dd></div>
    </dl>
    {children}
    <section className="network-section"><h3>Interface configuration</h3><dl className="network-properties">
      <dt>MAC address</dt><dd className="mono">{link.mac || 'Not reported'}</dd><dt>MTU</dt><dd>{link.mtu}</dd>
      <dt>Administrative state</dt><dd>{link.up ? 'Up' : 'Down'}</dd><dt>Running</dt><dd>{link.running ? 'Yes' : 'No'}</dd>
      <dt>Master interface</dt><dd>{link.master ? reference(link.master) : 'None'}</dd><dt>Interface index</dt><dd>{link.index}</dd>
    </dl></section>
    {!!link.members.length && <section className="network-section"><h3>Member interfaces</h3><ul className="interface-reference-list">{link.members.map(member => <li key={member}><InterfaceIcon />{reference(member)}</li>)}</ul></section>}
    {!!relatedNetworks.length && <section className="network-section"><h3>Libvirt networks using this bridge</h3><div className="interface-reference-list">{relatedNetworks.map(network => <Button className="text-link" key={network.uuid} onClick={() => onSelect('network:' + network.uuid)}>{network.name}</Button>)}</div></section>}
  </>
}
import type { ReactNode } from 'react'
