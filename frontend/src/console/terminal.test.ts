// @vitest-environment node
import { describe, expect, it } from 'vitest'
import { Ghostty } from 'ghostty-web'
import { resolve } from 'node:path'
import { readFile } from 'node:fs/promises'

describe('bundled Ghostty engine', () => {
  it('loads its matching WASM and preserves fragmented terminal bytes', async () => {
    const wasm = await readFile(resolve('node_modules/ghostty-web/ghostty-vt.wasm'))
    const ghostty = await Ghostty.load(`data:application/wasm;base64,${wasm.toString('base64')}`)
    const terminal = ghostty.createTerminal(80, 24)
    try {
      const bytes = new TextEncoder().encode('\x1b[31mRocky café\x1b[0m')
      for (const byte of bytes) terminal.write(new Uint8Array([byte]))
      const text = terminal.getLine(0)!.map(cell => cell.codepoint ? String.fromCodePoint(cell.codepoint) : '').join('')
      expect(text).toContain('Rocky café')
    } finally { terminal.free() }
  })
})
