/**
 * Decide whether a window `message` event is a completion message from the
 * OAuth popup this page opened.
 *
 * The OAuth callback page is served by the Helix API on the same origin as the
 * UI and posts to that origin only. Anything else — a message from another
 * origin, or from a same-origin window that is not the popup we opened — is
 * ignored so other pages cannot fake a completed (or failed) connection.
 */
export function isOAuthPopupMessage(
  event: MessageEvent,
  popup: Window | null,
  expectedType: string = 'oauth-success',
  expectedOrigin: string = window.location.origin,
): boolean {
  if (event.origin !== expectedOrigin) return false
  if (!popup || event.source !== popup) return false
  const data = event.data
  return typeof data === 'object' && data !== null && data.type === expectedType
}
