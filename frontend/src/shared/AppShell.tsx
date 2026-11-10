import type { ReactNode } from 'react'
import { Button } from '@base-ui/react/button'
import { LayersIcon, MachineIcon, NetworkIcon } from '../machines/Icons'

export type Page = 'machines' | 'networking'
type Props = {
  page: Page; onNavigate?: (page: Page) => void; children: ReactNode
  loading: boolean; stale: boolean; updatedAt: Date | null; status?: string
}

export function AppShell({ page, onNavigate, children, loading, stale, updatedAt, status }: Props) {
  return <div className="app-shell">
    <aside className="app-sidebar" aria-label="Application">
      <div className="app-brand"><span className="brand-symbol"><LayersIcon /></span><span>Tiny Machine<br />Manager</span></div>
      <nav aria-label="Main navigation">
        <Button className="navigation-item" aria-current={page === 'machines' ? 'page' : undefined} onClick={() => onNavigate?.('machines')}><MachineIcon />Machines</Button>
        <Button className="navigation-item" aria-current={page === 'networking' ? 'page' : undefined} onClick={() => onNavigate?.('networking')}><NetworkIcon />Networking</Button>
      </nav>
      <div className="sidebar-connection"><span className="eyebrow">Connection</span><strong>Local host</strong><code>qemu:///system</code></div>
    </aside>
    <div className="workspace"><main className={'machines-page' + (page === 'networking' ? ' networking-page' : '')}>{children}</main>
      <footer className="workspace-footer"><span>Local host <span className="footer-separator">/</span> System QEMU/KVM</span><span>{loading ? 'Reading inventory…' : status ?? (stale ? 'Last loaded inventory · refresh failed' : updatedAt ? 'Updated ' + updatedAt.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' }) : 'Not connected')}</span></footer>
    </div>
  </div>
}
