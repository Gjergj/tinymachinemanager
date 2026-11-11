import { useEffect, useRef, useState } from 'react'
import type { ConsoleAPI, ConsoleCapabilities } from './api'
import { connectConsole, type ConsoleConnection, type ConsoleStatus } from './connection'
import { createTerminal, type TerminalAdapter } from './terminal'

export function useConsole(uuid: string, api: ConsoleAPI, makeTerminal = createTerminal, connect = connectConsole) {
  const root = useRef<HTMLDivElement>(null)
  const serial = useRef<Promise<void>>(Promise.resolve())
  const commands = useRef<{ reconnect(key?: string): void; disconnect(): void; copy(): void; paste(): void }>()
  const [capabilities, setCapabilities] = useState<ConsoleCapabilities>()
  const [selected, setSelected] = useState('')
  const [status, setStatus] = useState<ConsoleStatus>({ type: 'connecting', message: 'Loading terminal…' })
  const [busy, setBusy] = useState(true)
  const [initialized, setInitialized] = useState(false)
  const [receivedOutput, setReceivedOutput] = useState(false)
  const [clipboardError, setClipboardError] = useState('')
  const [retry, setRetry] = useState(0)

  useEffect(() => {
    let alive = true
    let attempt = 0
    let terminal: TerminalAdapter | undefined
    let connection: ConsoleConnection | undefined
    let sessionID: string | undefined
    const abort = new AbortController()
    setBusy(true); setInitialized(false)
    const release = async (notifyBackend = true) => {
      const id = sessionID
      sessionID = undefined; connection?.close(); connection = undefined; terminal?.setInput(false)
      if (id && notifyBackend) await api.disconnect(id).catch(() => { /* WebSocket closure also releases the backend session. */ })
    }
    const enqueue = (task: () => Promise<void>) => { serial.current = serial.current.catch(() => {}).then(task) }
    const reconnect = (key?: string) => {
      const current = ++attempt
      setBusy(true); setReceivedOutput(false); setStatus({ type: 'connecting', message: 'Connecting to serial console…' })
      enqueue(async () => {
        await release()
        if (!alive || current !== attempt) return
        try {
          const caps = await api.capabilities(uuid)
          if (!alive || current !== attempt) return
          setCapabilities(caps)
          if (!caps.available) throw new Error(caps.reason)
          const device = key ? caps.devices.find(d => d.key === key && d.supported) : caps.devices.find(d => d.supported)
          if (!device) { setSelected(''); throw new Error('The selected device changed. Select an available console and reconnect.') }
          setSelected(device.key)
          const credentials = await api.create(uuid, device.key, caps.revision)
          if (!alive || current !== attempt) { await api.disconnect(credentials.id).catch(() => {}); return }
          sessionID = credentials.id
          connection = connect(credentials, bytes => {
            if (alive && current === attempt) { terminal!.write(bytes); setReceivedOutput(true) }
          }, update => {
            if (!alive || current !== attempt) return
            setStatus(update); setBusy(false); terminal?.setInput(update.type === 'connected')
            if (update.type === 'connected') terminal?.focus()
            else if (update.type === 'error' || update.type === 'disconnected') {
              // Native window close already releases this stream. Calling a
              // binding now races Wails removing the caller's window ID.
              enqueue(() => release(!update.backendOwnsCleanup))
            }
          })
        } catch (error) {
          await release()
          if (alive && current === attempt) { setBusy(false); setStatus({ type: 'error', message: String(error instanceof Error ? error.message : error) }) }
        }
      })
    }
    commands.current = {
      reconnect,
      disconnect() {
        ++attempt; setBusy(true); setStatus({ type: 'connecting', message: 'Disconnecting…' })
        connection?.close(); terminal?.setInput(false)
        enqueue(async () => { await release(); if (alive) { setBusy(false); setStatus({ type: 'disconnected', message: 'Disconnected. The guest shell may still be running.' }) } })
      },
      copy() { setClipboardError(''); void terminal?.copy().catch(() => { if (alive) setClipboardError('Could not copy the terminal selection.') }) },
      paste() { setClipboardError(''); void terminal?.paste().catch(() => { if (alive) setClipboardError('Clipboard access was denied. Use the terminal’s paste shortcut or context menu.') }) },
    }
    void makeTerminal(root.current!, bytes => connection?.send(bytes), abort.signal).then(value => {
      if (!alive) { value.dispose(); return }
      terminal = value; setInitialized(true)
      if (attempt === 0) reconnect()
    }).catch(error => {
      if (alive && attempt === 0) { setBusy(false); setStatus({ type: 'error', message: `Could not initialize the terminal: ${String(error)}` }) }
    })
    return () => {
      alive = false; ++attempt; abort.abort(); connection?.close(); terminal?.dispose(); enqueue(release)
    }
  }, [uuid, api, makeTerminal, connect, retry])

  return { root, capabilities, selected, setSelected, status, busy, initialized, receivedOutput, clipboardError,
    reconnect: () => initialized ? commands.current?.reconnect(selected || undefined) : setRetry(value => value + 1),
    disconnect: () => commands.current?.disconnect(), copy: () => commands.current?.copy(), paste: () => commands.current?.paste(),
  }
}
