import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import useApi from '../hooks/useApi';
import { FilestoreItem, FilestoreConfig } from '../api/api';
import { getRelativePath } from '../utils/filestore';

// Types for upload functionality
export interface UploadProgress {
  uploaded: number;
  total: number;
  currentFile?: string;
}

export interface UploadResult {
  success: boolean;
}

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

export const uploadFilestoreFilesMutationKey = (path: string) => [
  "upload-filestore-files",
  path
];

export const saveFilestoreFileMutationKey = (path: string) => [
  "save-filestore-file",
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

/**
 * Hook to upload files to the filestore
 */
export function useUploadFilestoreFiles() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: uploadFilestoreFilesMutationKey(""),
    mutationFn: async ({ path, files, config }: { path: string; files: File[]; config?: FilestoreConfig }) => {
      // Remove user prefix from path if config is provided and has user_prefix
      const uploadPath = config && config.user_prefix ? getRelativePath(config as any, { path, created: Date.now() } as any) : path
      
      // Upload files one by one since the generated API client expects a single file
      const results = []
      for (const file of files) {
        const response = await apiClient.v1FilestoreUploadCreate(
          { path: uploadPath },
          { files: file },
          {
            headers: {
              'Content-Type': 'multipart/form-data',
            },
          }
        )
        results.push(response.data)
      }
      return results
    },
    onSuccess: (_, { path }) => {
      // Invalidate and refetch filestore list queries
      queryClient.invalidateQueries({ queryKey: ["filestore-list"] })
      
      // Invalidate the specific directory query
      queryClient.invalidateQueries({ queryKey: filestoreListQueryKey(path) })
      
      // Invalidate parent directory queries if we're uploading to a subdirectory
      const parentPath = path.split('/').slice(0, -1).join('/')
      if (parentPath !== path) {
        queryClient.invalidateQueries({ queryKey: filestoreListQueryKey(parentPath) })
      }
    },
  })
}

/**
 * Hook to upload files with progress tracking
 */
export function useUploadFilestoreFilesWithProgress() {
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: uploadFilestoreFilesMutationKey(""),
    mutationFn: async ({ 
      path, 
      files, 
      config,
      onProgress 
    }: { 
      path: string; 
      files: File[]; 
      config?: FilestoreConfig;
      onProgress?: (progress: UploadProgress) => void 
    }) => {
      return await uploadFilesWithProgress(path, files, config, onProgress)
    },
    onSuccess: (_, { path }) => {
      // Invalidate and refetch filestore list queries
      queryClient.invalidateQueries({ queryKey: ["filestore-list"] })
      
      // Invalidate the specific directory query
      queryClient.invalidateQueries({ queryKey: filestoreListQueryKey(path) })
      
      // Invalidate parent directory queries if we're uploading to a subdirectory
      const parentPath = path.split('/').slice(0, -1).join('/')
      if (parentPath !== path) {
        queryClient.invalidateQueries({ queryKey: filestoreListQueryKey(parentPath) })
      }
    },
  })
}

/**
 * Hook to save file content by overwriting the existing file
 */
export function useSaveFilestoreFile() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationKey: saveFilestoreFileMutationKey(""),
    mutationFn: async ({ path, content, config }: { path: string; content: string; config?: FilestoreConfig }) => {
      // Create a Blob from the content
      const blob = new Blob([content], { type: 'text/plain' })
      const file = new File([blob], path.split('/').pop() || 'file', { type: 'text/plain' })
      
      // Remove user prefix from path if config is provided and has user_prefix
      // Use the full path for the upload to preserve the complete file path structure
      const uploadPath = config && config.user_prefix ? getRelativePath(config as any, { path, created: Date.now() } as any) : path
      
      // Use the API client to upload the file
      const response = await apiClient.v1FilestoreUploadCreate(
        { path: uploadPath },
        { files: file },
        {
          headers: {
            'Content-Type': 'multipart/form-data',
          },
        }
      )
      
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

/**
 * Upload files with progress tracking
 * @param path - The filestore path where files should be uploaded
 * @param files - Array of files to upload
 * @param config - Optional filestore config to remove user prefix
 * @param onProgress - Optional callback for progress updates
 * @returns Promise that resolves when all files are uploaded
 */
export async function uploadFilesWithProgress(
  path: string,
  files: File[],
  config?: FilestoreConfig,
  onProgress?: (progress: UploadProgress) => void
): Promise<UploadResult[]> {
  const results: UploadResult[] = []
  
  // Remove user prefix from path if config is provided and has user_prefix
  const uploadPath = config && config.user_prefix ? getRelativePath(config as any, { path, created: Date.now() } as any) : path
  
  for (let i = 0; i < files.length; i++) {
    const file = files[i]
    
    // Report progress
    if (onProgress) {
      onProgress({
        uploaded: i,
        total: files.length,
        currentFile: file.name
      })
    }
    
    try {
      // Create FormData for the file
      const formData = new FormData()
      formData.append('files', file)

      // Upload the file using axios which has the auth token set in its default headers
      const response = await axios.post(
        `/api/v1/filestore/upload?path=${encodeURIComponent(uploadPath)}`,
        formData,
        {
          headers: {
            'Content-Type': 'multipart/form-data',
          },
        }
      )

      results.push(response.data)
    } catch (error) {
      console.error(`Error uploading ${file.name}:`, error)
      results.push({ success: false })
    }
  }
  
  // Report final progress
  if (onProgress) {
    onProgress({
      uploaded: files.length,
      total: files.length
    })
  }
  
  return results
}
