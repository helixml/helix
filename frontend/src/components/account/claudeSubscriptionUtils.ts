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

// Anthropic reports the rate-limit tier as an internal slug — the live
// /api/oauth/profile response for a Max account is "default_claude_max_20x",
// not "20x". Only the multiplier means anything to a user, so surface that
// and drop the slug when it carries none. Values without underscores are
// taken as user-typed (the setup-token form lets the user report a tier) and
// shown verbatim.
export function formatRateLimitTier(tier?: string | null): string {
  const trimmed = (tier || '').trim()
  if (!trimmed) return ''
  const multiplier = /(\d+x)$/i.exec(trimmed)
  if (multiplier) return multiplier[1].toLowerCase()
  return trimmed.includes('_') ? '' : trimmed
}

// Shared identity line for a Claude subscription: the Claude account the
// token authenticates as (the billed identity), then plan and rate-limit
// tier — e.g. "phil@winder.ai · Max · 20x". Both the agent-settings caption
// and the account-settings pills render through this so they cannot drift.
// `fallbackName` (typically the Helix user who connected the subscription)
// is shown only when the Claude account is unknown; empty string when
// nothing at all is known.
export function formatClaudeAccountIdentity(input: {
  accountEmail?: string | null
  accountName?: string | null
  fallbackName?: string | null
  plan?: string | null
  tier?: string | null
}): string {
  const account = input.accountEmail || input.accountName || input.fallbackName || ''
  const plan = input.plan
    ? input.plan.charAt(0).toUpperCase() + input.plan.slice(1)
    : ''
  return [account, plan, formatRateLimitTier(input.tier)].filter(Boolean).join(' · ')
}
