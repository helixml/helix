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

/**
 * Proposals that need the user (pending, or failed and awaiting a retry) and
 * have not been revealed yet. Their cards live in the chat panel, so the task
 * view opens it once per proposal.
 */
export function proposalsToReveal(
  proposals: TypesSpecTaskPRProposal[],
  revealed: ReadonlySet<string>,
  hideControls = false,
): string[] {
  if (hideControls) return [];
  return proposals
    .filter(
      (p) =>
        p.status === TypesSpecTaskPRProposalStatus.PRProposalStatusPending ||
        p.status === TypesSpecTaskPRProposalStatus.PRProposalStatusFailed,
    )
    .map((p) => p.id || "")
    .filter((id) => id && !revealed.has(id));
}

/** Whether a project's agents open pull requests (any external repository). */
export function projectHasPullRequests(
  repositories: { is_external?: boolean; external_url?: string }[],
): boolean {
  return repositories.some((repo) => !!(repo.is_external || repo.external_url));
}

// Agents propose pull requests asynchronously, so poll while the task is open.
const POLL_INTERVAL_MS = 5000;

/** Who a pull request proposal belongs to: a spec task or an org bot. */
export type PRProposalOwner =
  | { kind: "task"; specTaskId: string }
  | { kind: "bot"; orgId: string; botId: string };

export const orgBotPRProposalsQueryKey = (orgId: string, botId: string) => [
  "orgs",
  orgId,
  "bots",
  botId,
  "pr-proposals",
];

function ownerQueryKey(owner: PRProposalOwner) {
  return owner.kind === "task"
    ? prProposalsQueryKey(owner.specTaskId)
    : orgBotPRProposalsQueryKey(owner.orgId, owner.botId);
}

function ownerReady(owner: PRProposalOwner | undefined): owner is PRProposalOwner {
  if (!owner) return false;
  return owner.kind === "task" ? !!owner.specTaskId : !!owner.orgId && !!owner.botId;
}

export function usePRProposals(owner: PRProposalOwner | undefined) {
  const api = useApi();
  const apiClient = api.getApiClient();
  return useQuery({
    queryKey: ownerReady(owner) ? ownerQueryKey(owner) : ["pr-proposals", "none"],
    queryFn: async () => {
      const o = owner!;
      const response =
        o.kind === "task"
          ? await apiClient.v1SpecTasksPrProposalsDetail(o.specTaskId)
          : await apiClient.v1OrgsBotsPrProposalsDetail(o.orgId, o.botId);
      return response.data as TypesSpecTaskPRProposal[];
    },
    enabled: ownerReady(owner),
    refetchInterval: POLL_INTERVAL_MS,
  });
}

export function useSpecTaskPRProposals(specTaskId: string | undefined) {
  return usePRProposals(specTaskId ? { kind: "task", specTaskId } : undefined);
}

export function useDecidePRProposalFor(owner: PRProposalOwner) {
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
      const response =
        owner.kind === "task"
          ? await apiClient.v1SpecTasksPrProposalsDecideCreate(owner.specTaskId, proposalId, request)
          : await apiClient.v1OrgsBotsPrProposalsDecideCreate(owner.orgId, owner.botId, proposalId, request);
      return response.data as TypesSpecTaskPRProposal;
    },
    onSettled: () => {
      queryClient.invalidateQueries({ queryKey: ownerQueryKey(owner) });
      if (owner.kind === "task") {
        queryClient.invalidateQueries({ queryKey: ["spec-tasks", owner.specTaskId] });
        queryClient.invalidateQueries({ queryKey: ["spec-tasks"] });
        queryClient.invalidateQueries({ queryKey: ["attention-events"] });
      } else {
        // The bot row carries auto_approve_pull_requests, set by "approve future".
        queryClient.invalidateQueries({ queryKey: ["helix-org"] });
      }
    },
  });
}

export function useDecidePRProposal(specTaskId: string) {
  return useDecidePRProposalFor({ kind: "task", specTaskId });
}
