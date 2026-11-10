import type { Domain, Interface } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'

export function DHCPAddresses({ nic, index, active, dhcp }: { nic: Interface; index: number; active: boolean; dhcp: Domain['dhcp'] }) {
  if (!active) return <span className="secondary">VM is stopped</span>
  if (nic.type !== 'network' || !nic.source.network || !nic.mac) return <span className="secondary">Not available</span>
  const entry = dhcp?.interfaces.find(item => item.index === index && item.mac.toLowerCase() === nic.mac?.trim().toLowerCase())
  const error = dhcp?.error || entry?.error
  if (error) return <span className="secondary" title={error}>Lease data unavailable</span>
  if (!entry) return <span className="secondary">Not reported</span>
  if (!entry.addresses.length) return <span className="secondary">No lease reported</span>
  return <span className="dhcp-addresses">{entry.addresses.map((address, i) =>
    <code key={address.family + ':' + address.address + ':' + address.prefix + ':' + i}>{address.address}/{address.prefix}</code>)}</span>
}
