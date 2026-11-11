import type { Attachment, Domain, Interface } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import { MachineIcon, NetworkIcon } from './Icons'
import { firmwareLabel, hypervisorLabel, memoryLabel, stateLabel } from './format'
import { DHCPAddresses } from './DHCPAddresses'
import { UsageDetails } from './UsageDetails'
import type { GuestUsage } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/usage/models'
import type { EnableMemoryReporting } from './api'
import type { MemoryReportingState } from './useMemoryReporting'
import { SerialConsoleAction } from '../console/SerialConsoleAction'

function sourceName(attachment: Attachment) {
  const source = attachment.source
  return source.network || source.bridge || source.device || source.address ||
    source.path || (source.deviceAddress ? 'Host device' : '') ||
    (attachment.type === 'user' ? 'User-mode networking' : 'Unspecified source')
}
function vlanLabel(attachment: Attachment) {
  return attachment.vlan?.tags.map(tag => 'VLAN ' + tag.id + (tag.nativeMode ? ' (' + tag.nativeMode + ')' : '')).join(', ')
}
function InterfaceDetails({ nic, index, active, dhcp }: { nic: Interface; index: number; active: boolean; dhcp: Domain['dhcp'] }) {
  const configuredVLAN = vlanLabel(nic)
  const actualVLAN = nic.actual ? vlanLabel(nic.actual) : undefined
  return <section className="nic-details" aria-label={'Network interface ' + (index + 1)}>
    <div className="nic-heading"><h4><NetworkIcon />NIC {index + 1}</h4><span className="badge">{nic.model || 'Model unspecified'}</span></div>
    <strong className="nic-source">{sourceName(nic)}</strong>
    <span className="secondary nic-kind">{nic.type || 'Unspecified interface type'}</span>
    <dl className="detail-list">
      <dt>MAC address</dt><dd className="mono">{nic.mac || 'Not reported'}</dd>
      <dt>DHCP addresses</dt><dd><DHCPAddresses nic={nic} index={index} active={active} dhcp={dhcp} /></dd>
      {nic.source.portGroup && <><dt>Port group</dt><dd>{nic.source.portGroup}</dd></>}
      {configuredVLAN && <><dt>VLAN</dt><dd>{configuredVLAN}{nic.vlan?.trunk ? ' · trunk' : ''}</dd></>}
      {nic.source.mode && <><dt>Source mode</dt><dd>{nic.source.mode}</dd></>}
      <dt>Link setting</dt><dd>{nic.linkState === 'down' ? 'Down' : nic.linkState === 'up' ? 'Up' : nic.linkState || 'Up (default)'}</dd>
      {nic.target && <><dt>{active ? 'Host device' : 'Defined target'}</dt><dd className="mono">{nic.target}</dd></>}
      {nic.portIsolated && <><dt>Port isolation</dt><dd>{nic.portIsolated}</dd></>}
      {nic.filter && <><dt>Network filter</dt><dd>{nic.filter}</dd></>}
    </dl>
    {nic.actual && <div className="resolved-attachment">
      <span className="eyebrow">Resolved attachment</span><strong>{sourceName(nic.actual)}</strong>
      <span className="secondary">{nic.actual.type}</span>
      {actualVLAN && <span>{actualVLAN}{nic.actual.vlan?.trunk ? ' · trunk' : ''}</span>}
      {nic.actual.portIsolated && <span className="secondary">Port isolation: {nic.actual.portIsolated}</span>}
    </div>}
  </section>
}
export function MachineDetails({ machine, usage, usageStale, onEnableMemoryReporting, reportingState }: { machine: Domain; usage?: GuestUsage; usageStale?: boolean; onEnableMemoryReporting?: EnableMemoryReporting; reportingState?: MemoryReportingState }) {
  return <aside className="machine-details" aria-label="Selected machine">
    <div className="selected-heading"><span className="machine-mark"><MachineIcon /></span><div><h2>{machine.name}</h2><span className="machine-state" data-state={machine.state}>{stateLabel(machine.state)}</span></div></div>
    <SerialConsoleAction key={'serial-' + machine.uuid} machine={machine} />
    <UsageDetails key={machine.uuid} machine={machine} usage={usage} stale={usageStale} onEnableMemoryReporting={onEnableMemoryReporting} reportingState={reportingState} />
    <section className="platform-details" aria-label="Platform"><h3 className="eyebrow">Platform</h3>
      <dl className="detail-list">
        <dt>Hypervisor</dt><dd>{hypervisorLabel(machine.type)}</dd>
        <dt>Architecture</dt><dd className="mono">{machine.os.type.arch || 'Not reported'}</dd>
        <dt>Emulator</dt><dd className="mono">{machine.devices.emulator || 'Not reported'}</dd>
        <dt>Firmware</dt><dd>{firmwareLabel(machine.os)}{machine.os.loader?.path && <code className="firmware-path">{machine.os.loader.path}</code>}</dd>
      </dl>
    </section>
    <section><h3 className="eyebrow">Assigned resources</h3>
      <dl className="detail-list"><dt>Virtual CPUs</dt><dd>{machine.vcpus}</dd><dt>Memory</dt><dd>{memoryLabel(machine.memoryKiB)}</dd><dt>Maximum memory</dt><dd>{memoryLabel(machine.maxMemoryKiB)}</dd><dt>Definition</dt><dd>{machine.persistent ? 'Persistent' : 'Transient'}</dd></dl>
    </section>
    <section className="network-details"><div className="section-heading"><h3>Network interfaces</h3><span className="badge">{machine.devices.interfaces.length}</span></div>
      {machine.devices.interfaces.length ? machine.devices.interfaces.map((nic, index) => <InterfaceDetails key={(nic.mac || nic.alias || 'nic') + '-' + index} nic={nic} index={index} active={machine.active} dhcp={machine.dhcp} />) : <p className="secondary no-interfaces">No network interfaces defined.</p>}
    </section>
    <div className="machine-identity"><span className="eyebrow">UUID</span><code>{machine.uuid}</code></div>
  </aside>
}
