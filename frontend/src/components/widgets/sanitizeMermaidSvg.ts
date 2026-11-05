import DOMPurify from 'dompurify'

// Tags that mermaid never needs in its output but that an injected payload
// could use to load remote resources, capture input or navigate (TST-01-293).
const FORBID_TAGS = [
  'img', 'image', 'iframe', 'frame', 'object', 'embed', 'video', 'audio',
  'source', 'track', 'picture', 'form', 'input', 'button', 'textarea',
  'select', 'option', 'link', 'meta', 'base', 'script', 'noscript',
  'template', 'animate', 'set', 'use',
]

/**
 * Sanitise SVG markup produced by mermaid.render() before it is injected with
 * dangerouslySetInnerHTML.
 *
 * Diagram text is attacker-influenced (user / agent content). Mermaid's own
 * escaping has had repeated gaps — e.g. GHSA-ghcm-xqfw-q4vr, where a
 * stateDiagram `classDef` value containing `</style>` breaks out of mermaid's
 * <style> element and injects arbitrary markup. Defence in depth:
 *
 *  1. Run the markup through DOMPurify (SVG + HTML profiles, because mermaid
 *     labels live in <foreignObject>), forbidding resource-loading and
 *     interactive tags and all event handler / javascript: URLs.
 *  2. Keep only the single root <svg> element; anything that escaped it
 *     (siblings after a premature </svg>) is dropped.
 *  3. Keep only mermaid's own leading <style> element; injected <style>
 *     elements (which would restyle the whole page) are removed, and '<' is
 *     stripped from the remaining CSS so it cannot close the element when the
 *     string is re-parsed.
 */
export function sanitizeMermaidSvg(svg: string): string {
  if (!svg) return ''

  const fragment = DOMPurify.sanitize(svg, {
    USE_PROFILES: { svg: true, svgFilters: true, html: true },
    ADD_TAGS: ['foreignObject'],
    // mermaid renders node/edge labels as HTML inside <foreignObject>.
    HTML_INTEGRATION_POINTS: { foreignobject: true },
    FORBID_TAGS,
    FORBID_ATTR: ['xlink:href', 'href', 'srcset', 'src', 'action', 'formaction'],
    ALLOW_UNKNOWN_PROTOCOLS: false,
    RETURN_DOM_FRAGMENT: true,
  })

  const root = Array.from(fragment.children).find(
    (el) => el.tagName.toLowerCase() === 'svg',
  )
  if (!root) return ''

  const styles = Array.from(root.querySelectorAll('style'))
  const ownStyle = styles.find((el) => el.parentElement === root)
  for (const style of styles) {
    if (style !== ownStyle) {
      style.remove()
    }
  }
  if (ownStyle) {
    ownStyle.textContent = (ownStyle.textContent || '').replace(/[<>]/g, '')
  }

  return root.outerHTML
}
