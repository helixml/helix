import { useQuery } from '@tanstack/react-query'
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
 * Represents a git workspace/repository in the container
 */
export interface WorkspaceInfo {
  /** Directory name (e.g., "my-repo") */
  name: string
  /** Full path to the repository */
  path: string
  /** Currently checked out branch */
  current_branch: string
  /** Whether this is the primary repository */
  is_primary: boolean
  /** Whether the repo has a helix-specs branch */
  has_helix_specs: boolean
}

/**
 * Response from the /workspaces endpoint
 */
export interface WorkspacesResponse {
  workspaces: WorkspaceInfo[]
  error?: string
}

const WORKSPACES_QUERY_KEY = (sessionId: string) => ['external-agent-workspaces', sessionId]

/**
 * Hook for fetching workspaces (git repositories) from a running desktop container.
 */
export function useWorkspaces(sessionId: string | undefined, enabled = true) {
  const api = useApi()

  return useQuery({
    queryKey: WORKSPACES_QUERY_KEY(sessionId || ''),
    queryFn: async (): Promise<WorkspacesResponse> => {
      if (!sessionId) {
        return { workspaces: [] }
      }

      try {
        const response = await api.getApiClient().v1ExternalAgentsWorkspacesDetail(sessionId)
        return response.data as WorkspacesResponse
      } catch (err: any) {
        console.error('Failed to fetch workspaces:', err)
        return { workspaces: [], error: err?.message || 'Failed to fetch workspaces' }
      }
    },
    enabled: enabled && !!sessionId,
    staleTime: 30000, // Workspaces don't change often
    refetchOnWindowFocus: false,
  })
}
