import type { SVGProps } from 'react'

type Props = SVGProps<SVGSVGElement>
function Icon({ children, ...props }: Props) {
  return <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.6" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true" {...props}>{children}</svg>
}
export function MachineIcon(props: Props) {
  return <Icon {...props}><rect x="3" y="4" width="18" height="13" rx="2" /><path d="M8 21h8M12 17v4" /></Icon>
}
export function LayersIcon(props: Props) {
  return <Icon {...props}><path d="m12 3 10 5-10 5L2 8l10-5ZM2 12l10 5 10-5M2 16l10 5 10-5" /></Icon>
}
export function NetworkIcon(props: Props) {
  return <Icon {...props}><rect x="9" y="3" width="6" height="5" rx="1" /><rect x="2" y="16" width="6" height="5" rx="1" /><rect x="16" y="16" width="6" height="5" rx="1" /><path d="M12 8v4M5 16v-4h14v4" /></Icon>
}
export function InfoIcon(props: Props) {
  return <Icon {...props}><circle cx="12" cy="12" r="9" /><path d="M12 11v6M12 7h.01" /></Icon>
}
