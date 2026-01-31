import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesTriggerConfiguration, TypesUsersAggregatedUsageMetric, TypesMemory } from '../api/api';

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

export const userTriggersListQueryKey = (orgId: string) => [
  "user-triggers",
  orgId
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

export const appTriggerExecutionsQueryKey = (triggerId: string) => [
  "app-trigger-executions",
  triggerId
];

export const appListQueryKey = (orgId: string) => [
  "apps",
  orgId
];

export const appDetailQueryKey = (appId: string) => [
  "app",
  appId
];

export const appMemoriesQueryKey = (appId: string) => [
  "app-memories",
  appId
];  

export function useListApps(orgId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  
  return useQuery({
    queryKey: appListQueryKey(orgId),
    queryFn: () => apiClient.v1AppsList({ organization_id: orgId }),
    enabled: options?.enabled ?? true
  })
}

export function useGetApp(appId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: appDetailQueryKey(appId),
    queryFn: () => apiClient.v1AppsDetail(appId),
    enabled: options?.enabled ?? true
  })
}

// useListSessionSteps returns the steps for a session, it includes
// steps for all interactions in the session
export function useListAppSteps(appId: string, interactionId: string, options?: { enabled?: boolean, refetchInterval?: number }) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: appStepsQueryKey(appId, interactionId),
    queryFn: () => apiClient.v1AppsStepInfoDetail(appId, {interactionId}),
    enabled: options?.enabled ?? true,
    refetchInterval: options?.refetchInterval
  })
}

// List all cron triggers (recurring tasks) for the user
export function useListUserCronTriggers(orgId: string, options?: { enabled?: boolean, refetchInterval?: number }) {
  const api = useApi()
  const apiClient = api.getApiClient()
  
  return useQuery({
    queryKey: userTriggersListQueryKey(orgId),
    queryFn: () => apiClient.v1TriggersList({ org_id: orgId }),
    enabled: options?.enabled ?? true,
    refetchInterval: options?.refetchInterval
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
export function useCreateAppTrigger(orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (trigger: TypesTriggerConfiguration) => apiClient.v1TriggersCreate(trigger),
    onSuccess: () => {
      // Invalidate any cached trigger data
      queryClient.invalidateQueries({ queryKey: userTriggersListQueryKey(orgId) })
    }
  })
}

// useUpdateAppTrigger returns a mutation for updating an app trigger
export function useUpdateAppTrigger(triggerId: string, orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (trigger: TypesTriggerConfiguration) => apiClient.v1TriggersUpdate(triggerId, trigger),
    onSuccess: () => {
      // Invalidate any cached trigger data
      queryClient.invalidateQueries({ queryKey: userTriggersListQueryKey(orgId) })
    }
  })
}

// useDeleteAppTrigger returns a mutation for deleting an app trigger
export function useDeleteAppTrigger(triggerId: string, orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({

    mutationFn: () => apiClient.v1TriggersDelete(triggerId),
    onSuccess: () => {
      // Invalidate any cached trigger data
      queryClient.invalidateQueries({ queryKey: userTriggersListQueryKey(orgId) })
    }
  })
}

export function useExecuteAppTrigger(triggerId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()
  
  return useMutation({
    mutationFn: () => apiClient.v1TriggersExecuteCreate(triggerId),
    onSuccess: () => {
      // Invalidate any cached trigger data
      queryClient.invalidateQueries({ queryKey: appTriggerExecutionsQueryKey(triggerId) })
    }
  })
}

export function useListAppTriggerExecutions(triggerId: string, options?: { offset?: number, limit?: number }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: appTriggerExecutionsQueryKey(triggerId),
    queryFn: () => apiClient.v1TriggersExecutionsDetail(triggerId, { offset: options?.offset ?? 0, limit: options?.limit ?? 100 }),
    enabled: !!triggerId,
    refetchInterval: 5000
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
    queryFn: async () => {
      const response = await apiClient.v1AppsUsersDailyUsageDetail(appId, { from, to })
      return response.data as unknown as TypesUsersAggregatedUsageMetric[]
    },
  })
}

// useDuplicateApp returns a mutation for duplicating an app
export function useDuplicateApp(orgId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: ({ appId, name }: { appId: string; name?: string }) => 
      apiClient.v1AppsDuplicateCreate(appId, { name }),
    onSuccess: () => {
      // Invalidate the apps list to refresh the UI
      queryClient.invalidateQueries({ queryKey: appListQueryKey(orgId) })
    }
  })
}

// useListAppMemories returns a query for listing app memories
export function useListAppMemories(appId: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: appMemoriesQueryKey(appId),
    queryFn: () => apiClient.v1AppsMemoriesDetail(appId),
    enabled: options?.enabled ?? true
  })
}

// useDeleteAppMemory returns a mutation for deleting an app memory
export function useDeleteAppMemory(appId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: (memoryId: string) => apiClient.v1AppsMemoriesDelete(appId, memoryId),
    onSuccess: () => {
      // Invalidate the memories list to refresh the UI
      queryClient.invalidateQueries({ queryKey: appMemoriesQueryKey(appId) })
    }
  })
}

