// A clipboard read can outlive a connection. Its text belongs to the input
// generation that requested it, never to a subsequently reconnected guest.
export class InputGuard {
  private generation = 0
  private active = false
  private disposed = false
  get enabled() { return this.active && !this.disposed }
  setEnabled(enabled: boolean) { ++this.generation; this.active = enabled }
  dispose() { ++this.generation; this.disposed = true; this.active = false }
  async paste(read: () => Promise<string>, send: (text: string) => void) {
    if (!this.enabled) return
    const generation = this.generation
    const text = await read()
    if (this.enabled && generation === this.generation) send(text)
  }
}
