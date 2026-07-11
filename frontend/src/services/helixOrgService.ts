import axios from 'axios'
import { useQuery, useQueries, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import {
  ApiBotActivateDTO,
  ApiBotBadge,
  ApiBotChatDTO,
  ApiBotDTO,
  ApiBotDetailDTO,
  ApiBotSubscriptionDTO,
  ApiBotSubscriptionsResponse,
  ApiCreateBotRequest,
  ApiCreateBotResponse,
  ApiCreateTopicRequest,
  ApiEventCard,
  ApiGitHubReposResponse,
  ApiGitHubInstallationStatus,
  ApiGitHubManifestStartResponse,
  ApiInstallGitHubWebhookResponse,
  ApiGitHubWebhookStatusResponse,
  ApiOrgOverview,
  ApiSettingsResponse,
  ApiSettingsSpecDTO,
  ApiTopicDTO,
  ApiTopicsResponse,
  ApiToolDTO,
  ApiUpdateBotRequest,
  ApiUpdateTopicRequest,
} from '../api/api'

// Re-exported aliases. Generated Api* types mark every field
// optional; consumers use them as if fields are present. strict
// null checks are off project-wide so plain aliases suffice.
export type BotBadge = ApiBotBadge
export type BotDTO = ApiBotDTO
export type BotDetailDTO = ApiBotDetailDTO
export type BotActivateDTO = ApiBotActivateDTO
export type BotChatDTO = ApiBotChatDTO
export type ToolDTO = ApiToolDTO
export type TopicDTO = ApiTopicDTO
export type EventCard = ApiEventCard
export type SettingsSpecDTO = ApiSettingsSpecDTO
export type SettingsResponse = ApiSettingsResponse
export type TopicsResponse = ApiTopicsResponse
export type GitHubRepoDTO = NonNullable<ApiGitHubReposResponse['repos']>[number]
export type GitHubReposResponse = ApiGitHubReposResponse
export type GitHubInstallationStatus = ApiGitHubInstallationStatus
export type GitHubManifestStartResponse = ApiGitHubManifestStartResponse
export type InstallGitHubWebhookResponse = ApiInstallGitHubWebhookResponse
export type GitHubWebhookStatusResponse = ApiGitHubWebhookStatusResponse
export type BotSubscription = ApiBotSubscriptionDTO
export type BotSubscriptionsResponse = ApiBotSubscriptionsResponse
export type OrgOverview = ApiOrgOverview

export type CreateBotRequest = ApiCreateBotRequest & { id: string; content: string }
export type CreateBotResponse = ApiCreateBotResponse
export type UpdateBotRequest = ApiUpdateBotRequest
export type CreateTopicRequest = ApiCreateTopicRequest & { name: string }
export type UpdateTopicRequest = ApiUpdateTopicRequest

export interface HelixModelInfo {
  id: string
  name?: string
  description?: string
  context_length?: number
}

export const QUERY_KEYS = {
  overview: (orgID: string) => ['helix-org', orgID, 'overview'] as const,
  bot: (orgID: string, id: string) => ['helix-org', orgID, 'bots', id] as const,
  bots: (orgID: string) => ['helix-org', orgID, 'bots'] as const,
  tools: (orgID: string) => ['helix-org', orgID, 'tools'] as const,
  settings: (orgID: string) => ['helix-org', orgID, 'settings'] as const,
  providers: () => ['helix-org', 'providers'] as const,
  modelsForProvider: (provider: string) => ['helix-org', 'models', provider] as const,
  topics: (orgID: string) => ['helix-org', orgID, 'topics'] as const,
  topic: (orgID: string, id: string) => ['helix-org', orgID, 'topics', id] as const,
  webhookStatus: (orgID: string, id: string) => ['helix-org', orgID, 'topics', id, 'webhook-status'] as const,
  topicMessageCount: (orgID: string, id: string) => ['helix-org', orgID, 'topics', id, 'message-count'] as const,
  botSubs: (orgID: string, botID: string) => ['helix-org', orgID, 'bots', botID, 'subscriptions'] as const,
  processors: (orgID: string) => ['helix-org', orgID, 'processors'] as const,
  processor: (orgID: string, id: string) => ['helix-org', orgID, 'processors', id] as const,
  chartPositions: (orgID: string) => ['helix-org', orgID, 'chart-positions'] as const,
}

// ---- Processors ---------------------------------------------------------
// A Processor is a transform/filter node interposed on the edge between a
// Topic and its subscribers. Its REST surface is JSON:API, so the hooks
// flatten {data:{id,attributes}} resources into flat ProcessorDTOs.

export interface ProcessorOutput {
  topic_id: string
  match?: string
  label?: string
  owned?: boolean
  // Set when this route is auto-managed by a reconciler for the named
  // Worker (the Slack auto-router); empty for human-authored routes.
  managed_for?: string
}

export interface ProcessorDTO {
  id: string
  name: string
  input_topic_id: string
  kind: string
  config?: Record<string, unknown>
  outputs: ProcessorOutput[]
  created_by?: string
  created_at?: string
  // True for automation-created processors (the Slack auto-router).
  automated?: boolean
}

interface JsonApiResource<T> { id: string; type: string; attributes: T }
interface JsonApiDoc<T> { data: T }

function flattenProcessor(res: JsonApiResource<Omit<ProcessorDTO, 'id'>>): ProcessorDTO {
  return { id: res.id, ...res.attributes }
}

export function useHelixOrgBase(): { base: string; orgID: string } {
  const { params } = useRouter()
  const orgID = (params.org_id as string) || ''
  const base = orgID ? `/api/v1/orgs/${encodeURIComponent(orgID)}` : ''
  return { base, orgID }
}

// useListSlackWorkspaces returns the Slack workspaces installed for the
// current org (org-scoped slack_workspace ServiceConnections). Used by
// the topic transport picker (choose a workspace) and the Settings
// integrations panel (list / disconnect).
export function useListSlackWorkspaces(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: ['helix-org', orgID, 'slack-workspaces'],
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsSlackWorkspacesDetail(orgID)
      return (res.data as any[]) || []
    },
    enabled: (options?.enabled ?? true) && !!orgID,
  })
}

export function useListSlackApps(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: ['helix-org', orgID, 'slack-apps'],
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsSlackAppsDetail(orgID)
      return (res.data as any[]) || []
    },
    enabled: (options?.enabled ?? true) && !!orgID,
  })
}

// useStartSlackInstall asks the backend for the OAuth authorize URL (for a
// specific app when more than one is configured), then the caller
// redirects the browser to it.
export function useStartSlackInstall() {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (appId?: string) => {
      const res = await api.getApiClient().v1OrgsSlackOauthStartDetail(orgID, appId ? { app_id: appId } : undefined)
      return (res.data as any).url as string
    },
  })
}

// useConnectSlackWorkspace connects a workspace from a pasted bot token
// (Socket Mode / on-prem — no OAuth). The backend auth.tests the token,
// derives the team, and stores a slack_workspace connection.
export function useConnectSlackWorkspace() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (args: { botToken: string; appConnectionId?: string }) => {
      const res = await api.getApiClient().v1OrgsSlackWorkspacesCreate(orgID, {
        bot_token: args.botToken,
        app_connection_id: args.appConnectionId,
      })
      return res.data
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['helix-org', orgID, 'slack-workspaces'] })
    },
  })
}

export function useDisconnectSlackWorkspace() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (id: string) => {
      await api.getApiClient().v1OrgsSlackWorkspacesDelete(orgID, id)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ['helix-org', orgID, 'slack-workspaces'] })
    },
  })
}

export function useHelixOrgOverview(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.overview(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsOverviewDetail(orgID)
      return res.data as OrgOverview
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useEnsureBotChat() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (botId: string) => {
      const res = await api.getApiClient().v1OrgsBotsChatCreate(botId, orgID)
      return res.data as BotChatDTO
    },
    onSuccess: (_data, botId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bot(orgID, botId) })
    },
  })
}

// useActivateBot manually triggers an activation for a Bot. The click
// goes through the full activation pipeline (ensureProject →
// AttachHelixOrgMCP → ensureSession → container start) instead of the
// generic /sessions/{id}/resume — which doesn't re-attach the helix-org
// MCP and so leaves the desktop without the org-graph tools.
//
// Accepts an orgID override so callers that aren't running inside the
// helix-org base context can pass the org slug explicitly.
export function useActivateBot(orgIDOverride?: string) {
  const api = useApi()
  const { orgID: baseOrgID } = useHelixOrgBase()
  const orgID = orgIDOverride ?? baseOrgID
  return useMutation({
    mutationFn: async (botId: string) => {
      const res = await api.getApiClient().v1OrgsBotsActivateCreate(botId, orgID)
      return res.data as BotActivateDTO
    },
  })
}

// useRestartBotAgent recreates the bot's desktop container from scratch.
// Wired to the bot page's "Restart agent session" button. Unlike
// useActivateBot (which continues the existing session via SendMessage),
// this hits the dedicated bot restart endpoint, which fully removes the
// bot's current session (stop desktop → delete session → clear pointer)
// and then activates a brand-new one — a fresh desktop, thread and MCP
// surface. When the bot has no live session it just activates.
export function useRestartBotAgent(orgIDOverride?: string) {
  const api = useApi()
  const { orgID: baseOrgID } = useHelixOrgBase()
  const orgID = orgIDOverride ?? baseOrgID
  return useMutation({
    mutationFn: async (botId: string) => {
      const res = await api.getApiClient().v1OrgsBotsRestartAgentCreate(botId, orgID)
      return res.data as BotActivateDTO
    },
  })
}

export function useListHelixOrgBots(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.bots(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsBotsDetail(orgID)
      return (res.data ?? []) as BotDTO[]
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useHelixOrgBot(botId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.bot(orgID, botId ?? ''),
    queryFn: async () => {
      if (!botId) return null
      const res = await api.getApiClient().v1OrgsBotsDetail2(botId, orgID)
      return res.data as BotDetailDTO
    },
    enabled: !!orgID && !!botId && (options?.enabled ?? true),
  })
}

export function useListHelixOrgTools(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.tools(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsToolsDetail(orgID)
      return (res.data ?? []) as ToolDTO[]
    },
    staleTime: 5 * 60 * 1000,
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useCreateBot() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreateBotRequest) => {
      const res = await api.getApiClient().v1OrgsBotsCreate(orgID, payload)
      return res.data as CreateBotResponse
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      // Creating a bot mints its s-transcript-<id> topic — refresh the
      // Topics list / chart topic nodes so it shows without a reload.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
    },
  })
}

// useUpdateBot rewrites a Bot's content (its identity markdown), tools,
// and/or topics. The Spawner projects the new content on the next
// activation. Drives the editable content + tools panels on the bot
// detail page.
export function useUpdateBot() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { id: string } & UpdateBotRequest) => {
      const { id, ...body } = payload
      const res = await api.getApiClient().v1OrgsBotsPartialUpdate(orgID, id, body)
      return res.data as BotDTO
    },
    onSuccess: (_data, payload) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bot(orgID, payload.id) })
    },
  })
}

// useAddBotParent adds a reporting line — the Bot now also reports to
// parentID. Reporting is many-to-many, so this is additive. Drives the
// chart's drag-to-report: dragging manager → subordinate adds the line.
// The topology reconciler wires the comms channels the edge implies (the
// manager's s-team-<mgr> topic and the pair's s-dm-<pair> channel, plus
// the manager observing the report's transcript), so we refresh topics
// too — not just the bot list — so those new nodes render without a
// reload.
export function useAddBotParent() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ botID, parentID }: { botID: string; parentID: string }) => {
      await api.getApiClient().v1OrgsBotsParentsCreate(botID, orgID, { parent_id: parentID })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
    },
  })
}

// useRemoveBotParent drops one reporting line — the Bot no longer reports
// to parentID. Drives the chart's delete-edge flow; only the dragged
// edge's line is removed, leaving any other managers intact. The
// reconciler tears down the channels the edge implied (the manager's team
// topic when its last report leaves, and the pair's DM channel), so
// refresh topics as well as the bot list.
export function useRemoveBotParent() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ botID, parentID }: { botID: string; parentID: string }) => {
      await api.getApiClient().v1OrgsBotsParentsDelete(botID, parentID, orgID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
    },
  })
}

// useSubscribeBotAtChart drives the chart's drag-to-subscribe flow. Each
// call carries its own (botID, topicID) because the chart wires arbitrary
// Bots to arbitrary topics; useSubscribeBot is bound to a single botID and
// so doesn't fit the canvas's onConnect.
export function useSubscribeBotAtChart() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ botID, topicID }: { botID: string; topicID: string }) => {
      await api.getApiClient().v1OrgsBotsSubscriptionsCreate(botID, orgID, { topic_id: topicID })
    },
    onSuccess: (_data, { botID }) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.botSubs(orgID, botID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

// useUnsubscribeBotAtChart is the chart-scoped counterpart of
// useSubscribeBotAtChart — it drops a (bot, topic) subscription when the
// user deletes a subscription edge on the canvas.
export function useUnsubscribeBotAtChart() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ botID, topicID }: { botID: string; topicID: string }) => {
      await api.getApiClient().v1OrgsBotsSubscriptionsDelete(botID, topicID, orgID)
    },
    onSuccess: (_data, { botID }) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.botSubs(orgID, botID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useDeleteBot() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (botId: string) => {
      await api.getApiClient().v1OrgsBotsDelete(botId, orgID)
    },
    onSuccess: (_data, botId) => {
      // Evict the deleted bot's own queries (the bot key prefix-matches
      // its subscriptions key, so this drops both) and cancel any
      // in-flight fetch. Without this the bot detail page would refetch a
      // now-deleted bot and log a 404.
      qc.removeQueries({ queryKey: QUERY_KEYS.bot(orgID, botId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      // Exact: refresh the list itself without prefix-matching (and so
      // refetching) the bot/subscriptions queries we just removed.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID), exact: true })
      // Deleting cascades away the bot's s-transcript-<id> topic and its
      // direct reports' parent edge — refresh the Topics list and any
      // open topic detail.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
    },
  })
}

export function useHelixOrgSettings(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.settings(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsSettingsDetail(orgID)
      return res.data as SettingsResponse
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useSetHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { key: string; value: string }) => {
      await api.getApiClient().v1OrgsSettingsUpdate(payload.key, orgID, { value: payload.value })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings(orgID) })
    },
  })
}

export function useDeleteHelixOrgSetting() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (key: string) => {
      await api.getApiClient().v1OrgsSettingsDelete(key, orgID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.settings(orgID) })
    },
  })
}

// /api/v1/providers and /v1/models are not currently in the generated
// HelixOrg-tagged client surface; left on raw api.get for now.
export function useHelixProviders(options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.providers(),
    queryFn: async () => {
      const data = await api.get<string[]>('/api/v1/providers')
      return data ?? []
    },
    staleTime: 5 * 60 * 1000,
    enabled: options?.enabled ?? true,
  })
}

export function useHelixModelsForProvider(provider: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  return useQuery({
    queryKey: QUERY_KEYS.modelsForProvider(provider ?? ''),
    queryFn: async () => {
      if (!provider) return [] as HelixModelInfo[]
      const data = await api.get<{ data: HelixModelInfo[] }>(`/v1/models?provider=${encodeURIComponent(provider)}`)
      return data?.data ?? []
    },
    staleTime: 5 * 60 * 1000,
    enabled: !!provider && (options?.enabled ?? true),
  })
}

export function useListHelixOrgTopics(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.topics(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsTopicsDetail(orgID)
      return res.data as TopicsResponse
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useHelixOrgTopic(topicId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.topic(orgID, topicId ?? ''),
    queryFn: async () => {
      if (!topicId) return null
      const res = await api.getApiClient().v1OrgsTopicsDetail2(topicId, orgID)
      return res.data as TopicDTO
    },
    enabled: !!orgID && !!topicId && (options?.enabled ?? true),
  })
}

// useGitHubWebhookStatus reports the LIVE state of a github topic's repo
// webhook as seen on GitHub (state: "installed" | "missing" | "unknown"), so
// the detail page can link to the real hook or offer a re-install. Read-only;
// safe to refetch. Invalidate QUERY_KEYS.webhookStatus after an install.
export function useGitHubWebhookStatus(topicId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.webhookStatus(orgID, topicId ?? ''),
    queryFn: async () => {
      if (!topicId) return null
      const res = await api.getApiClient().v1OrgsTopicsGithubWebhookStatusDetail(topicId, orgID)
      return res.data as GitHubWebhookStatusResponse
    },
    enabled: !!orgID && !!topicId && (options?.enabled ?? true),
  })
}

// useTopicMessageCount reports the total number of messages waiting on
// a single topic via the paginated JSON:API messages endpoint. We only
// need meta.total, so we request the smallest possible page (size 1) and
// ignore the body — the count is the cheap part the server computes
// independently of the page slice. Used by the topic detail metric card.
export function useTopicMessageCount(topicId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.topicMessageCount(orgID, topicId ?? ''),
    queryFn: async () => {
      if (!topicId) return 0
      const res = await api.getApiClient().v1OrgsTopicsMessagesDetail(topicId, orgID, {
        'page[number]': 1,
        'page[size]': 1,
      })
      return res.data?.meta?.total ?? 0
    },
    enabled: !!orgID && !!topicId && (options?.enabled ?? true),
  })
}

// useTopicMessageCounts fans the same per-topic count query out across
// every topic id so the org chart can label each topic card. Returns a
// topicId → total map; missing/in-flight ids resolve to 0 (the caller
// renders 0 rather than flickering). One React Query per id keeps each
// count independently cached + invalidated, shared with the detail
// page's single-topic hook above (same query key).
export function useTopicMessageCounts(topicIds: string[], options?: { enabled?: boolean }): Record<string, number> {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  const enabled = !!orgID && (options?.enabled ?? true)
  const results = useQueries({
    queries: topicIds.map((id) => ({
      queryKey: QUERY_KEYS.topicMessageCount(orgID, id),
      queryFn: async () => {
        const res = await api.getApiClient().v1OrgsTopicsMessagesDetail(id, orgID, {
          'page[number]': 1,
          'page[size]': 1,
        })
        return res.data?.meta?.total ?? 0
      },
      enabled,
    })),
  })
  const counts: Record<string, number> = {}
  topicIds.forEach((id, i) => {
    counts[id] = (results[i]?.data as number | undefined) ?? 0
  })
  return counts
}

export function useCreateHelixOrgTopic() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreateTopicRequest) => {
      const res = await api.getApiClient().v1OrgsTopicsCreate(orgID, payload)
      return res.data as TopicDTO
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

// Probes "is GitHub connected?" — must stay quiet on failure (the
// caller renders the disabled-transport hint, not a toast). The
// generated client throws on non-2xx, so we swallow here.
export function useListGitHubRepos(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: ['helix-org', 'github-repos', orgID],
    queryFn: async () => {
      try {
        const res = await api.getApiClient().v1OrgsGithubReposDetail(orgID)
        return res.data as GitHubReposResponse
      } catch {
        return null
      }
    },
    enabled: !!orgID && (options?.enabled ?? true),
    staleTime: 0,
    refetchOnMount: 'always',
  })
}

// Probes "is the Helix GitHub App installed for this org?" — drives the
// New Topic "Install Helix" gate. Quiet on failure (returns null) so the
// dialog renders the install CTA rather than a toast.
export function useGitHubAppInstallation(options?: { enabled?: boolean; pollWhileNotInstalled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: ['helix-org', 'github-app-installation', orgID],
    queryFn: async () => {
      try {
        const res = await api.getApiClient().v1OrgsGithubAppInstallationDetail(orgID)
        return res.data as GitHubInstallationStatus
      } catch {
        return null
      }
    },
    enabled: !!orgID && (options?.enabled ?? true),
    staleTime: 0,
    refetchOnMount: 'always',
    // Poll until installed: the GitHub popup's postMessage is severed by
    // GitHub's COOP headers, so polling is how the dialog reliably detects
    // create→install completing.
    refetchInterval: options?.pollWhileNotInstalled
      ? (query) => ((query.state.data as GitHubInstallationStatus | null)?.installed ? false : 4000)
      : false,
  })
}

// Starts the GitHub App Manifest flow: the backend returns the GitHub POST
// URL + a Helix-authored manifest + CSRF state, which the dialog submits as a
// form so GitHub creates the app on the user's behalf.
export function useGitHubManifestStart() {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (input: { github_org: string; origin: string }) => {
      const res = await api.getApiClient().v1OrgsGithubAppManifestCreate(orgID, { body: input } as any)
      return res.data as GitHubManifestStartResponse
    },
  })
}

// Throws InstallWebhookFailedError on non-2xx so callers can detect
// the "snackbar already shown" sentinel and skip their own toast.
export function useInstallGitHubWebhook() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (topicId: string) => {
      try {
        const res = await api.getApiClient().v1OrgsTopicsGithubInstallWebhookCreate(topicId, orgID)
        return res.data as InstallGitHubWebhookResponse
      } catch (e: any) {
        const msg = e?.response?.data?.error ?? e?.message ?? 'install webhook failed'
        const failed = new InstallWebhookFailedError(msg)
        throw failed
      }
    },
    onSuccess: (_data, topicId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topic(orgID, topicId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.webhookStatus(orgID, topicId) })
    },
  })
}

export class InstallWebhookFailedError extends Error {
  constructor(message = 'install webhook failed') {
    super(message)
    this.name = 'InstallWebhookFailedError'
  }
}

export function useUpdateHelixOrgTopic() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ topicId, payload }: { topicId: string; payload: UpdateTopicRequest }) => {
      const res = await api.getApiClient().v1OrgsTopicsUpdate(topicId, orgID, payload)
      return res.data as TopicDTO
    },
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topic(orgID, vars.topicId) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useDeleteHelixOrgTopic() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (topicId: string) => {
      await api.getApiClient().v1OrgsTopicsDelete(topicId, orgID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useListBotSubscriptions(botID: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.botSubs(orgID, botID ?? ''),
    queryFn: async () => {
      if (!botID) return null
      const res = await api.getApiClient().v1OrgsBotsSubscriptionsDetail(botID, orgID)
      return res.data as BotSubscriptionsResponse
    },
    enabled: !!orgID && !!botID && (options?.enabled ?? true),
  })
}

export function useSubscribeBot(botID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (topicID: string) => {
      if (!botID) throw new Error('botID is required to subscribe')
      const res = await api.getApiClient().v1OrgsBotsSubscriptionsCreate(botID, orgID, { topic_id: topicID })
      return res.data as BotSubscription
    },
    onSuccess: () => {
      if (botID) {
        qc.invalidateQueries({ queryKey: QUERY_KEYS.botSubs(orgID, botID) })
      }
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useUnsubscribeBot(botID: string | undefined) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (topicID: string) => {
      if (!botID) throw new Error('botID is required to unsubscribe')
      await api.getApiClient().v1OrgsBotsSubscriptionsDelete(botID, topicID, orgID)
    },
    onSuccess: () => {
      if (botID) {
        qc.invalidateQueries({ queryKey: QUERY_KEYS.botSubs(orgID, botID) })
      }
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

// useTopicSampleMessage fetches the single most recent REAL message on a
// topic (newest first, page size 1) so the processor drawer can show
// "what a message on this topic looks like". Returns null when the topic
// has no messages — no synthetic/fake data.
export function useTopicSampleMessage(topicId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: ['helix-org', orgID, 'topics', topicId ?? '', 'sample-message'] as const,
    queryFn: async () => {
      if (!topicId) return null
      const res = await api.getApiClient().v1OrgsTopicsMessagesDetail(topicId, orgID, {
        'page[number]': 1,
        'page[size]': 1,
      })
      const doc = res.data as unknown as { data?: { attributes?: ProcessorPreviewSample & { raw?: string } }[] }
      const first = doc.data?.[0]
      return first?.attributes ?? null
    },
    enabled: !!orgID && !!topicId && (options?.enabled ?? true),
  })
}

export interface ProcessorPreviewSample {
  from?: string
  subject?: string
  body?: string
  raw?: string
}

export function useListHelixOrgProcessors(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.processors(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsProcessorsDetail(orgID)
      const doc = res.data as unknown as { data: JsonApiResource<Omit<ProcessorDTO, 'id'>>[] }
      return (doc.data ?? []).map(flattenProcessor)
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useHelixOrgProcessor(id: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.processor(orgID, id ?? ''),
    queryFn: async () => {
      if (!id) return null
      const res = await api.getApiClient().v1OrgsProcessorsDetail2(orgID, id)
      const doc = res.data as unknown as JsonApiDoc<JsonApiResource<Omit<ProcessorDTO, 'id'>>>
      return flattenProcessor(doc.data)
    },
    enabled: !!orgID && !!id && (options?.enabled ?? true),
  })
}

export interface ProcessorWriteAttrs {
  name: string
  input_topic_id?: string
  kind: string
  config?: Record<string, unknown>
  created_by?: string
  outputs?: { topic_id?: string; match?: string; label?: string }[]
}

export function useCreateHelixOrgProcessor() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (attrs: ProcessorWriteAttrs) => {
      const res = await api.getApiClient().v1OrgsProcessorsCreate(orgID, {
        data: { type: 'processors', attributes: attrs },
      })
      const doc = res.data as unknown as JsonApiDoc<JsonApiResource<Omit<ProcessorDTO, 'id'>>>
      return flattenProcessor(doc.data)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.processors(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

export function useUpdateHelixOrgProcessor() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ id, attrs }: { id: string; attrs: ProcessorWriteAttrs }) => {
      const res = await api.getApiClient().v1OrgsProcessorsUpdate(orgID, id, {
        data: { type: 'processors', attributes: attrs },
      })
      const doc = res.data as unknown as JsonApiDoc<JsonApiResource<Omit<ProcessorDTO, 'id'>>>
      return flattenProcessor(doc.data)
    },
    onSuccess: (_data, vars) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.processors(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.processor(orgID, vars.id) })
    },
  })
}

export function useDeleteHelixOrgProcessor() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (id: string) => {
      await api.getApiClient().v1OrgsProcessorsDelete(orgID, id)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.processors(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.topics(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

// ---- Chart positions (free-placed canvas layout) ------------------------
// Nodes without a saved position fall back to the chart's auto-layout
// (dagre for bots, topic columns, processor strip). The OpenAPI client
// is not regenerated for this yet — raw axios via useApi matches the
// providers pattern until `./stack update_openapi` picks up the swagger
// annotations on chart_positions.go.

export type ChartNodeKind = 'bot' | 'topic' | 'processor'

export interface ChartPositionDTO {
  kind: ChartNodeKind | string
  id: string
  x: number
  y: number
}

export interface ChartPositionsResponse {
  positions: ChartPositionDTO[]
}

/** Map key is `${kind}:${id}` → {x,y}. */
export type ChartPositionMap = Record<string, { x: number; y: number }>

export function chartPositionKey(kind: string, id: string): string {
  return `${kind}:${id}`
}

export function useListChartPositions(options?: { enabled?: boolean }) {
  const { base, orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.chartPositions(orgID),
    queryFn: async (): Promise<ChartPositionMap> => {
      // axios directly so 4xx/5xx throw into react-query (useApi.get
      // swallows errors and returns null).
      const res = await axios.get<ChartPositionsResponse>(`${base}/chart/positions`, {
        withCredentials: true,
      })
      const map: ChartPositionMap = {}
      for (const p of res.data?.positions ?? []) {
        if (!p.kind || !p.id) continue
        map[chartPositionKey(p.kind, p.id)] = { x: p.x, y: p.y }
      }
      return map
    },
    enabled: !!orgID && !!base && (options?.enabled ?? true),
  })
}

export function useUpsertChartPositions() {
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (positions: ChartPositionDTO[]) => {
      // Optimistically merge so the node stays put even if the
      // response is slow / the graph rebuilds mid-flight.
      qc.setQueryData<ChartPositionMap>(QUERY_KEYS.chartPositions(orgID), (prev) => {
        const next: ChartPositionMap = { ...(prev ?? {}) }
        for (const p of positions) {
          if (!p.kind || !p.id) continue
          next[chartPositionKey(p.kind, p.id)] = { x: p.x, y: p.y }
        }
        return next
      })
      const res = await axios.put<ChartPositionsResponse>(
        `${base}/chart/positions`,
        { positions },
        { withCredentials: true },
      )
      return res.data
    },
  })
}

export function useClearChartPositions() {
  const qc = useQueryClient()
  const { base, orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async () => {
      await axios.delete(`${base}/chart/positions`, { withCredentials: true })
    },
    onSuccess: () => {
      qc.setQueryData<ChartPositionMap>(QUERY_KEYS.chartPositions(orgID), {})
    },
  })
}

