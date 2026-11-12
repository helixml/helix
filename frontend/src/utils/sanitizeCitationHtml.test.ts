// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { sanitizeCitationHtml } from './sanitizeCitationHtml'

// What ends up in the page: dangerouslySetInnerHTML is innerHTML.
function mount(markup: string): HTMLElement {
  const host = document.createElement('span')
  host.innerHTML = markup
  return host
}

describe('sanitizeCitationHtml', () => {
  it('keeps plain text and inline formatting', () => {
    const clean = sanitizeCitationHtml('A <b>bold</b> and <em>emphasised</em><br><mark>match</mark>')
    expect(clean).toBe('A <b>bold</b> and <em>emphasised</em><br><mark>match</mark>')
  })

  it('removes img elements and their event handlers', () => {
    const clean = sanitizeCitationHtml('text <img src=x onerror="alert(1)"> more')
    expect(clean).not.toMatch(/<img|onerror/i)
    expect(mount(clean).querySelector('img')).toBeNull()
  })

  it('removes script elements and their contents', () => {
    const clean = sanitizeCitationHtml('before<script>alert(1)</script>after')
    expect(clean).toBe('beforeafter')
  })

  it('removes links, including javascript: URLs, but keeps their text', () => {
    const clean = sanitizeCitationHtml('<a href="javascript:alert(1)">click</a>')
    expect(clean).toBe('click')
    expect(clean).not.toMatch(/javascript:/i)
  })

  it('drops all attributes from allowed tags', () => {
    const clean = sanitizeCitationHtml('<span onclick="alert(1)" style="position:fixed" class="x" id="y">t</span>')
    expect(clean).toBe('<span>t</span>')
  })

  it('unwraps block elements such as p, keeping their text', () => {
    expect(sanitizeCitationHtml('<p>one</p><div>two</div>')).toBe('onetwo')
  })

  it('drops svg, iframe and form markup', () => {
    const host = mount(sanitizeCitationHtml(
      '<svg onload="alert(1)"></svg><iframe src="https://evil.example"></iframe><form><input></form>ok'
    ))
    expect(host.querySelector('svg, iframe, form, input')).toBeNull()
    expect(host.innerHTML).not.toMatch(/onload|evil\.example/i)
    expect(host.textContent).toBe('ok')
  })

  it('handles empty input', () => {
    expect(sanitizeCitationHtml('')).toBe('')
    expect(sanitizeCitationHtml(undefined)).toBe('')
  })
})
