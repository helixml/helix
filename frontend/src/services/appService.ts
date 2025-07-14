import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesTriggerConfiguration } from '../api/api';

export const appStepsQueryKey = (id: string, interactionId: string) => [
  "app-steps",
  id,
  interactionId
];

export const appTriggerStatusQueryKey = (id: string, triggerType: string) => [
  "app-trigger-status",
  id,
  triggerType
];

export const appTriggersListQueryKey = (id: string) => [
  "app-triggers",
  id
];

// Create app trigger mutation
export const createAppTriggerMutationKey = (appId: string) => [
  "create-app-trigger",
  appId
];

// Update app trigger mutation
export const updateAppTriggerMutationKey = (appId: string, triggerId: string) => [
  "update-app-trigger",
  appId,  
  triggerId
];

// Delete app trigger mutation
export const deleteAppTriggerMutationKey = (appId: string, triggerId: string) => [
  "delete-app-trigger",
  appId,
  triggerId
];

// App usage query key
export const appUsageQueryKey = (appId: string, from: string, to: string) => [
  "app-usage",
  appId,
  from,
  to
];

// useListSessionSteps returns the steps for a session, it includes
// steps for all interactions in the session
export function useListAppSteps(appId: string, interactionId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: appStepsQueryKey(appId, interactionId),
    queryFn: () => apiClient.v1AppsStepInfoDetail(appId, {interactionId}),
    enabled: options?.enabled ?? true
  })
}

export function useListAppTriggers(appId: string, options?: { enabled?: boolean, refetchInterval?: number }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: appTriggersListQueryKey(appId),
    queryFn: () => apiClient.v1AppsTriggersDetail(appId),
    enabled: options?.enabled ?? true,
    refetchInterval: options?.refetchInterval
  })
}

// useCreateAppTrigger returns a mutation for creating a new app trigger
export function useCreateAppTrigger(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (trigger: TypesTriggerConfiguration) => apiClient.v1AppsTriggersCreate(appId, trigger),
    onSuccess: () => {
      // Invalidate any cached trigger data
      queryClient.invalidateQueries({ queryKey: appTriggersListQueryKey(appId) })
    }
  })
}

// useUpdateAppTrigger returns a mutation for updating an app trigger
export function useUpdateAppTrigger(appId: string, triggerId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (trigger: TypesTriggerConfiguration) => apiClient.v1AppsTriggersUpdate(appId, triggerId, trigger),
    onSuccess: () => {
      // Invalidate any cached trigger data
      queryClient.invalidateQueries({ queryKey: appTriggersListQueryKey(appId) })
    }
  })
}

// useDeleteAppTrigger returns a mutation for deleting an app trigger
export function useDeleteAppTrigger(appId: string, triggerId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({

    mutationFn: () => apiClient.v1AppsTriggersDelete(appId, triggerId),
    onSuccess: () => {
      // Invalidate any cached trigger data
      queryClient.invalidateQueries({ queryKey: appTriggersListQueryKey(appId) })
    }
  })
}

// useGetAppTriggerStatus returns the status of a specific trigger type for an app
export function useGetAppTriggerStatus(appId: string, triggerType: string, options?: { enabled?: boolean, refetchInterval?: number }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: appTriggerStatusQueryKey(appId, triggerType),
    queryFn: () => apiClient.v1AppsTriggerStatusDetail(appId, { trigger_type: triggerType }),
    enabled: options?.enabled ?? true,
    refetchInterval: options?.refetchInterval
  })
}

// useUpdateAppAvatar returns a mutation for uploading/updating an app's avatar
export function useUpdateAppAvatar(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (file: File) => {
      // Convert file to base64 string
      const reader = new FileReader()
      const base64Promise = new Promise<string>((resolve) => {
        reader.onload = () => {
          const base64 = reader.result as string
          // Remove the data URL prefix (e.g., "data:image/jpeg;base64,")
          const base64Data = base64.split(',')[1]
          resolve(base64Data)
        }
      })
      reader.readAsDataURL(file)
      const base64Data = await base64Promise
      
      return apiClient.v1AppsAvatarCreate(appId, base64Data)
    },
    onSuccess: () => {
      // Invalidate any cached avatar data
      queryClient.invalidateQueries({ queryKey: ['app', appId] })
    }
  })
}

// useDeleteAppAvatar returns a mutation for deleting an app's avatar
export function useDeleteAppAvatar(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useMutation({
    mutationFn: () => apiClient.v1AppsAvatarDelete(appId)
  })
}

export function useGetAppUsage(appId: string, from: string, to: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: appUsageQueryKey(appId, from, to),
    queryFn: () => apiClient.v1AppsUsersDailyUsageDetail(appId, { from, to }),
  })
}

export function refreshAppUsage(appId: string, from: string, to: string) {
  // Invalidate the app usage query
  const queryClient = useQueryClient()
  queryClient.invalidateQueries({ queryKey: appUsageQueryKey(appId, from, to) })
}