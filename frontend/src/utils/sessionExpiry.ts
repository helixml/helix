// Detects a server-side logout (expired or deleted session) while the SPA is
// running, so the UI can send the user to login instead of looking broken.
//
// A 401 alone is not proof of logout (requireAdmin returns 401 for logged-in
// non-admins), so callers confirm with /api/v1/auth/authenticated via `probe`.

type Listener = () => void

const listeners = new Set<Listener>()
let inFlight: Promise<void> | null = null

export const onSessionExpired = (cb: Listener): (() => void) => {
  listeners.add(cb)
  return () => {
    listeners.delete(cb)
  }
}

// Concurrent callers share one probe. Only an explicit `false` counts as
// logged out; errors (network, 503 during an API restart) are ignored.
export const runSessionCheck = (probe: () => Promise<boolean | undefined>): Promise<void> => {
  if (inFlight) return inFlight
  inFlight = (async () => {
    try {
      if (await probe() === false) {
        listeners.forEach((cb) => cb())
      }
    } catch {
      // unknown state; don't log the user out
    } finally {
      inFlight = null
    }
  })()
  return inFlight
}

export const isUnauthorizedApiError = (error: any): boolean => {
  const url: string = error?.config?.url || ''
  return error?.response?.status === 401 && !url.includes('/api/v1/auth/')
}

export const __resetSessionExpiryForTests = () => {
  listeners.clear()
  inFlight = null
}
