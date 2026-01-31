import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi'; 

import { TypesOAuthProvider, TypesOAuthConnection, RequestParams, ContentType } from '../api/api';

export const oauthProvidersQueryKey = () => ["oauth-providers"];
export const oauthConnectionsQueryKey = () => ["oauth-connections"];
export const oauthConnectionQueryKey = (id: string) => ["oauth-connections", id];

export function useListOAuthProviders() {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: oauthProvidersQueryKey(),
    queryFn: async () => {
      const result = await apiClient.v1OauthProvidersList()
      return result.data
    },
    enabled: true,
    staleTime: 3 * 1000, // 3 seconds (useful when going between pages)
  });
}

export function useCreateOAuthProvider() {
  const api = useApi()
  const apiClient = api.getApiClient()  
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (oauthProvider: Partial<TypesOAuthProvider>) => {
      const result = await apiClient.v1OauthProvidersCreate(oauthProvider, {
        type: ContentType.Json
      } as RequestParams)
      return result.data
    },
    onSuccess: () => {
      // Invalidate OAuth provider queries to refetch the list
      queryClient.invalidateQueries({ queryKey: oauthProvidersQueryKey() })
    }
  });
}

export function useDeleteOAuthProvider() {
  const api = useApi()
  const apiClient = api.getApiClient()  
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      const result = await apiClient.v1OauthProvidersDelete(id)
      return result.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: oauthProvidersQueryKey() })
    }
  });
}

// OAuth Connections methods
export function useListOAuthConnections(refetchInterval?: number) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: oauthConnectionsQueryKey(),
    queryFn: async () => {
      const result = await apiClient.v1OauthConnectionsList()
      return result.data
    },
    enabled: true,
    staleTime: 3 * 1000, // 3 seconds (useful when going between pages)
    refetchInterval: refetchInterval,
  });
}

export function useGetOAuthConnection(id: string) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: oauthConnectionQueryKey(id),
    queryFn: async () => {
      const result = await apiClient.v1OauthConnectionsDetail(id)
      return result.data
    },
    enabled: !!id,
    staleTime: 3 * 1000, // 3 seconds (useful when going between pages)
  });
}

export function useDeleteOAuthConnection() {
  const api = useApi()
  const apiClient = api.getApiClient()  
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      const result = await apiClient.v1OauthConnectionsDelete(id)
      return result.data
    },
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: oauthConnectionsQueryKey() })
      queryClient.removeQueries({ queryKey: oauthConnectionQueryKey(id) })
    }
  });
}

export function useRefreshOAuthConnection() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      const result = await apiClient.v1OauthConnectionsRefreshCreate(id)
      return result.data
    },
    onSuccess: (_, id) => {
      queryClient.invalidateQueries({ queryKey: oauthConnectionsQueryKey() })
      queryClient.invalidateQueries({ queryKey: oauthConnectionQueryKey(id) })
    }
  });
}

// Repository listing from OAuth connections
export const oauthConnectionRepositoriesQueryKey = (id: string) => ["oauth-connection-repositories", id];

export function useListOAuthConnectionRepositories(connectionId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: oauthConnectionRepositoriesQueryKey(connectionId),
    queryFn: async () => {
      const result = await apiClient.v1OauthConnectionsRepositoriesDetail(connectionId)
      return result.data
    },
    enabled: !!connectionId,
    staleTime: 60 * 1000, // 1 minute
  });
}
