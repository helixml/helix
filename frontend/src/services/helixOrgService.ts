import axios from 'axios'
import { useQuery, useQueries, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import {
  ApiAssetDTO,
  ApiAssetHealthDTO,
  ApiCreateAssetRequest,
  ApiBotActivateDTO,
  ApiAgentDetailDTO,
  ApiBotBadge,
  ApiBotChatDTO,
  ApiBotDTO,
  ApiBotDetailDTO,
  ApiCreateBotRequest,
  ApiCreateBotResponse,
  ApiGitHubReposResponse,
  ApiGitHubInstallationStatus,
  ApiGitHubManifestStartResponse,
  ApiOrgOverview,
  ApiSettingsResponse,
  ApiSettingsSpecDTO,
  ApiToolDTO,
  ApiUpdateBotRequest,
  ApiUpdateAssetRequest,
  ApiWorkerSecretBindingDTO,
  WorkersecretAvailableSource,
  ApiPutWorkerSecretRequest,
} from '../api/api'

// Re-exported aliases. Generated Api* types mark every field
// optional; consumers use them as if fields are present. strict
// null checks are off project-wide so plain aliases suffice.
export type BotBadge = ApiBotBadge
export type BotDTO = ApiBotDTO
export type BotDetailDTO = Omit<ApiBotDetailDTO, 'bot'> & { bot?: BotDTO }
export type AgentDetailDTO = ApiAgentDetailDTO
export type BotActivateDTO = ApiBotActivateDTO
export type BotChatDTO = ApiBotChatDTO
export type ToolDTO = ApiToolDTO
export type SettingsSpecDTO = ApiSettingsSpecDTO
export type SettingsResponse = ApiSettingsResponse
export type GitHubRepoDTO = NonNullable<ApiGitHubReposResponse['repos']>[number]
export type GitHubReposResponse = ApiGitHubReposResponse
export type GitHubInstallationStatus = ApiGitHubInstallationStatus
export type GitHubManifestStartResponse = ApiGitHubManifestStartResponse
export type OrgOverview = ApiOrgOverview
export type AssetDTO = ApiAssetDTO
export type AssetHealthDTO = ApiAssetHealthDTO
export type CreateAssetRequest = ApiCreateAssetRequest
export type UpdateAssetRequest = ApiUpdateAssetRequest

export type CreateBotRequest = ApiCreateBotRequest & { id: string; content: string }
export type CreateBotResponse = ApiCreateBotResponse
export type UpdateBotRequest = ApiUpdateBotRequest
export type WorkerSecretBinding = ApiWorkerSecretBindingDTO
export type AvailableWorkerSecret = WorkersecretAvailableSource

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
  triggers: (orgID: string) => ['helix-org', orgID, 'triggers'] as const,
  botSubs: (orgID: string, botID: string) => ['helix-org', orgID, 'bots', botID, 'subscriptions'] as const,
  processors: (orgID: string) => ['helix-org', orgID, 'processors'] as const,
  processor: (orgID: string, id: string) => ['helix-org', orgID, 'processors', id] as const,
  chartPositions: (orgID: string) => ['helix-org', orgID, 'chart-positions'] as const,
  assets: (orgID: string) => ['helix-org', orgID, 'assets'] as const,
  asset: (orgID: string, id: string) => ['helix-org', orgID, 'assets', id] as const,
  assetHealth: (orgID: string, id: string) => ['helix-org', orgID, 'assets', id, 'health'] as const,
  workerSecrets: (orgID: string, id: string) => ['helix-org', orgID, 'agents', id, 'secrets'] as const,
  availableWorkerSecrets: (orgID: string, id: string) => ['helix-org', orgID, 'agents', id, 'available-secrets'] as const,
}

export function useWorkerSecrets(agentID?: string) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.workerSecrets(orgID, agentID ?? ''),
    queryFn: async () => (await api.getApiClient().v1OrgsAgentsSecretsDetail(orgID, agentID!)).data,
    enabled: !!orgID && !!agentID,
  })
}
export function useAvailableWorkerSecrets(agentID?: string) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.availableWorkerSecrets(orgID, agentID ?? ''),
    queryFn: async () => (await api.getApiClient().v1OrgsAgentsAvailableSecretsDetail(orgID, agentID!)).data,
    enabled: !!orgID && !!agentID,
  })
}
export function usePutWorkerSecret(agentID?: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (input: { name: string; payload: ApiPutWorkerSecretRequest }) => (
      await api.getApiClient().v1OrgsAgentsSecretsUpdate(orgID, agentID!, input.name, input.payload)
    ).data,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSecrets(orgID, agentID ?? '') })
      await qc.invalidateQueries({ queryKey: QUERY_KEYS.availableWorkerSecrets(orgID, agentID ?? '') })
    },
  })
}
export function useDeleteWorkerSecret(agentID?: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (name: string) => api.getApiClient().v1OrgsAgentsSecretsDelete(orgID, agentID!, name),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: QUERY_KEYS.workerSecrets(orgID, agentID ?? '') })
      await qc.invalidateQueries({ queryKey: QUERY_KEYS.availableWorkerSecrets(orgID, agentID ?? '') })
    },
  })
}

export function useListAssets(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.assets(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsAssetsDetail(orgID)
      return (res.data.assets ?? []) as AssetDTO[]
    },
    enabled: !!orgID && (options?.enabled ?? true),
  })
}

export function useAssetHealth(assetIDs: string[], options?: { enabled?: boolean; refetchInterval?: number | false }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  const enabled = !!orgID && (options?.enabled ?? true)
  const results = useQueries({
    queries: assetIDs.map((id) => ({
      queryKey: QUERY_KEYS.assetHealth(orgID, id),
      queryFn: async () => {
        const res = await api.getApiClient().v1OrgsAssetsHealthDetail(orgID, id)
        return res.data as AssetHealthDTO
      },
      enabled: enabled && !!id,
      refetchInterval: options?.refetchInterval,
      retry: false,
    })),
  })
  return assetIDs.reduce<Record<string, AssetHealthDTO | undefined>>((health, id, index) => {
    health[id] = results[index]?.data as AssetHealthDTO | undefined
    return health
  }, {})
}

export function useCreateAsset() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: CreateAssetRequest) => {
      const res = await api.getApiClient().v1OrgsAssetsCreate(orgID, payload)
      return res.data as AssetDTO
    },
    onSuccess: () => qc.invalidateQueries({ queryKey: QUERY_KEYS.assets(orgID) }),
  })
}

export function useUpdateAsset() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { id: string } & UpdateAssetRequest) => {
      const { id, ...body } = payload
      const res = await api.getApiClient().v1OrgsAssetsPartialUpdate(orgID, id, body)
      return res.data as AssetDTO
    },
    onSuccess: (_data, payload) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.assets(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.asset(orgID, payload.id) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.assetHealth(orgID, payload.id) })
    },
  })
}

export function useDeleteAsset() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (id: string) => api.getApiClient().v1OrgsAssetsDelete(orgID, id),
    onSuccess: (_data, id) => {
      qc.removeQueries({ queryKey: QUERY_KEYS.asset(orgID, id) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.assets(orgID) })
    },
  })
}

export function useLinkAsset() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ assetID, agentID }: { assetID: string; agentID: string }) =>
      api.getApiClient().v1OrgsAssetsLinksCreate(orgID, assetID, { agent_id: agentID }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.assets(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
    },
  })
}

export function useUnlinkAsset() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ assetID, agentID }: { assetID: string; agentID: string }) =>
      api.getApiClient().v1OrgsAssetsLinksDelete(orgID, assetID, agentID),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.assets(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
    },
  })
}

// ---- Processors ---------------------------------------------------------
// A Processor is a transform/filter node interposed between a source and
// the Workers attached downstream. Its REST surface is JSON:API, so the
// hooks flatten {data:{id,attributes}} resources into flat ProcessorDTOs.

export interface ProcessorOutput {
  // id is the branch's durable identity — what an attachment names.
  id: string
  // source is the terminal handle for this branch,
  // "processor_output:<processorId>:<outputId>".
  source?: string
  match?: string
  label?: string
  // Set when this route is auto-managed by a reconciler for the named
  // Worker (the Slack auto-router); empty for human-authored routes.
  managed_for?: string
}

export interface ProcessorDTO {
  id: string
  name: string
  // input_source is "trigger:<id>" or
  // "processor_output:<processorId>:<outputId>"; empty means unwired.
  input_source: string
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
// the trigger transport picker (choose a workspace) and the Settings
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
      const res = await api.getApiClient().v1OrgsAgentsChatCreate(orgID, botId)
      return res.data as BotChatDTO
    },
    onSuccess: (_data, botId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bot(orgID, botId) })
    },
  })
}

// useActivateBot starts (or wakes) a bot's agent desktop via the full
// activation pipeline. Used as "Start" when agent_status is stopped.
export function useActivateBot(orgIDOverride?: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID: baseOrgID } = useHelixOrgBase()
  const orgID = orgIDOverride ?? baseOrgID
  return useMutation({
    mutationFn: async (botId: string) => {
      const res = await api.getApiClient().v1OrgsAgentsActivateCreate(orgID, botId)
      return res.data as BotActivateDTO
    },
    onSuccess: (_data, botId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bot(orgID, botId) })
    },
  })
}

// useStopBotAgent stops the bot's desktop sandbox; session + transcript stay.
export function useStopBotAgent(orgIDOverride?: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID: baseOrgID } = useHelixOrgBase()
  const orgID = orgIDOverride ?? baseOrgID
  return useMutation({
    mutationFn: async (botId: string) => {
      await api.getApiClient().v1OrgsAgentsStopAgentCreate(orgID, botId)
    },
    onSuccess: (_data, botId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bot(orgID, botId) })
    },
  })
}

// useRestartBotAgent fully tears down the current session and starts a
// brand-new one (fresh desktop + thread). When there is no session it
// just activates.
export function useRestartBotAgent(orgIDOverride?: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID: baseOrgID } = useHelixOrgBase()
  const orgID = orgIDOverride ?? baseOrgID
  return useMutation({
    mutationFn: async (botId: string) => {
      const res = await api.getApiClient().v1OrgsAgentsRestartAgentCreate(orgID, botId)
      return res.data as BotActivateDTO
    },
    onSuccess: (_data, botId) => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bot(orgID, botId) })
    },
  })
}

export function useListHelixOrgBots(options?: { enabled?: boolean; refetchInterval?: number | false }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.bots(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsAgentsDetail(orgID)
      return (res.data ?? []) as BotDTO[]
    },
    enabled: !!orgID && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval,
  })
}

export function useHelixOrgBot(botId: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: QUERY_KEYS.bot(orgID, botId ?? ''),
    queryFn: async () => {
      if (!botId) return null
      const res = await api.getApiClient().v1OrgsAgentsDetail2(orgID, botId)
      const agent = res.data as AgentDetailDTO
      return {
        bot: agent as BotDTO,
        agent_id: agent.agent_id ?? agent.agent_app_id,
        agent_app_id: agent.agent_app_id,
        project_id: agent.project_id,
      } as BotDetailDTO
    },
    enabled: !!orgID && !!botId && (options?.enabled ?? true),
  })
}

// The list endpoint intentionally stays compact and omits the runtime's
// project/app identifiers. The chart needs those identifiers to correlate a
// bot with the spec tasks using its agent, so fetch the details in parallel.
export function useListHelixOrgBotDetails(
  botIds: string[],
  options?: { enabled?: boolean; refetchInterval?: number | false },
): Array<BotDetailDTO | undefined> {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  const enabled = !!orgID && (options?.enabled ?? true)
  const results = useQueries({
    queries: botIds.map((botId) => ({
      queryKey: QUERY_KEYS.bot(orgID, botId),
      queryFn: async () => {
        const res = await api.getApiClient().v1OrgsAgentsDetail2(orgID, botId)
        const agent = res.data as AgentDetailDTO
        return {
          bot: agent as BotDTO,
          agent_id: agent.agent_id ?? agent.agent_app_id,
          agent_app_id: agent.agent_app_id,
          project_id: agent.project_id,
        } as BotDetailDTO
      },
      enabled: enabled && !!botId,
      refetchInterval: options?.refetchInterval,
    })),
  })
  return results.map((result) => result.data as BotDetailDTO | undefined)
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
      const res = await api.getApiClient().v1OrgsAgentsCreate(orgID, payload)
      return res.data as CreateBotResponse
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      // Creating a bot mints its s-transcript-<id> channel — refresh the
      // Triggers list / chart nodes so it shows without a reload.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.triggers(orgID) })
    },
  })
}

// useUpdateBot rewrites a Bot's content (its identity markdown), tools,
// and/or tools. The Spawner projects the new content on the next
// activation. Drives the editable content + tools panels on the bot
// detail page.
export function useUpdateBot() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: { id: string } & UpdateBotRequest) => {
      const { id, ...body } = payload
      const res = await api.getApiClient().v1OrgsAgentsPartialUpdate(orgID, id, body)
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
// manager's s-team-<mgr> chat and the pair's s-dm-<pair> channel, plus
// the manager observing the report's transcript), so we refresh triggers
// too — not just the bot list — so those new nodes render without a
// reload.
export function useAddBotParent() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ botID, parentID }: { botID: string; parentID: string }) => {
      await api.getApiClient().v1OrgsAgentsParentsCreate(orgID, botID, { parent_id: parentID })
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.triggers(orgID) })
    },
  })
}

// useRemoveBotParent drops one reporting line — the Bot no longer reports
// to parentID. Drives the chart's delete-edge flow; only the dragged
// edge's line is removed, leaving any other managers intact. The
// reconciler tears down the channels the edge implied (the manager's team
// chat when its last report leaves, and the pair's DM channel), so
// refresh triggers as well as the bot list.
export function useRemoveBotParent() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ botID, parentID }: { botID: string; parentID: string }) => {
      await api.getApiClient().v1OrgsAgentsParentsDelete(orgID, botID, parentID)
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.bots(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.triggers(orgID) })
    },
  })
}

export function useDeleteBot() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (botId: string) => {
      await api.getApiClient().v1OrgsAgentsDelete(orgID, botId)
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
      // Deleting cascades away the bot's s-transcript-<id> channel and its
      // direct reports' parent edge — refresh the Triggers list and any
      // open trigger detail.
      qc.invalidateQueries({ queryKey: QUERY_KEYS.triggers(orgID) })
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
// New Trigger "Install Helix" gate. Quiet on failure (returns null) so the
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
  // input_source is "trigger:<id>" or
  // "processor_output:<processorId>:<outputId>". Omit to leave it
  // unchanged on update; "" disconnects it.
  input_source?: string
  kind: string
  config?: Record<string, unknown>
  created_by?: string
  outputs?: { id?: string; match?: string; label?: string }[]
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.triggers(orgID) })
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
      qc.invalidateQueries({ queryKey: QUERY_KEYS.triggers(orgID) })
      qc.invalidateQueries({ queryKey: QUERY_KEYS.overview(orgID) })
    },
  })
}

// ---- Chart positions (free-placed canvas layout) ------------------------
// Nodes without a saved position fall back to the chart's auto-layout
// (dagre for bots, trigger columns, processor strip). The OpenAPI client
// is not regenerated for this yet — raw axios via useApi matches the
// providers pattern until `./stack update_openapi` picks up the swagger
// annotations on chart_positions.go.

// The 'topic' kind is the persisted wire value for a source node's saved
// position. It predates the Trigger rename and is deliberately unchanged:
// renaming it would orphan every layout a user has already saved.
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
