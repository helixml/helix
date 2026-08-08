import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'

import type { ServerPinChatRequest, TypesPinnedChat } from '../api/api'
import useApi from '../hooks/useApi'

export const pinnedChatsQueryKey = ['pinned-chats']

export const usePinnedChats = (enabled = true) => {
  const api = useApi()
  return useQuery<TypesPinnedChat[]>({
    queryKey: pinnedChatsQueryKey,
    queryFn: async () => {
      const response = await api.getApiClient().v1UsersMePinnedChatsList()
      return response.data?.pinned_chats || []
    },
    enabled,
  })
}

const useSetChatPinned = (pinned: boolean) => {
  const api = useApi()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (request: ServerPinChatRequest) => {
      const client = api.getApiClient()
      const response = pinned
        ? await client.v1UsersMePinnedChatsCreate(request)
        : await client.v1UsersMePinnedChatsDelete(request)
      return response.data
    },
    onSuccess: (data) => {
      queryClient.setQueryData(pinnedChatsQueryKey, data.pinned_chats || [])
    },
  })
}

export const usePinChat = () => useSetChatPinned(true)
export const useUnpinChat = () => useSetChatPinned(false)
