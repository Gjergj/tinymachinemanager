import { expect, it, vi } from 'vitest'
import { InputGuard } from './input'

it('discards delayed clipboard text after disconnect/reconnect or disposal', async () => {
  const guard = new InputGuard()
  const paste = vi.fn()
  let finish!: (text: string) => void
  guard.setEnabled(true)
  const pending = guard.paste(() => new Promise<string>(resolve => { finish = resolve }), paste)
  guard.setEnabled(false); guard.setEnabled(true)
  finish('old input'); await pending
  expect(paste).not.toHaveBeenCalled()
  await guard.paste(async () => 'new input', paste)
  expect(paste).toHaveBeenCalledWith('new input')
  const late = guard.paste(() => new Promise<string>(resolve => { finish = resolve }), paste)
  guard.dispose(); finish('disposed input'); await late
  expect(paste).toHaveBeenCalledTimes(1)
})
