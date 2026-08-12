export const CHART_TOPIC_FILTERS = [
  { id: 'direct_messages', label: 'Direct messages' },
  { id: 'local', label: 'Local topics' },
  { id: 'webhook', label: 'Webhooks' },
  { id: 'github', label: 'GitHub' },
  { id: 'gitlab', label: 'GitLab' },
  { id: 'postmark', label: 'Postmark email' },
  { id: 'cron', label: 'Cron' },
  { id: 'other', label: 'Other' },
] as const

export type ChartTopicFilter = (typeof CHART_TOPIC_FILTERS)[number]['id']

export const DEFAULT_CHART_TOPIC_FILTERS: ChartTopicFilter[] = CHART_TOPIC_FILTERS
  .map((filter) => filter.id)
  .filter((filter) => filter !== 'direct_messages' && filter !== 'local' && filter !== 'other')

type TopicIdentity = {
  name: string
  kind: string
}

const filterIds = new Set<ChartTopicFilter>(CHART_TOPIC_FILTERS.map((filter) => filter.id))

const storageKey = (userId: string, orgId: string): string =>
  `helix.orgChart.topicVisibility.${userId}.${orgId}`

export const chartTopicFilterFor = (topic: TopicIdentity): ChartTopicFilter => {
  if (topic.name.trimStart().toLowerCase().startsWith('dm:')) return 'direct_messages'

  switch (topic.kind.trim().toLowerCase()) {
    case 'local':
      return 'local'
    case 'webhook':
      return 'webhook'
    case 'github':
      return 'github'
    case 'gitlab':
      return 'gitlab'
    case 'email':
    case 'postmark':
      return 'postmark'
    case 'cron':
      return 'cron'
    default:
      return 'other'
  }
}

export const loadChartTopicVisibility = (
  userId: string,
  orgId: string,
): ChartTopicFilter[] | null => {
  if (!userId || !orgId) return null
  try {
    const raw = window.localStorage.getItem(storageKey(userId, orgId))
    if (!raw) return null
    const parsed: unknown = JSON.parse(raw)
    if (!Array.isArray(parsed) || !parsed.every((value) => typeof value === 'string' && filterIds.has(value as ChartTopicFilter))) {
      return null
    }
    const selected = new Set(parsed as ChartTopicFilter[])
    return CHART_TOPIC_FILTERS.map((filter) => filter.id).filter((id) => selected.has(id))
  } catch {
    return null
  }
}

export const saveChartTopicVisibility = (
  userId: string,
  orgId: string,
  filters: ChartTopicFilter[],
): void => {
  if (!userId || !orgId) return
  const selected = new Set(filters)
  const validFilters = CHART_TOPIC_FILTERS.map((filter) => filter.id).filter((id) => selected.has(id))
  try {
    window.localStorage.setItem(storageKey(userId, orgId), JSON.stringify(validFilters))
  } catch {
    // Quota / private mode — the chart remains usable without persistence.
  }
}
