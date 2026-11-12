import { describe, expect, it } from 'vitest'
import { escapeHtml } from './escapeHtml'

describe('escapeHtml', () => {
  it('escapes markup characters', () => {
    expect(escapeHtml(`<img src=x onerror="alert('1')">&`)).toBe(
      '&lt;img src=x onerror=&quot;alert(&#39;1&#39;)&quot;&gt;&amp;',
    )
  })

  it('renders as inert text when interpolated into innerHTML', () => {
    const host = document.createElement('div')
    const message = '<img src=x onerror="alert(1)"><script>alert(2)</script>'
    host.innerHTML = `<strong>${escapeHtml(message)}</strong>`
    expect(host.querySelector('img, script')).toBeNull()
    expect(host.textContent).toBe(message)
  })

  it('stringifies non-string values', () => {
    expect(escapeHtml(undefined)).toBe('')
    expect(escapeHtml(null)).toBe('')
    expect(escapeHtml(42)).toBe('42')
  })
})
