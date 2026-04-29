import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from './useApi'
import {
  TypesSpecTaskProposal,
  TypesProposalDecisionRequest,
} from '../api/api'

const PROPOSAL_LIST_KEY = 'spec-task-proposals'
const PROJECT_PROPOSAL_LIST_KEY = 'project-pending-proposals'

/**
 * Subscribe to all proposals for a given spec task. Polls every 5 seconds so
 * the UI reflects new proposals from the agent and decision results promptly.
 * Returns React Query state plus a `decideMutation` for approve/reject actions.
 */
export function useSpecTaskProposals(taskId: string | undefined, enabled: boolean = true) {
  const api = useApi()
  const queryClient = useQueryClient()

  const queryKey = [PROPOSAL_LIST_KEY, taskId]

  const query = useQuery<TypesSpecTaskProposal[]>({
    queryKey,
    queryFn: async () => {
      if (!taskId) return []
      const result = await api.getApiClient().v1SpecTasksProposalsDetail(taskId)
      return result.data || []
    },
    enabled: enabled && !!taskId,
    refetchInterval: 5_000,
  })

  const decideMutation = useMutation({
    mutationFn: async (args: { proposalId: string; request: TypesProposalDecisionRequest }) => {
      const result = await api.getApiClient().v1ProposalsDecideCreate(args.proposalId, args.request)
      return result.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      // Also invalidate project-level pending counts since this proposal may
      // have been one of the ones counted there.
      queryClient.invalidateQueries({ queryKey: [PROJECT_PROPOSAL_LIST_KEY] })
    },
  })

  const pending = (query.data || []).filter(p => p.status === 'pending')

  return {
    ...query,
    proposals: query.data || [],
    pending,
    decide: decideMutation.mutate,
    decideAsync: decideMutation.mutateAsync,
    isDeciding: decideMutation.isPending,
  }
}

/**
 * Subscribe to pending-proposal counts for a project. Used by the kanban /
 * board to show a badge of how many proposals need attention.
 */
export function useProjectPendingProposals(projectId: string | undefined, enabled: boolean = true) {
  const api = useApi()

  const query = useQuery<TypesSpecTaskProposal[]>({
    queryKey: [PROJECT_PROPOSAL_LIST_KEY, projectId],
    queryFn: async () => {
      if (!projectId) return []
      const result = await api.getApiClient().v1ProjectsProposalsDetail(projectId)
      return result.data || []
    },
    enabled: enabled && !!projectId,
    refetchInterval: 10_000,
  })

  return {
    ...query,
    proposals: query.data || [],
    count: (query.data || []).length,
  }
}
