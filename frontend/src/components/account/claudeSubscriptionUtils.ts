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
