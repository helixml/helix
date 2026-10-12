import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import type { TypesApiKey } from '../api/api'

export const orgApiKeysQueryKey = (orgId: string) => ["org", orgId, "api_keys"]

export function useListOrgApiKeys(orgId: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: orgApiKeysQueryKey(orgId),
    // Metadata only: the list never carries key secrets.
    queryFn: async () => {
      const response = await apiClient.v1OrganizationsApiKeysDetail(orgId)
      return response.data
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
    mutationFn: async (keyId: string) => {
      await apiClient.v1OrganizationsApiKeysDelete(orgId, keyId)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: orgApiKeysQueryKey(orgId) })
    },
  })
}
