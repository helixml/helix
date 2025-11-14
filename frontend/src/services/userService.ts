import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const userQueryKey = (id: string) => [
  "user",
  id
];

export const userUsageQueryKey = (id: string) => [
  "user",
  id,
  "usage"
];

export const getUserByIdQueryKey = (id: string) => [
  "user",
  "details",
  id
];


export function getUserById(id: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: getUserByIdQueryKey(id),
    queryFn: async () => {
      const response = await apiClient.v1UsersDetail(id)
      return response.data
    },
    enabled: enabled,
  })
}

export function useGetConfig() {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: ["config"],
    queryFn: async () => {
      const response = await apiClient.v1ConfigList()
      return response.data
    },
  })
}

export function getUserTokenUsageQueryKey() {
  return [
    "user",
    "token-usage"
  ];
}

export function useGetUserTokenUsage() {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: getUserTokenUsageQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1UsersTokenUsageList()
      return response.data
    },
    refetchInterval: 30000, // 30 seconds
  })
}

export function useGetUserUsage(enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: userUsageQueryKey("current"),
    queryFn: async () => {
      const response = await apiClient.v1UsageList()
      return response.data
    },
    refetchInterval: 30000, // 30 seconds
    enabled: enabled ?? true,
  })
}

export function useGetUserAPIKeys() {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: userQueryKey("api-keys"),
    queryFn: async () => {
      const response = await apiClient.v1ApiKeysList()
      return response.data
    },
  })
}

export function useCreateUserAPIKey() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (data: any) => {
      const response = await apiClient.v1ApiKeysCreate(data)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userQueryKey("api-keys") })
    },
  })
}

export function useDeleteUserAPIKey() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (data: any) => {
      const response = await apiClient.v1ApiKeysDelete(data)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userQueryKey("api-keys") })
    },
  })
}

export function useRegenerateUserAPIKey() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (keyToRegenerate: string) => {
      // Delete the existing key - backend will auto-create a new one when none exist
      await apiClient.v1ApiKeysDelete({ key: keyToRegenerate })
      return keyToRegenerate
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userQueryKey("api-keys") })
    },
  })
}

export function useUpdatePassword() {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useMutation({
    mutationFn: async (newPassword: string) => {
      await apiClient.v1AuthPasswordUpdateCreate({ new_password: newPassword })
      return newPassword
    },
  })
}

export function useUpdateAccount() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (data: { full_name?: string }) => {
      const response = await apiClient.v1AuthUpdateCreate({ full_name: data.full_name })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["user"] })
    },
  })
}