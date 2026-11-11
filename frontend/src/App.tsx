import { useState } from 'react'
import { MachinesView } from './machines/MachinesView'
import { enableMemoryReporting, loadMachines } from './machines/api'
import { NetworkingView } from './networking/NetworkingView'
import { loadNetworking } from './networking/api'
import type { Page } from './shared/AppShell'
import { lazy, Suspense } from 'react'

const ConsoleView = lazy(() => import('./console/ConsoleView').then(module => ({ default: module.ConsoleView })))

export default function App() {
  const consoleUUID = new URLSearchParams(window.location.search).get('console')
  if (consoleUUID) return <Suspense fallback={<p role="status">Loading serial console…</p>}><ConsoleView uuid={consoleUUID} /></Suspense>
  return <InventoryApp />
}

function InventoryApp() {
  const [page, setPage] = useState<Page>('machines')
  return page === 'machines'
    ? <MachinesView loadMachines={loadMachines} enableMemoryReporting={enableMemoryReporting} onNavigate={setPage} />
    : <NetworkingView loadNetworking={loadNetworking} onNavigate={setPage} />
}
