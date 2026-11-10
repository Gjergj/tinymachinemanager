import type { Domain } from '../../bindings/github.com/Gjergj/tinymachinemanager/pkg/domains/models'

const stateLabels: Record<string, string> = {
  running: 'Running', paused: 'Paused', blocked: 'Blocked',
  shutdown: 'Shutting down', shutoff: 'Shut off', crashed: 'Crashed',
  pmsuspended: 'Suspended', unknown: 'Unknown',
}
export function stateLabel(state: Domain['state']) {
  return stateLabels[state] ?? 'Unknown'
}
export function memoryLabel(kib: number) {
  if (!kib) return 'Not reported'
  const gib = kib / 1024 / 1024
  const number = new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 })
  return gib >= 1 ? number.format(gib) + ' GiB' : number.format(kib / 1024) + ' MiB'
}
