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

// Setup tokens cannot be profiled, so they have no email — but Anthropic does
// return an organization uuid on every probe. Its first segment is enough to
// tell two subscriptions apart, or recognise them as the same one.
export function formatClaudeOrganizationRef(organizationId?: string | null): string {
  const trimmed = (organizationId || '').trim()
  if (!trimmed) return ''
  return `Claude org ${trimmed.split('-')[0]}`
}

// Shared identity line for a Claude subscription: the Claude account the
// token authenticates as (the billed identity), then plan and rate-limit
// tier — e.g. "phil@winder.ai · Max · 20x". Both the agent-settings caption
// and the account-settings pills render through this so they cannot drift.
//
// Precedence is strongest-evidence-first. Everything above `fallbackName` is
// something Anthropic told us about the *Claude* account; `fallbackName` (the
// Helix user who connected the subscription) is a different fact entirely and
// only stands in when Anthropic told us nothing. Empty when nothing is known.
export function formatClaudeAccountIdentity(input: {
  accountEmail?: string | null
  accountName?: string | null
  organizationId?: string | null
  fallbackName?: string | null
  plan?: string | null
  tier?: string | null
}): string {
  const account =
    input.accountEmail ||
    input.accountName ||
    formatClaudeOrganizationRef(input.organizationId) ||
    input.fallbackName ||
    ''
  const plan = input.plan
    ? input.plan.charAt(0).toUpperCase() + input.plan.slice(1)
    : ''
  return [account, plan, formatRateLimitTier(input.tier)].filter(Boolean).join(' · ')
}

export interface ClaudeLoginExpiry {
  /** The login is already dead; agents using it will fail. */
  isExpired: boolean
  /** Dies within a day — the point at which it is worth interrupting someone. */
  isExpiringToday: boolean
  /** "Expired 2h ago" / "Expires in 5h". */
  label: string
}

/**
 * How long until the user must sign in to Claude again.
 *
 * This reads refresh_token_expires_at, not the access token's expiry. The
 * access token lives 8h and Helix refreshes it automatically, so it says
 * nothing about whether anyone needs to act. The login behind it is a hard
 * deadline — measured: rotation does not extend it — and that is the one worth
 * warning about.
 *
 * Returns null when there is nothing to say: no recorded deadline (setup
 * tokens, which carry no refresh token), or more than a day left.
 */
export function getClaudeLoginExpiry(refreshTokenExpiresAt?: string | null): ClaudeLoginExpiry | null {
  if (!refreshTokenExpiresAt) return null
  const expiresAt = new Date(refreshTokenExpiresAt)
  if (isNaN(expiresAt.getTime())) return null

  const diffMs = expiresAt.getTime() - Date.now()
  const DAY_MS = 24 * 60 * 60 * 1000
  if (diffMs > DAY_MS) return null

  if (diffMs <= 0) {
    return { isExpired: true, isExpiringToday: false, label: `Expired ${formatSpan(-diffMs)} ago` }
  }
  return { isExpired: false, isExpiringToday: true, label: `Expires in ${formatSpan(diffMs)}` }
}

function formatSpan(ms: number): string {
  const minutes = Math.floor(ms / 60000)
  if (minutes < 60) return `${Math.max(minutes, 1)}m`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h`
  return `${Math.floor(hours / 24)}d`
}

/**
 * The non-personal half of a Claude identity: "Max · 20x".
 *
 * Split out from formatClaudeAccountIdentity so the email can be rendered as
 * its own click-to-reveal element while the plan stays visible.
 */
export function formatClaudeAccountDetail(input: {
  plan?: string | null
  tier?: string | null
}): string {
  const plan = input.plan?.trim()
    ? input.plan.trim().charAt(0).toUpperCase() + input.plan.trim().slice(1)
    : ''
  return [plan, formatRateLimitTier(input.tier)].filter(Boolean).join(' · ')
}
