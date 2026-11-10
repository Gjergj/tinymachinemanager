import type { Domain } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'
import type { Resources } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/hostinfo/models'

export function ResourceSummary({ machines, shown, host }: { machines: Domain[]; shown: number; host: Resources }) {
  const vcpus = machines.reduce((sum, machine) => sum + machine.vcpus, 0)
  // An unreported allocation must not make the aggregate look smaller.
  const memory = machines.every(machine => machine.memoryKiB > 0)
    ? machines.reduce((sum, machine) => sum + machine.memoryKiB, 0) : null
  const unit = Math.max(memory ?? 0, host.memoryKiB) >= 1048576 ? 'GiB' : 'MiB'
  const divisor = unit === 'GiB' ? 1048576 : 1024
  const number = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 })
  const memoryRatio = `${memory === null ? '—' : number.format(memory / divisor)} / ${host.memoryKiB ? number.format(host.memoryKiB / divisor) : '—'} ${unit}`

  return <div className="inventory-summary" aria-label="Host resource assignments">
    <span><strong>{shown}</strong> {shown === 1 ? 'machine' : 'machines'} shown</span>
    <span title="Assigned memory across all VMs, including stopped VMs, / total host memory. — means an allocation or capacity was not reported."><strong>{memoryRatio}</strong> memory assigned</span>
    <span title="Assigned vCPUs across all VMs, including stopped VMs, / online host logical CPUs."><strong>{number.format(vcpus)} / {host.logicalCPUs ? number.format(host.logicalCPUs) : '—'}</strong> vCPUs assigned</span>
  </div>
}
