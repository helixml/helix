// The org Usage area's sections: one per sidebar entry, in sidebar order.
// The overview lives at /orgs/:org_id/usage; the rest at /orgs/:org_id/usage/:section.

export type UsageSectionKey = 'overview' | 'tokens' | 'performance' | 'compute' | 'breakdowns'

export interface UsageSectionInfo {
  key: UsageSectionKey
  title: string
  description: string
}

export const USAGE_SECTIONS: UsageSectionInfo[] = [
  { key: 'overview', title: 'Overview', description: 'LLM token spend and sandbox runtime, and how they trended.' },
  { key: 'tokens', title: 'LLM tokens', description: 'What each provider, model and token type processed, and its cost at API rates.' },
  { key: 'performance', title: 'Performance', description: 'LLM latency, tool call errors and prompt-cache hits.' },
  { key: 'compute', title: 'Compute', description: 'How long desktops and headless sandboxes ran, and what they cost.' },
  { key: 'breakdowns', title: 'Breakdowns', description: 'Tokens, cost and latency by project, agent, task, session and user.' },
]

export const usageSection = (key: unknown): UsageSectionInfo => (
  USAGE_SECTIONS.find(section => section.key === key) ?? USAGE_SECTIONS[0]
)

export type UsageTableKey = 'projects' | 'agents' | 'tasks' | 'sessions' | 'users'

export const USAGE_TABLES: Array<{ key: UsageTableKey; label: string }> = [
  { key: 'projects', label: 'Projects' },
  { key: 'agents', label: 'Agents' },
  { key: 'tasks', label: 'Tasks' },
  { key: 'sessions', label: 'Sessions' },
  { key: 'users', label: 'Users' },
]

// usageSectionRoute names the route and params that open a section, keeping
// the current query (date range, filters) so moving between sections never
// resets what the user is looking at.
export const usageSectionRoute = (key: UsageSectionKey, orgId: string, search: string) => {
  const query = Object.fromEntries(new URLSearchParams(search))
  delete query.table
  return key === 'overview'
    ? { name: 'org_usage', params: { ...query, org_id: orgId } }
    : { name: 'org_usage_section', params: { ...query, org_id: orgId, section: key } }
}
