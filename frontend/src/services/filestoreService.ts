import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { FilestoreItem, FilestoreConfig } from '../api/api';

// Query keys for filestore operations
export const filestoreListQueryKey = (path?: string) => [
  "filestore-list",
  path || ""
];

export const filestoreItemQueryKey = (path: string) => [
  "filestore-item",
  path
];

export const filestoreConfigQueryKey = () => [
  "filestore-config"
];

// Mutation keys for filestore operations
export const deleteFilestoreItemMutationKey = (path: string) => [
  "delete-filestore-item",
  path
];

export const renameFilestoreItemMutationKey = (path: string) => [
  "rename-filestore-item",
  path
];

export const createFilestoreFolderMutationKey = (path: string) => [
  "create-filestore-folder",
  path
];

// Query hooks

/**
 * Hook to list files and folders in the filestore
 * @param path - Optional path to list. If not provided, lists root directory
 * @param enabled - Whether the query should be enabled
 */
export function useListFilestore(path?: string, enabled: boolean = true) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: filestoreListQueryKey(path),
    queryFn: async () => {
      const response = await apiClient.v1FilestoreListList({
        path: path || ""
      })
      return response.data
    },
    enabled: enabled,
  })
}

/**
 * Hook to get information about a specific file or folder
 * @param path - Path to the file or folder
 * @param enabled - Whether the query should be enabled
 */
export function useGetFilestoreItem(path: string, enabled: boolean = true) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: filestoreItemQueryKey(path),
    queryFn: async () => {
      const response = await apiClient.v1FilestoreGetList({
        path: path
      })
      return response.data
    },
    enabled: enabled && !!path,
  })
}

/**
 * Hook to get filestore configuration including user prefix and available folders
 */
export function useFilestoreConfig() {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: filestoreConfigQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1FilestoreConfigList()
      return response.data
    },
  })
}

// Mutation hooks

/**
 * Hook to delete a file or folder from the filestore
 */
export function useDeleteFilestoreItem() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: deleteFilestoreItemMutationKey(""),
    mutationFn: async (path: string) => {
      const response = await apiClient.v1FilestoreDeleteDelete({
        path: path
      })
      return response.data
    },
    onSuccess: (_, path) => {
      // Invalidate and refetch filestore list queries
      queryClient.invalidateQueries({ queryKey: ["filestore-list"] })
      
      // Invalidate the specific item query
      queryClient.invalidateQueries({ queryKey: filestoreItemQueryKey(path) })
      
      // If we deleted a folder, we might need to invalidate parent directory queries
      const parentPath = path.split('/').slice(0, -1).join('/')
      if (parentPath !== path) {
        queryClient.invalidateQueries({ queryKey: filestoreListQueryKey(parentPath) })
      }
    },
  })
}

/**
 * Hook to rename a file or folder in the filestore
 */
export function useRenameFilestoreItem() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: renameFilestoreItemMutationKey(""),
    mutationFn: async ({ path, newName }: { path: string; newName: string }) => {
      const response = await apiClient.v1FilestoreRenameUpdate({
        path: path,
        new_path: newName
      })
      return response.data
    },
    onSuccess: (_, { path }) => {
      // Invalidate and refetch filestore list queries
      queryClient.invalidateQueries({ queryKey: ["filestore-list"] })
      
      // Invalidate the specific item query
      queryClient.invalidateQueries({ queryKey: filestoreItemQueryKey(path) })
      
      // Invalidate parent directory queries
      const parentPath = path.split('/').slice(0, -1).join('/')
      if (parentPath !== path) {
        queryClient.invalidateQueries({ queryKey: filestoreListQueryKey(parentPath) })
      }
    },
  })
}

/**
 * Hook to create a new folder in the filestore
 */
export function useCreateFilestoreFolder() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: createFilestoreFolderMutationKey(""),
    mutationFn: async (path: string) => {
      const response = await apiClient.v1FilestoreFolderCreate({
        path: path
      })
      return response.data
    },
    onSuccess: (_, path) => {
      // Invalidate and refetch filestore list queries
      queryClient.invalidateQueries({ queryKey: ["filestore-list"] })
      
      // Invalidate parent directory queries
      const parentPath = path.split('/').slice(0, -1).join('/')
      if (parentPath !== path) {
        queryClient.invalidateQueries({ queryKey: filestoreListQueryKey(parentPath) })
      }
    },
  })
}

// Utility functions

/**
 * Get the file viewer URL for a given path
 * @param path - The filestore path
 * @param baseUrl - Optional base URL, defaults to current window origin
 */
export function getFilestoreViewerUrl(path: string, baseUrl?: string): string {
  const base = baseUrl || window.location.origin
  return `${base}/api/v1/filestore/viewer/${path}`
}

/**
 * Get a signed URL for public access to a file
 * @param path - The filestore path
 * @param signature - The URL signature for public access
 */
export function getFilestoreSignedUrl(path: string, signature: string, baseUrl?: string): string {
  const base = baseUrl || window.location.origin
  return `${base}/api/v1/filestore/viewer/${path}?signature=${signature}`
}
