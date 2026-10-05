import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import useApi from "../hooks/useApi";
import {
  TypesPRProposalDecisionRequest,
  TypesSpecTaskPRProposal,
  TypesSpecTaskPRProposalStatus,
} from "../api/api";

export const prProposalsQueryKey = (specTaskId: string) => [
  "spec-tasks",
  specTaskId,
  "pr-proposals",
];

/** Proposals the user can still act on: decide, cancel, or retry. */
export const ACTIONABLE_PR_PROPOSAL_STATUSES: ReadonlySet<string> = new Set([
  TypesSpecTaskPRProposalStatus.PRProposalStatusPending,
  TypesSpecTaskPRProposalStatus.PRProposalStatusApproved,
  TypesSpecTaskPRProposalStatus.PRProposalStatusFailed,
]);

/** Whether a project's agents open pull requests (any external repository). */
export function projectHasPullRequests(
  repositories: { is_external?: boolean; external_url?: string }[],
): boolean {
  return repositories.some((repo) => !!(repo.is_external || repo.external_url));
}

// Agents propose pull requests asynchronously, so poll while the task is open.
const POLL_INTERVAL_MS = 5000;

export function useSpecTaskPRProposals(specTaskId: string | undefined) {
  const api = useApi();
  const apiClient = api.getApiClient();
  return useQuery({
    queryKey: prProposalsQueryKey(specTaskId || ""),
    queryFn: async () => {
      const response = await apiClient.v1SpecTasksPrProposalsDetail(specTaskId!);
      return response.data as TypesSpecTaskPRProposal[];
    },
    enabled: !!specTaskId,
    refetchInterval: POLL_INTERVAL_MS,
  });
}

export function useDecidePRProposal(specTaskId: string) {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      proposalId,
      request,
    }: {
      proposalId: string;
      request: TypesPRProposalDecisionRequest;
    }) => {
      const response = await apiClient.v1SpecTasksPrProposalsDecideCreate(
        specTaskId,
        proposalId,
        request,
      );
      return response.data as TypesSpecTaskPRProposal;
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: prProposalsQueryKey(specTaskId) });
      queryClient.invalidateQueries({ queryKey: ["spec-tasks", specTaskId] });
      queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });
      queryClient.invalidateQueries({ queryKey: ["attention-events"] });
    },
  });
}
