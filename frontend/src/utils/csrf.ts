// CSRF token utility for BFF authentication pattern
// The CSRF token is stored in the helix_csrf cookie (readable by JS)

const CSRF_COOKIE_NAME = 'helix_csrf'
export const CSRF_HEADER_NAME = 'X-CSRF-Token'

/**
 * Reads the CSRF token from the helix_csrf cookie.
 * Returns null if the cookie is not present.
 */
export const getCSRFToken = (): string | null => {
  const match = document.cookie.match(new RegExp('(^| )' + CSRF_COOKIE_NAME + '=([^;]+)'))
  return match ? decodeURIComponent(match[2]) : null
}

/**
 * Returns headers object with CSRF token for fetch() requests.
 * Use this for POST/PUT/DELETE/PATCH requests.
 */
export const getCSRFHeaders = (): Record<string, string> => {
  const token = getCSRFToken()
  return token ? { [CSRF_HEADER_NAME]: token } : {}
}
