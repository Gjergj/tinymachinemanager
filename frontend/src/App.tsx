import { useState } from 'react'
import { MachinesView } from './machines/MachinesView'
import { loadMachines } from './machines/api'
import { NetworkingView } from './networking/NetworkingView'
import { loadNetworking } from './networking/api'
import type { Page } from './shared/AppShell'

export default function App() {
  const [page, setPage] = useState<Page>('machines')
  return page === 'machines'
    ? <MachinesView loadMachines={loadMachines} onNavigate={setPage} />
    : <NetworkingView loadNetworking={loadNetworking} onNavigate={setPage} />
}
