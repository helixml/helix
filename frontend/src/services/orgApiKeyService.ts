import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import type { TypesApiKey } from '../api/api'

export interface OrgApiKeyResponse extends TypesApiKey {
  owner_email?: string
}

export const orgApiKeysQueryKey = (orgId: string) => ["org", orgId, "api_keys"]

export function useListOrgApiKeys(orgId: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: orgApiKeysQueryKey(orgId),
    queryFn: async () => {
      const response = await apiClient.v1OrganizationsApiKeysDetail(orgId)
      return response.data as OrgApiKeyResponse[]
    },
    enabled: enabled !== false && !!orgId,
  })
}

export function useCreateOrgApiKey(orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (name: string) => {
      const response = await apiClient.v1OrganizationsApiKeysCreate(orgId, { name })
      return response.data as TypesApiKey
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: orgApiKeysQueryKey(orgId) })
    },
  })
}

export function useDeleteOrgApiKey(orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (key: string) => {
      await apiClient.v1OrganizationsApiKeysDelete(orgId, key)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: orgApiKeysQueryKey(orgId) })
    },
  })
}
