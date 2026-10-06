import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import type { TypesOrganization, TypesOrganizationMembership } from '../api/api';

export const orgListQueryKey = () => ["orgs"];

export const orgQueryNameKey = (name: string) => [
  "org",
  name
];

export const orgUsageQueryKey = (
  id: string,
  from?: string,
  to?: string,
  filters?: {
    userId?: string
    projectId?: string
    taskId?: string
    appId?: string
    sessionId?: string
    provider?: string
    model?: string
  },
  userSearch?: string,
  userLimit?: number,
  userOffset?: number,
  projectLimit?: number,
  projectOffset?: number,
  taskLimit?: number,
  taskOffset?: number,
  sessionLimit?: number,
  sessionOffset?: number,
) => [
  "org",
  id,
  "usage",
  from,
  to,
  filters?.userId,
  filters?.projectId,
  filters?.taskId,
  filters?.appId,
  filters?.sessionId,
  filters?.provider,
  filters?.model,
  userSearch,
  userLimit,
  userOffset,
  projectLimit,
  projectOffset,
  taskLimit,
  taskOffset,
  sessionLimit,
  sessionOffset,
];

export const orgMembersQueryKey = (id: string) => ["org", id, "members"];

// Live members list with presence (`online`). The account context loads
// memberships once per org switch; surfaces that show who is online poll this
// instead so a colleague opening or closing Helix shows within a minute.
export function useOrganizationMembers(
  id: string,
  options?: { enabled?: boolean; refetchInterval?: number | false },
) {
  const api = useApi();
  return useQuery({
    queryKey: orgMembersQueryKey(id),
    queryFn: async () => {
      const result = await api.getApiClient().v1OrganizationsMembersDetail(id);
      return (result.data ?? []) as TypesOrganizationMembership[];
    },
    enabled: !!id && (options?.enabled ?? true),
    refetchInterval: options?.refetchInterval,
  });
}

export function getOrgByIdQueryKey(id: string) {
  return [
    "org",
    id
  ];
}

export function useGetOrgById(id: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: getOrgByIdQueryKey(id),
    queryFn: async () => {
      const response = await apiClient.v1OrganizationsDetail(id)
      return response.data
    },
    enabled: enabled,
  })
}

export function useGetOrgByName(name: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: orgQueryNameKey(name),
    queryFn: async () => {
      const response = await apiClient.v1OrganizationsDetail(name)
      return response.data
    },    
    enabled: enabled,
  })
}

export function useCreateOrg() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (org: TypesOrganization) => {
      const response = await apiClient.v1OrganizationsCreate(org)
      return response.data
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: orgListQueryKey() })
      if (data.id) {
        queryClient.invalidateQueries({ queryKey: getOrgByIdQueryKey(data.id) })
      }
      if (data.name) {
        queryClient.invalidateQueries({ queryKey: orgQueryNameKey(data.name) })
      }
    },
  })
}

export function useDeleteOrg() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (id: string) => {
      const response = await apiClient.v1OrganizationsDelete(id)
      return response.data
    },
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: orgListQueryKey() })
      queryClient.removeQueries({ queryKey: getOrgByIdQueryKey(id) })
    },
  })
}

export function useGetOrgUsage(
  id: string,
  options?: {
    from?: string
    to?: string
    userId?: string
    projectId?: string
    taskId?: string
    appId?: string
    sessionId?: string
    provider?: string
    model?: string
    userSearch?: string
    userLimit?: number
    userOffset?: number
    projectLimit?: number
    projectOffset?: number
    taskLimit?: number
    taskOffset?: number
    sessionLimit?: number
    sessionOffset?: number
    enabled?: boolean
  },
) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: orgUsageQueryKey(
      id,
      options?.from,
      options?.to,
      {
        userId: options?.userId,
        projectId: options?.projectId,
        taskId: options?.taskId,
        appId: options?.appId,
        sessionId: options?.sessionId,
        provider: options?.provider,
        model: options?.model,
      },
      options?.userSearch,
      options?.userLimit,
      options?.userOffset,
      options?.projectLimit,
      options?.projectOffset,
      options?.taskLimit,
      options?.taskOffset,
      options?.sessionLimit,
      options?.sessionOffset,
    ),
    queryFn: async () => {
      const response = await apiClient.v1UsageOrgSummaryList({
        org_id: id,
        from: options?.from,
        to: options?.to,
        user_id: options?.userId,
        project_id: options?.projectId,
        task_id: options?.taskId,
        app_id: options?.appId,
        session_id: options?.sessionId,
        provider: options?.provider,
        model: options?.model,
        user_search: options?.userSearch,
        user_limit: options?.userLimit,
        user_offset: options?.userOffset,
        project_limit: options?.projectLimit,
        project_offset: options?.projectOffset,
        task_limit: options?.taskLimit,
        task_offset: options?.taskOffset,
        session_limit: options?.sessionLimit,
        session_offset: options?.sessionOffset,
      })
      return response.data
    },
    placeholderData: (previousData) => previousData,
    enabled: options?.enabled ?? true,
  })
}
