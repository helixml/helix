import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const appStepsQueryKey = (id: string, interactionId: string) => [
  "app-steps",
  id,
  interactionId
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
