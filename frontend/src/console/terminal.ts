import { FitAddon, Ghostty, Terminal } from 'ghostty-web'
import wasmURL from 'ghostty-web/ghostty-vt.wasm?url'
import { InputGuard } from './input'

export interface TerminalAdapter {
  write(bytes: Uint8Array): void
  setInput(enabled: boolean): void
  focus(): void
  copy(): Promise<void>
  paste(): Promise<void>
  dispose(): void
}

let engine: Promise<Ghostty> | undefined
function loadEngine() {
  // Cache concurrent initialization, but allow a later retry after load failure.
  return engine ??= Ghostty.load(wasmURL).catch(error => { engine = undefined; throw error })
}

export async function createTerminal(root: HTMLElement, onInput: (data: Uint8Array) => void, signal?: AbortSignal): Promise<TerminalAdapter> {
  const ghostty = await loadEngine()
  if (signal?.aborted) throw new DOMException('Terminal initialization cancelled.', 'AbortError')
  const palette = getComputedStyle(document.documentElement)
  const terminal = new Terminal({ ghostty, cols: 80, rows: 24, scrollback: 1000,
    fontFamily: 'monospace', fontSize: 14, cursorBlink: true, disableStdin: true,
    theme: { background: palette.getPropertyValue('--page').trim() || '#171717',
      foreground: palette.getPropertyValue('--text').trim() || '#eee' },
  })
  const fit = new FitAddon()
  const guard = new InputGuard()
  let disposed = false
  try {
    terminal.open(root)
    terminal.loadAddon(fit)
    fit.fit()
    fit.observeResize()
    const input = terminal.onData(data => { if (guard.enabled) onInput(new TextEncoder().encode(data)) })
    const copy = async () => { const selection = terminal.getSelection(); if (selection) await navigator.clipboard.writeText(selection) }
    const paste = () => guard.paste(() => navigator.clipboard.readText(), text => terminal.paste(text))
    terminal.attachCustomKeyEventHandler(event => {
      if (event.type === 'keydown' && event.ctrlKey && event.shiftKey && !event.altKey && !event.metaKey) {
        if (event.code === 'KeyC' || event.code === 'KeyV') {
          event.preventDefault()
          void (event.code === 'KeyC' ? copy() : paste()).catch(() => { /* Toolbar reports clipboard errors. */ })
          return true
        }
      }
      // ghostty-web 0.4 treats true as handled (the reverse of xterm's API).
      return false
    })
    const dispose = () => { if (!disposed) { disposed = true; guard.dispose(); signal?.removeEventListener('abort', dispose); input.dispose(); terminal.dispose(); root.replaceChildren() } }
    signal?.addEventListener('abort', dispose, { once: true })
    return {
      write: bytes => { if (!disposed) terminal.write(bytes) },
      setInput: enabled => { if (!disposed) { guard.setEnabled(enabled); terminal.options.disableStdin = !enabled } },
      focus: () => { if (!disposed) terminal.focus() }, copy, paste,
      dispose,
    }
  } catch (error) { terminal.dispose(); root.replaceChildren(); throw error }
}
