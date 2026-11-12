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

/**
 * True when an OAuth provider's callback URL is on a different origin from the
 * page the admin is using. The callback page reports completion only to its
 * own origin, so popups opened from this UI would then not be notified (the
 * connection still completes; the UI falls back to noticing the popup closed).
 * Returns false for empty or unparsable input — that is validated elsewhere.
 */
export function callbackUrlOriginDiffers(
  callbackUrl: string | null | undefined,
  pageOrigin: string = window.location.origin,
): boolean {
  if (!callbackUrl) return false
  try {
    return new URL(callbackUrl).origin !== pageOrigin
  } catch {
    return false
  }
}
