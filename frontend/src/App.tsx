import { MachinesView } from './machines/MachinesView'
import { loadMachines } from './machines/api'

export default function App() {
  return <MachinesView loadMachines={loadMachines} />
}
