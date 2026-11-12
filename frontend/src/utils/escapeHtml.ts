const HTML_ESCAPES: Record<string, string> = {
  '&': '&amp;',
  '<': '&lt;',
  '>': '&gt;',
  '"': '&quot;',
  "'": '&#39;',
}

/**
 * Escape text for interpolation into an HTML string (element content or a
 * quoted attribute value). Prefer textContent; use this only where markup is
 * assembled as a string, such as the plain-HTML error overlay that must work
 * when React is not running.
 */
export function escapeHtml(value: unknown): string {
  return String(value ?? '').replace(/[&<>"']/g, (ch) => HTML_ESCAPES[ch])
}
