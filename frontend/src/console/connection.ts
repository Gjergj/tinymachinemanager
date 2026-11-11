export interface Credentials { id: string; url: string; token: string }
export interface ConsoleStatus {
  type: 'connecting' | 'connected' | 'disconnected' | 'error'
  message?: string
  // An authenticated stream or a terminal status from the server already has
  // backend cleanup. Its window may have been unregistered before this arrives.
  backendOwnsCleanup?: boolean
}
export interface ConsoleConnection { send(bytes: Uint8Array): void; close(): void }

export function connectConsole(credentials: Credentials, write: (bytes: Uint8Array) => void,
  status: (status: ConsoleStatus) => void, socketFactory = (url: string) => new WebSocket(url)): ConsoleConnection {
  const socket = socketFactory(credentials.url)
  socket.binaryType = 'arraybuffer'
  let closed = false
  let connected = false
  const fail = (message: string) => {
    if (closed) return
    const backendOwnsCleanup = connected
    closed = true; connected = false; socket.close()
    status({ type: 'error', message, backendOwnsCleanup })
  }
  socket.onopen = () => {
    if (closed) return
    socket.send(JSON.stringify({ type: 'auth', sessionID: credentials.id, token: credentials.token }))
    credentials.token = ''
  }
  socket.onmessage = event => {
    if (closed) return
    try {
      if (event.data instanceof ArrayBuffer) {
        if (!connected || event.data.byteLength > 32768) throw new Error('Invalid console output frame.')
        const bytes = new Uint8Array(event.data)
        write(bytes)
        socket.send(JSON.stringify({ type: 'ack', bytes: bytes.byteLength }))
      } else if (typeof event.data === 'string') {
        const control = JSON.parse(event.data)
        if (control.type === 'connected') { connected = true; status({ type: 'connected' }) }
        else if (control.type === 'error' || control.type === 'disconnected') {
          closed = true; connected = false; socket.close()
          status({ type: control.type, message: typeof control.message === 'string' ? control.message : undefined, backendOwnsCleanup: true })
        } else throw new Error('Invalid console control message.')
      } else throw new Error('Invalid console message.')
    } catch { fail('The console stream could not be processed. Reconnect to try again.') }
  }
  socket.onerror = () => fail('Could not connect to the local console service.')
  socket.onclose = () => {
    if (!closed) {
      const backendOwnsCleanup = connected
      closed = true; connected = false
      status({ type: 'disconnected', message: 'The console connection closed.', backendOwnsCleanup })
    }
  }
  return {
    send(bytes) {
      if (closed || !connected || socket.readyState !== WebSocket.OPEN || bytes.length === 0) return
      if (socket.bufferedAmount + bytes.length > 1024 * 1024) { fail('Serial input buffer is full. Reconnect before sending more input.'); return }
      try {
        for (let offset = 0; offset < bytes.length; offset += 16384) socket.send(new Uint8Array(bytes.subarray(offset, offset + 16384)).buffer)
      } catch { fail('The console disconnected while sending input. Input will not be replayed.') }
    },
    close() { closed = true; connected = false; socket.close() },
  }
}
