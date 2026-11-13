/**
 * Static third-party resources referenced by index.html must pin a version
 * and carry a subresource-integrity hash, so a change on the CDN cannot
 * alter what the app loads. Google Fonts is exempt: its CSS is generated per
 * browser, so no single hash can match it.
 */
import { describe, expect, it } from 'vitest'
import fs from 'fs'
import path from 'path'

const INDEX_HTML = fs.readFileSync(path.resolve(__dirname, '../../index.html'), 'utf8')

// Origins whose responses differ per client and cannot be hashed.
const UNHASHABLE_ORIGINS = ['https://fonts.googleapis.com']

const externalStylesheets = (): string[] =>
  [...INDEX_HTML.matchAll(/<link\b[^>]*>/gs)]
    .map((m) => m[0])
    .filter((tag) => /rel="stylesheet"/.test(tag) && /href="https:\/\//.test(tag))

describe('index.html third-party stylesheets', () => {
  it('pin integrity for every hashable CDN stylesheet', () => {
    const hashable = externalStylesheets().filter(
      (tag) => !UNHASHABLE_ORIGINS.some((origin) => tag.includes(`href="${origin}`)),
    )
    expect(hashable.length).toBeGreaterThan(0)
    for (const tag of hashable) {
      expect(tag).toMatch(/integrity="sha384-[A-Za-z0-9+/=]+"/)
      expect(tag).toMatch(/crossorigin="anonymous"/)
      expect(tag).toMatch(/@\d+\.\d+\.\d+\//)
    }
  })
})
