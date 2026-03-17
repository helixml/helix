// Open URL in external browser. Works in both WKWebView (desktop app) and regular browsers.
// In the desktop app the Helix frontend runs in an iframe inside WKWebView — window.open()
// is silently suppressed. The Wails host listens for 'open-external-url' postMessages.
// This matches the pattern in index.tsx for link interception.
export function openExternalUrl(url: string) {
  if (window.parent !== window) {
    // Desktop app: the Helix frontend runs in an iframe inside WKWebView where
    // window.open is suppressed. The Wails host listens for this postMessage.
    window.parent.postMessage({ type: 'open-external-url', url }, '*')
  } else {
    window.open(url, '_blank')
  }
}

export interface TokenExpiryStatus {
  isExpired: boolean
  isExpiringSoon: boolean // less than 1 hour
  label: string // e.g. "Expires in 45m", "Expired 2h ago"
  color: 'success' | 'warning' | 'error'
}

const EXPIRING_SOON_MS = 60 * 60 * 1000 // 1 hour

export function getTokenExpiryStatus(expiresAtStr?: string): TokenExpiryStatus | null {
  if (!expiresAtStr) return null

  const expiresAt = new Date(expiresAtStr)
  if (isNaN(expiresAt.getTime())) return null

  const now = Date.now()
  const diffMs = expiresAt.getTime() - now

  if (diffMs <= 0) {
    return {
      isExpired: true,
      isExpiringSoon: false,
      label: `Expired ${formatDuration(-diffMs)} ago`,
      color: 'error',
    }
  }

  if (diffMs < EXPIRING_SOON_MS) {
    return {
      isExpired: false,
      isExpiringSoon: true,
      label: `Expires in ${formatDuration(diffMs)}`,
      color: 'warning',
    }
  }

  return {
    isExpired: false,
    isExpiringSoon: false,
    label: `Expires in ${formatDuration(diffMs)}`,
    color: 'success',
  }
}

function formatDuration(ms: number): string {
  const minutes = Math.floor(ms / 60000)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const remainMinutes = minutes % 60
  if (hours < 24) {
    return remainMinutes > 0 ? `${hours}h ${remainMinutes}m` : `${hours}h`
  }
  const days = Math.floor(hours / 24)
  const remainHours = hours % 24
  return remainHours > 0 ? `${days}d ${remainHours}h` : `${days}d`
}
