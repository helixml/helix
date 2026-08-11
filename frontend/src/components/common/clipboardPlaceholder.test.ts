import { describe, expect, it } from 'vitest'

import {
  isPlaceholderPng,
  PLACEHOLDER_PNG_BASE64,
  PLACEHOLDER_PNG_BYTE_LENGTH,
} from './clipboardPlaceholder'

const placeholderBytes = (): Uint8Array => {
  const binary = atob(PLACEHOLDER_PNG_BASE64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return bytes
}

describe('clipboard placeholder sentinel', () => {
  // The synchronous match in isPlaceholderPng keys off the byte length, so this
  // test is what guarantees the constant and the match cannot drift apart.
  it('decodes to exactly 70 bytes of valid PNG', () => {
    const bytes = placeholderBytes()
    expect(bytes.length).toBe(70)
    expect(PLACEHOLDER_PNG_BYTE_LENGTH).toBe(70)
    expect(Array.from(bytes.slice(0, 8))).toEqual([137, 80, 78, 71, 13, 10, 26, 10])
    // Trailing IEND chunk — proves the PNG is complete and decodable, which is
    // what Chrome's clipboard sanitiser requires.
    expect(String.fromCharCode(...bytes.slice(-8, -4))).toBe('IEND')
  })

  it('matches a file built from the sentinel bytes', () => {
    const file = new File([placeholderBytes()], 'image.png', { type: 'image/png' })
    expect(file.size).toBe(PLACEHOLDER_PNG_BYTE_LENGTH)
    expect(isPlaceholderPng(file)).toBe(true)
  })

  it('does not match real images or same-sized non-PNG files', () => {
    expect(isPlaceholderPng(
      new File([new Uint8Array(4096)], 'screenshot.png', { type: 'image/png' }),
    )).toBe(false)
    expect(isPlaceholderPng(
      new File([placeholderBytes()], 'photo.jpg', { type: 'image/jpeg' }),
    )).toBe(false)
    expect(isPlaceholderPng(
      new File(['x'.repeat(70)], 'notes.txt', { type: 'text/plain' }),
    )).toBe(false)
  })
})
