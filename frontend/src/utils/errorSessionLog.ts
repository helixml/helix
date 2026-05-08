const SESSION_KEY = 'helix_error_log'
const MAX_ERRORS = 20

interface ErrorEntry {
  message: string
  stack?: string
  timestamp: string
}

export function logErrorToSession(message: string, stack?: string) {
  try {
    const existing = JSON.parse(sessionStorage.getItem(SESSION_KEY) || '[]') as ErrorEntry[]
    existing.push({
      message,
      stack,
      timestamp: new Date().toISOString(),
    })
    // Keep only the most recent errors
    while (existing.length > MAX_ERRORS) existing.shift()
    sessionStorage.setItem(SESSION_KEY, JSON.stringify(existing))
  } catch {
    // sessionStorage may be unavailable
  }
}

export function getRecentErrors(): ErrorEntry[] {
  try {
    return JSON.parse(sessionStorage.getItem(SESSION_KEY) || '[]') as ErrorEntry[]
  } catch {
    return []
  }
}

export function clearErrorLog() {
  try {
    sessionStorage.removeItem(SESSION_KEY)
  } catch {
    // ignore
  }
}
