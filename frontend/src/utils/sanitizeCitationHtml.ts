import DOMPurify from 'dompurify'

// Citation snippets come from model output and from indexed document text, so
// they are untrusted. They may legitimately carry light inline formatting
// (emphasis, highlighting, line breaks); everything else is dropped. The
// output sits inside an inline <span>, so only phrasing tags are allowed.
const ALLOWED_TAGS = ['b', 'i', 'em', 'strong', 'mark', 'u', 's', 'sub', 'sup', 'br', 'span', 'code']
const ALLOWED_ATTR: string[] = []

/**
 * Sanitise a citation snippet before it is injected with
 * dangerouslySetInnerHTML. Only inline text-formatting tags survive, no
 * attributes are kept, and the contents of disallowed elements such as
 * <script> and <style> are removed rather than rendered as text.
 */
export function sanitizeCitationHtml(snippet: string | null | undefined): string {
  if (!snippet) return ''
  return DOMPurify.sanitize(snippet, {
    ALLOWED_TAGS,
    ALLOWED_ATTR,
    ALLOW_DATA_ATTR: false,
    ALLOW_ARIA_ATTR: false,
    KEEP_CONTENT: true,
  })
}
