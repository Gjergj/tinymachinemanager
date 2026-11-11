import { describe, expect, it } from 'vitest'
import { connectConsole, type ConsoleStatus } from './connection'

class Socket {
  readyState = 1; bufferedAmount = 0; binaryType = ''; closed = false
  onopen: (() => void) | null = null
  onmessage: ((e: { data: unknown }) => void) | null = null
  onerror: (() => void) | null = null
  onclose: (() => void) | null = null
  sent: (string | ArrayBuffer)[] = []
  send(data: string | ArrayBuffer) { this.sent.push(data) }
  close() { this.closed = true }
}
const credentials = { id: 'session', token: 'secret', url: 'ws://127.0.0.1:1234/serial' }

describe('serial WebSocket client', () => {
  it('still requests credential cleanup when transport fails before attachment', () => {
    const socket = new Socket(); const states: ConsoleStatus[] = []
    connectConsole({ ...credentials }, () => {}, status => states.push(status), () => socket as unknown as WebSocket)
    socket.onerror!()
    expect(states[states.length - 1]).toMatchObject({ type: 'error', backendOwnsCleanup: false })
  })
  it('leaves established connection cleanup to the backend when the window socket closes', () => {
    const socket = new Socket(); const states: ConsoleStatus[] = []
    connectConsole({ ...credentials }, () => {}, status => states.push(status), () => socket as unknown as WebSocket)
    socket.onmessage!({ data: '{"type":"connected"}' })
    socket.onclose!()
    expect(states[states.length - 1]).toMatchObject({ type: 'disconnected', backendOwnsCleanup: true })
  })
  it('authenticates before input and acknowledges bytes after terminal consumption', () => {
    const socket = new Socket(); const output: number[][] = []; const states: string[] = []
    const client = connectConsole(credentials, bytes => output.push([...bytes]), state => states.push(state.type), () => socket as unknown as WebSocket)
    client.send(new Uint8Array([3])); expect(socket.sent).toEqual([])
    socket.onopen!()
    expect(JSON.parse(socket.sent[0] as string)).toEqual({ type: 'auth', sessionID: 'session', token: 'secret' })
    socket.onmessage!({ data: '{"type":"connected"}' })
    socket.onmessage!({ data: new Uint8Array([27, 91, 51, 49, 109, 195, 169]).buffer })
    expect(output).toEqual([[27, 91, 51, 49, 109, 195, 169]])
    expect(socket.sent[1]).toBe('{"type":"ack","bytes":7}')
    client.send(new Uint8Array([3]))
    expect([...new Uint8Array(socket.sent[2] as ArrayBuffer)]).toEqual([3])
    expect(states).toEqual(['connected'])
    client.close(); socket.onmessage!({ data: new Uint8Array([1]).buffer })
    expect(output).toHaveLength(1)
  })

  it('closes on input backpressure and malformed controls without replay', () => {
    const socket = new Socket(); const states: string[] = []
    const client = connectConsole(credentials, () => {}, status => states.push(status.type), () => socket as unknown as WebSocket)
    socket.onopen!(); socket.onmessage!({ data: '{"type":"connected"}' })
    socket.bufferedAmount = 1024 * 1024
    client.send(new Uint8Array([1]))
    expect(socket.closed).toBe(true); expect(states[states.length - 1]).toBe('error')
    expect(socket.sent).toHaveLength(1)
  })
})
