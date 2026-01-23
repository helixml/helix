import { useState, useEffect, useCallback, useRef } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from './useApi'

/**
 * Represents a single file's diff information
 */
export interface FileDiff {
  /** Path relative to the repository root */
  path: string
  /** Status: "added", "modified", "deleted", "renamed", "copied" */
  status: 'added' | 'modified' | 'deleted' | 'renamed' | 'copied'
  /** Old path for renamed/copied files */
  old_path?: string
  /** Number of lines added */
  additions: number
  /** Number of lines deleted */
  deletions: number
  /** Unified diff content (only if include_content=true) */
  diff?: string
  /** Whether the file is binary */
  is_binary?: boolean
}

/**
 * Response from the diff endpoint
 */
export interface DiffResponse {
  /** List of changed files */
  files: FileDiff[]
  /** Total additions across all files */
  total_additions: number
  /** Total deletions across all files */
  total_deletions: number
  /** Current branch name */
  branch?: string
  /** Base branch being compared against */
  base_branch?: string
  /** Whether there are uncommitted changes */
  has_uncommitted_changes: boolean
  /** Working directory used */
  work_dir?: string
  /** Error message if something went wrong */
  error?: string
}

interface UseLiveFileDiffOptions {
  /** Session ID to get diff from */
  sessionId: string | undefined
  /** Base branch to compare against (default: main) */
  baseBranch?: string
  /** Include full diff content for each file */
  includeContent?: boolean
  /** Filter to specific file path */
  pathFilter?: string
  /** Polling interval in ms when container is running (default: 3000) */
  pollInterval?: number
  /** Whether to enable polling (default: true when sessionId is set) */
  enabled?: boolean
}

const DIFF_QUERY_KEY = (sessionId: string) => ['external-agent-diff', sessionId]

/**
 * Hook for fetching and polling file diffs from a running desktop container.
 *
 * When the container is running, this polls for live changes.
 * Shows uncommitted/unstaged changes and diffs against the base branch.
 */
export function useLiveFileDiff({
  sessionId,
  baseBranch = 'main',
  includeContent = false,
  pathFilter,
  pollInterval = 3000,
  enabled = true,
}: UseLiveFileDiffOptions) {
  const api = useApi()
  const queryClient = useQueryClient()
  const [isLive, setIsLive] = useState(false)
  const lastSuccessTime = useRef<number>(0)

  const query = useQuery({
    queryKey: [...DIFF_QUERY_KEY(sessionId || ''), baseBranch, includeContent, pathFilter],
    queryFn: async (): Promise<DiffResponse> => {
      if (!sessionId) {
        return {
          files: [],
          total_additions: 0,
          total_deletions: 0,
          has_uncommitted_changes: false,
        }
      }

      try {
        const response = await api.getApiClient().v1ExternalAgentsDiffDetail(sessionId, {
          base: baseBranch,
          include_content: includeContent,
          path: pathFilter,
        })

        // Mark as live if we got a successful response
        lastSuccessTime.current = Date.now()
        setIsLive(true)

        // Response is typed as object, cast to our known type
        return response.data as DiffResponse
      } catch (err: any) {
        // If request fails (container not running), mark as not live
        setIsLive(false)

        // Return empty response with error
        return {
          files: [],
          total_additions: 0,
          total_deletions: 0,
          has_uncommitted_changes: false,
          error: err?.message || 'Failed to fetch diff',
        }
      }
    },
    enabled: enabled && !!sessionId,
    refetchInterval: enabled ? pollInterval : false,
    staleTime: 1000, // Consider data stale after 1 second
    refetchOnWindowFocus: false,
  })

  // Check if we've gone offline (no successful response in 2x poll interval)
  useEffect(() => {
    if (!enabled || !sessionId) return

    const checkInterval = setInterval(() => {
      if (Date.now() - lastSuccessTime.current > pollInterval * 2) {
        setIsLive(false)
      }
    }, pollInterval)

    return () => clearInterval(checkInterval)
  }, [enabled, sessionId, pollInterval])

  // Fetch diff for a specific file (with content)
  const fetchFileDiff = useCallback(async (filePath: string): Promise<FileDiff | null> => {
    if (!sessionId) return null

    try {
      const response = await api.getApiClient().v1ExternalAgentsDiffDetail(sessionId, {
        base: baseBranch,
        include_content: true,
        path: filePath,
      })

      const data = response.data as DiffResponse
      return data.files.find(f => f.path === filePath) || null
    } catch (err) {
      console.error('Failed to fetch file diff:', err)
      return null
    }
  }, [sessionId, baseBranch])

  // Force refresh
  const refresh = useCallback(() => {
    if (sessionId) {
      queryClient.invalidateQueries({ queryKey: DIFF_QUERY_KEY(sessionId) })
    }
  }, [sessionId])

  return {
    /** The diff response data */
    data: query.data,
    /** Whether the query is loading */
    isLoading: query.isLoading,
    /** Whether there was an error */
    isError: query.isError,
    /** Error object if any */
    error: query.error,
    /** Whether we're receiving live updates from the container */
    isLive,
    /** Fetch diff for a specific file with content */
    fetchFileDiff,
    /** Force refresh the diff data */
    refresh,
    /** Number of changed files */
    fileCount: query.data?.files.length || 0,
  }
}

export default useLiveFileDiff
