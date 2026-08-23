import { useMutation, useQueries, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  ApiAttachmentDTO,
  ApiAttachmentWriteRequest,
  ApiGitHubWebhookStatusResponse,
  ApiGitLabWebhookStatusResponse,
  ApiInstallGitHubWebhookResponse,
  ApiInstallGitLabWebhookResponse,
  ApiTriggerDTO,
  ApiTriggerWriteRequest,
} from '../api/api'
import useApi from '../hooks/useApi'
import { useHelixOrgBase } from './helixOrgService'

export type TriggerDTO = ApiTriggerDTO
export type AttachmentDTO = ApiAttachmentDTO
export type TriggerWriteRequest = ApiTriggerWriteRequest & { name: string; kind: string }

export const TRIGGER_QUERY_KEYS = {
  all: (orgID: string) => ['helix-org', orgID, 'triggers'] as const,
  one: (orgID: string, id: string) => ['helix-org', orgID, 'triggers', id] as const,
  events: (orgID: string, id: string) => ['helix-org', orgID, 'triggers', id, 'events'] as const,
  attachments: (orgID: string, workerID: string) => ['helix-org', orgID, 'agents', workerID, 'attachments'] as const,
  webhookStatus: (orgID: string, id: string) => ['helix-org', orgID, 'triggers', id, 'webhook-status'] as const,
}

export type GitHubWebhookStatusResponse = ApiGitHubWebhookStatusResponse
export type GitLabWebhookStatusResponse = ApiGitLabWebhookStatusResponse
export type InstallGitHubWebhookResponse = ApiInstallGitHubWebhookResponse
export type InstallGitLabWebhookResponse = ApiInstallGitLabWebhookResponse

// InstallWebhookFailedError marks a failure whose error snackbar the API
// layer already showed, so callers skip their own redundant toast.
export class InstallWebhookFailedError extends Error {
  constructor(message: string) {
    super(message)
    this.name = 'InstallWebhookFailedError'
  }
}

// useGitHubWebhookStatus asks GitHub whether a webhook for this
// Trigger's payload URL actually exists. Live truth beats the stored
// config, which goes stale when the hook is deleted on GitHub.
export function useGitHubWebhookStatus(triggerID: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: TRIGGER_QUERY_KEYS.webhookStatus(orgID, triggerID ?? ''),
    queryFn: async () => (await api.getApiClient().v1OrgsTriggersGithubWebhookStatusDetail(triggerID!, orgID)).data,
    enabled: !!orgID && !!triggerID && (options?.enabled ?? true),
  })
}

export function useGitLabWebhookStatus(triggerID: string | undefined, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: [...TRIGGER_QUERY_KEYS.webhookStatus(orgID, triggerID ?? ''), 'gitlab'] as const,
    queryFn: async () => (await api.getApiClient().v1OrgsTriggersGitlabWebhookStatusDetail(triggerID!, orgID)).data,
    enabled: !!orgID && !!triggerID && (options?.enabled ?? true),
  })
}

// Throws InstallWebhookFailedError on non-2xx so callers can detect the
// "snackbar already shown" sentinel and skip their own toast.
export function useInstallGitHubWebhook() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (triggerID: string) => {
      try {
        return (await api.getApiClient().v1OrgsTriggersGithubInstallWebhookCreate(triggerID, orgID)).data
      } catch (e: any) {
        throw new InstallWebhookFailedError(e?.response?.data?.error ?? e?.message ?? 'install webhook failed')
      }
    },
    onSuccess: async (_data, triggerID) => {
      await qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.one(orgID, triggerID) })
      await qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.all(orgID) })
      await qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.webhookStatus(orgID, triggerID) })
    },
  })
}

export function useInstallGitLabWebhook() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (triggerID: string) => (await api.getApiClient().v1OrgsTriggersGitlabInstallWebhookCreate(triggerID, orgID)).data,
    onSuccess: async (_data, triggerID) => {
      await qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.one(orgID, triggerID) })
      await qc.invalidateQueries({ queryKey: [...TRIGGER_QUERY_KEYS.webhookStatus(orgID, triggerID), 'gitlab'] })
    },
  })
}

export function useTriggers() {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: TRIGGER_QUERY_KEYS.all(orgID),
    queryFn: async () => (await api.getApiClient().v1OrgsTriggersDetail(orgID)).data.triggers ?? [],
    enabled: !!orgID,
  })
}

export function useTrigger(id?: string) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: TRIGGER_QUERY_KEYS.one(orgID, id ?? ''),
    queryFn: async () => (await api.getApiClient().v1OrgsTriggersDetail2(orgID, id!)).data,
    enabled: !!orgID && !!id,
  })
}

export function useTriggerEvents(id?: string) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: TRIGGER_QUERY_KEYS.events(orgID, id ?? ''),
    queryFn: async () => (await api.getApiClient().v1OrgsTriggersEventsDetail(orgID, id!, { limit: 50 })).data,
    enabled: !!orgID && !!id,
    refetchInterval: 5000,
  })
}

// useTriggerSampleEvent fetches the single most recent REAL event on a
// Trigger (page size 1) so the processor drawer can show "what an event
// from this source looks like". Returns null when the Trigger has none —
// no synthetic data.
export function useTriggerSampleEvent(id?: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: [...TRIGGER_QUERY_KEYS.events(orgID, id ?? ''), 'sample'] as const,
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsTriggersEventsDetail(orgID, id!, { limit: 1 })
      return res.data.events?.[0] ?? null
    },
    enabled: !!orgID && !!id && (options?.enabled ?? true),
  })
}

export function useTriggerEventCounts(triggerIDs: string[]) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  const results = useQueries({
    queries: triggerIDs.map((id) => ({
      queryKey: [...TRIGGER_QUERY_KEYS.events(orgID, id), 'count'],
      queryFn: async () => (await api.getApiClient().v1OrgsTriggersEventsDetail(orgID, id, { limit: 1 })).data.total ?? 0,
      enabled: !!orgID && !!id,
    })),
  })
  return triggerIDs.reduce<Record<string, number>>((counts, id, index) => {
    counts[id] = results[index]?.data ?? 0
    return counts
  }, {})
}

export function useCreateTrigger() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: TriggerWriteRequest) => (await api.getApiClient().v1OrgsTriggersCreate(orgID, payload)).data,
    onSuccess: async () => qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.all(orgID) }),
  })
}

export function useUpdateTrigger(id: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: TriggerWriteRequest) => (await api.getApiClient().v1OrgsTriggersUpdate(orgID, id, payload)).data,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.one(orgID, id) })
      await qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.all(orgID) })
    },
  })
}

export function useDeleteTrigger() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (id: string) => api.getApiClient().v1OrgsTriggersDelete(orgID, id),
    onSuccess: async () => qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.all(orgID) }),
  })
}

export function useAgentAttachments(workerID?: string) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: TRIGGER_QUERY_KEYS.attachments(orgID, workerID ?? ''),
    queryFn: async () => (await api.getApiClient().v1OrgsAgentsAttachmentsDetail(orgID, workerID!)).data.attachments ?? [],
    enabled: !!orgID && !!workerID,
  })
}

export function useAgentAttachmentsForWorkers(workerIDs: string[]) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  const results = useQueries({
    queries: workerIDs.map((workerID) => ({
      queryKey: TRIGGER_QUERY_KEYS.attachments(orgID, workerID),
      queryFn: async () => (await api.getApiClient().v1OrgsAgentsAttachmentsDetail(orgID, workerID)).data.attachments ?? [],
      enabled: !!orgID && !!workerID,
    })),
  })
  return {
    attachments: results.flatMap((result) => result.data ?? []),
    isLoading: results.some((result) => result.isLoading),
  }
}

export function useCreateAgentAttachmentForChart() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerID, source }: { workerID: string; source: ApiAttachmentWriteRequest['source'] }) =>
      (await api.getApiClient().v1OrgsAgentsAttachmentsCreate(orgID, workerID, { source })).data,
    onSuccess: async (_data, { workerID }) => qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.attachments(orgID, workerID) }),
  })
}

export function useDeleteAgentAttachmentForChart() {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async ({ workerID, attachmentID }: { workerID: string; attachmentID: string }) =>
      api.getApiClient().v1OrgsAgentsAttachmentsDelete(orgID, workerID, attachmentID),
    onSuccess: async (_data, { workerID }) => qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.attachments(orgID, workerID) }),
  })
}

export function useCreateAgentAttachment(workerID?: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (payload: ApiAttachmentWriteRequest) => (await api.getApiClient().v1OrgsAgentsAttachmentsCreate(orgID, workerID!, payload)).data,
    onSuccess: async () => qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.attachments(orgID, workerID ?? '') }),
  })
}

export function useDeleteAgentAttachment(workerID?: string) {
  const api = useApi()
  const qc = useQueryClient()
  const { orgID } = useHelixOrgBase()
  return useMutation({
    mutationFn: async (attachmentID: string) => api.getApiClient().v1OrgsAgentsAttachmentsDelete(orgID, workerID!, attachmentID),
    onSuccess: async () => qc.invalidateQueries({ queryKey: TRIGGER_QUERY_KEYS.attachments(orgID, workerID ?? '') }),
  })
}
