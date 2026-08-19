// OpenAI reports the plan as an internal slug in chatgpt_plan_type. Most are
// already presentable once capitalised, but a few are run-together words that
// read wrong title-cased ("Prolite"), so those get an explicit label.
const CODEX_PLAN_LABELS: Record<string, string> = {
  free: 'Free',
  plus: 'Plus',
  pro: 'Pro',
  prolite: 'Pro Lite',
  team: 'Team',
  business: 'Business',
  enterprise: 'Enterprise',
  edu: 'Edu',
}

export function formatCodexPlan(plan?: string | null): string {
  const trimmed = (plan || '').trim()
  if (!trimmed) return ''
  const normalized = trimmed.toLowerCase().replace(/[\s_-]+/g, '')
  return CODEX_PLAN_LABELS[normalized] || trimmed.charAt(0).toUpperCase() + trimmed.slice(1)
}

// A Codex credential whose id_token could not be verified has no email, but the
// account id is still on the row. Its first segment is enough to tell two
// subscriptions apart — the Codex counterpart of Claude's org reference.
export function formatCodexAccountRef(accountId?: string | null): string {
  const trimmed = (accountId || '').trim()
  if (!trimmed) return ''
  return `ChatGPT account ${trimmed.split('-')[0]}`
}

// Shared identity line for a Codex subscription: the ChatGPT account the
// credential authenticates as, then its plan — e.g. "phil@winder.ai · Pro".
// Mirrors formatClaudeAccountIdentity so both harnesses read the same wherever
// they appear. Precedence is strongest-evidence-first: everything above
// `fallbackName` is a claim OpenAI signed, while `fallbackName` (the Helix user
// or the subscription's label) only stands in when nothing was verified.
export function formatCodexAccountIdentity(input: {
  accountEmail?: string | null
  accountName?: string | null
  accountId?: string | null
  fallbackName?: string | null
  plan?: string | null
}): string {
  const account =
    input.accountEmail ||
    input.accountName ||
    formatCodexAccountRef(input.accountId) ||
    input.fallbackName ||
    ''
  return [account, formatCodexPlan(input.plan)].filter(Boolean).join(' · ')
}
