export function createRandomId(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID()
  }
  if (typeof globalThis.crypto?.getRandomValues === 'function') {
    const words = globalThis.crypto.getRandomValues(new Uint32Array(4))
    return Array.from(words, (word) => word.toString(16).padStart(8, '0')).join('-')
  }
  return `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`
}
