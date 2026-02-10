import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'
import { TypesSystemSettingsRequest, TypesSystemSettingsResponse } from '../api/api'

export const SYSTEM_SETTINGS_QUERY_KEY = ['system-settings'] as const

export function useGetSystemSettings() {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: SYSTEM_SETTINGS_QUERY_KEY,
    queryFn: async () => {
      const response = await apiClient.v1SystemSettingsList()
      return response.data
    },
  })
}

export function useUpdateSystemSettings() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (request: TypesSystemSettingsRequest) => {
      const response = await apiClient.v1SystemSettingsUpdate(request)
      return response.data
    },
    onSuccess: (data) => {
      queryClient.setQueryData(SYSTEM_SETTINGS_QUERY_KEY, data)
    },
  })
}
