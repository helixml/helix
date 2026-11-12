// Sends a signed-in browser to the login page once its session has ended
// server side (signed out elsewhere, identity-provider session expired).
// Every API call then answers 401; without this, polling hooks keep making
// those calls on a page that can no longer load anything.
//
// A 401 alone is not proof the session is gone, so the first one triggers a
// single check of /api/v1/auth/authenticated; only a definite "false" leads
// to the login page, and the redirect happens at most once.

export interface SessionExpiryDeps {
  // Resolves true/false for a definite answer, null when it is unknown
  // (network error, server error) and nothing should happen.
  checkAuthenticated: () => Promise<boolean | null>
  redirectToLogin: () => void
  // How long a "still authenticated" answer suppresses further checks.
  recheckAfterMs?: number
  now?: () => number
}

const isApiRequest = (url: unknown, origin: string): boolean => {
  if (typeof url !== 'string') return false
  const path = url.startsWith(origin) ? url.slice(origin.length) : url
  return path.startsWith('/api/')
}

export function createSessionExpiryHandler(deps: SessionExpiryDeps, origin = '') {
  const recheckAfterMs = deps.recheckAfterMs ?? 30_000
  const now = deps.now ?? Date.now
  let signedIn = false
  let checking = false
  let redirected = false
  let quietUntil = 0

  const verify = async () => {
    checking = true
    try {
      const authenticated = await deps.checkAuthenticated()
      if (authenticated === false && signedIn && !redirected) {
        redirected = true
        deps.redirectToLogin()
      } else if (authenticated === true) {
        quietUntil = now() + recheckAfterMs
      }
    } catch {
      // unknown: leave the page alone
    } finally {
      checking = false
    }
  }

  return {
    setSignedIn(value: boolean) {
      signedIn = value
    },
    // Axios response error interceptor.
    onResponseError(error: any) {
      if (
        signedIn &&
        !redirected &&
        !checking &&
        now() >= quietUntil &&
        error?.response?.status === 401 &&
        isApiRequest(error?.config?.url, origin)
      ) {
        void verify()
      }
      return Promise.reject(error)
    },
  }
}

const checkAuthenticated = async (): Promise<boolean | null> => {
  try {
    const res = await fetch('/api/v1/auth/authenticated', { credentials: 'same-origin' })
    if (!res.ok) return null
    const body = await res.json()
    return typeof body?.authenticated === 'boolean' ? body.authenticated : null
  } catch {
    return null
  }
}

const redirectToLogin = () => {
  try {
    const currentPath = window.location.pathname + window.location.search
    if (currentPath !== '/' && currentPath !== '/notfound' && currentPath !== '/login') {
      localStorage.setItem('login_redirect_url', currentPath)
    }
    localStorage.setItem('logout_reason', 'Your session has ended. Please sign in again.')
  } catch {
    // storage unavailable — the redirect alone still re-authenticates
  }
  window.location.assign('/login')
}

export const sessionExpiry = createSessionExpiryHandler(
  { checkAuthenticated, redirectToLogin },
  typeof window !== 'undefined' ? window.location.origin : '',
)
