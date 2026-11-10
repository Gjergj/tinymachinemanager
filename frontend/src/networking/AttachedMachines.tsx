import { stateLabel } from '../machines/format'
import { vlanLabel, type GuestAttachment } from './model'

export function AttachedMachines({ attachments, unavailable }: { attachments: GuestAttachment[]; unavailable: boolean }) {
  return <section className="network-section" aria-label="Attached virtual machines">
    <div className="section-heading"><h3>Guest attachments</h3><span className="badge">{unavailable ? 'Partial inventory' : attachments.length + (attachments.length === 1 ? ' interface' : ' interfaces')}</span></div>
    {!attachments.length ? <p className="network-empty-text secondary">{unavailable ? 'Attachment inventory incomplete' : 'No guest attachments'}</p> :
      <div className="machine-table-scroll"><table className="machine-table attachment-table">
        <caption className="sr-only">Guest network attachments</caption>
        <thead><tr><th scope="col">Virtual machine</th><th scope="col">State</th><th scope="col">NIC</th><th scope="col">Port group / VLAN</th><th scope="col">Attachment</th></tr></thead>
        <tbody>{attachments.map(({ machine, nic, index, via }) => <tr key={machine.uuid + ':' + index}>
          <th scope="row">{machine.name}</th>
          <td><span className="machine-state" data-state={machine.state}>{stateLabel(machine.state)}</span></td>
          <td><span>NIC {index + 1}</span><code>{nic.mac || 'MAC not reported'}</code>{nic.model && <span className="secondary">{nic.model}</span>}</td>
          <td>{nic.source.portGroup && <span>{nic.source.portGroup}</span>}<span className="secondary">{vlanLabel(nic.actual?.vlan ?? nic.vlan)}</span>{nic.linkState === 'down' && <span className="secondary">Link down</span>}</td>
          <td><span>{via}</span>{nic.target && <span><span className="secondary">{machine.active ? 'Host device' : 'Defined target'} </span><code>{nic.target}</code></span>}</td>
        </tr>)}</tbody>
      </table></div>}
  </section>
}
