import { Button } from '@base-ui/react/button'
import { Select } from '@base-ui/react/select'
import { consoleAPI, type ConsoleAPI } from './api'
import { connectConsole } from './connection'
import { createTerminal } from './terminal'
import { useConsole } from './useConsole'

export function ConsoleView({ uuid, api = consoleAPI, makeTerminal = createTerminal, connect = connectConsole }: {
  uuid: string; api?: ConsoleAPI; makeTerminal?: typeof createTerminal; connect?: typeof connectConsole
}) {
  const c = useConsole(uuid, api, makeTerminal, connect)
  const connected = c.status.type === 'connected'
  return <main className="console-window">
    <header className="console-heading"><div><span className="eyebrow">Serial console</span><h1>{c.capabilities?.name || 'Virtual machine'}</h1></div>
      <span className="machine-state" data-state={c.status.type}>{c.status.type === 'connecting' ? 'Connecting' : connected ? 'Connected' : c.status.type === 'error' ? 'Connection unavailable' : 'Disconnected'}</span>
    </header>
    <div className="console-toolbar">
      {c.capabilities && c.capabilities.devices.length > 1 && <Select.Root value={c.selected} onValueChange={value => { if (typeof value === 'string') c.setSelected(value) }} disabled={c.busy || connected}>
        <Select.Trigger className="console-select" aria-label="Serial device"><Select.Value>{c.capabilities.devices.find(d => d.key === c.selected)?.label || 'Select a console'}</Select.Value><Select.Icon>⌄</Select.Icon></Select.Trigger>
        <Select.Portal><Select.Positioner sideOffset={4}><Select.Popup className="console-select-popup"><Select.List>{c.capabilities.devices.map(device => <Select.Item className="console-select-item" key={device.key} value={device.key} disabled={!device.supported}><Select.ItemText>{device.label} · {device.backend}</Select.ItemText></Select.Item>)}</Select.List></Select.Popup></Select.Positioner></Select.Portal>
      </Select.Root>}
      <Button onClick={c.disconnect} disabled={!c.busy && !connected}>Disconnect</Button>
      <Button onClick={c.reconnect} disabled={c.busy || connected}>{c.initialized ? 'Reconnect' : 'Retry terminal'}</Button>
      <span className="console-toolbar-spacer" />
      <Button onClick={c.copy} disabled={!c.initialized}>Copy</Button><Button onClick={c.paste} disabled={!connected}>Paste</Button>
    </div>
    <div className="console-notice" role={c.status.type === 'error' ? 'alert' : 'status'}>
      {c.status.message || (connected ? 'Connected to the guest serial port.' : 'Ready to connect.')}
      {connected && !c.receivedOutput && <span> Press Enter to request a prompt. A login service must be enabled in the guest.</span>}
      {c.capabilities?.state === 'paused' && <span> This VM is paused; guest input and output may not progress.</span>}
    </div>
    {c.clipboardError && <p className="console-notice" role="alert">{c.clipboardError}</p>}
    <div className="console-terminal" ref={c.root} />
    <footer className="console-footer">Ctrl+C interrupts the guest · Ctrl+Shift+C / V copies / pastes · Resizing fits this view; guest terminal dimensions are managed inside the guest.</footer>
  </main>
}
