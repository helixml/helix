import { useQuery } from "@tanstack/react-query";
import useApi from "../../hooks/useApi";

/**
 * The workspace endpoints answer 503 when the session's desktop bridge cannot
 * be dialled, which is the ordinary state of a finished or paused task rather
 * than a failure. Callers render the start-desktop placeholder for this
 * instead of an error, so a stopped sandbox never reads as a broken feature.
 */
export function isDesktopUnavailableError(error: unknown): boolean {
  return (error as { response?: { status?: number } } | null)?.response?.status === 503;
}

/**
 * A stopped sandbox is a settled state, not a flaky one. Retrying its 503 —
 * or polling straight through it — produces an endless stream of requests
 * against a container we already know is gone, each of which dials RevDial
 * and logs "container not ready" server-side. Only that status is treated as
 * terminal; anything else keeps normal retry behaviour.
 */
export const desktopQueryRetry = (failureCount: number, error: unknown) =>
  !isDesktopUnavailableError(error) && failureCount < 2;

export const desktopPollInterval =
  (interval: number) =>
  (query: { state: { error: unknown } }) =>
    isDesktopUnavailableError(query.state.error) ? false : interval;

export const workspaceReviewKeys = {
  all: (sessionId: string) => ["workspace-review", sessionId] as const,
  review: (
    sessionId: string,
    workspace: string | undefined,
    base: string,
    ignoreWhitespace: boolean,
  ) =>
    [
      ...workspaceReviewKeys.all(sessionId),
      "review",
      workspace,
      base,
      ignoreWhitespace,
    ] as const,
  files: (sessionId: string, workspace: string | undefined) =>
    [...workspaceReviewKeys.all(sessionId), "files", workspace] as const,
  skills: (sessionId: string, workspace: string | undefined) =>
    [...workspaceReviewKeys.all(sessionId), "skills", workspace] as const,
  file: (sessionId: string, workspace: string | undefined, path: string) =>
    [...workspaceReviewKeys.all(sessionId), "file", workspace, path] as const,
  turn: (sessionId: string, interactionId: string, ignoreWhitespace: boolean) =>
    [
      ...workspaceReviewKeys.all(sessionId),
      "turn",
      interactionId,
      ignoreWhitespace,
    ] as const,
  workspaces: (sessionId: string) =>
    [...workspaceReviewKeys.all(sessionId), "workspaces"] as const,
};

export function useWorkspaces(sessionId: string | undefined, enabled = true) {
  const api = useApi();
  return useQuery({
    queryKey: workspaceReviewKeys.workspaces(sessionId || ""),
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ExternalAgentsWorkspacesDetail(sessionId!);
      return response.data;
    },
    enabled: !!sessionId && enabled,
    staleTime: 30_000,
    refetchOnWindowFocus: false,
    retry: desktopQueryRetry,
  });
}

export function useWorkspaceReview(
  sessionId: string | undefined,
  workspace: string | undefined,
  base: string,
  ignoreWhitespace: boolean,
  pollInterval: number,
  enabled = true,
) {
  const api = useApi();
  return useQuery({
    queryKey: workspaceReviewKeys.review(
      sessionId || "",
      workspace,
      base,
      ignoreWhitespace,
    ),
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ExternalAgentsWorkspaceReviewDetail(sessionId!, {
          workspace,
          base,
          ignore_whitespace: ignoreWhitespace,
        });
      return response.data;
    },
    enabled: !!sessionId && enabled,
    refetchInterval: desktopPollInterval(pollInterval),
    refetchOnWindowFocus: false,
    retry: desktopQueryRetry,
  });
}

export function useWorkspaceFiles(
  sessionId: string | undefined,
  workspace: string | undefined,
  enabled = true,
) {
  const api = useApi();
  return useQuery({
    queryKey: workspaceReviewKeys.files(sessionId || "", workspace),
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ExternalAgentsWorkspaceFilesDetail(sessionId!, { workspace });
      return response.data;
    },
    enabled: !!sessionId && enabled,
    staleTime: 10_000,
    refetchOnWindowFocus: false,
    retry: desktopQueryRetry,
  });
}

export function useWorkspaceSkills(
  sessionId: string | undefined,
  workspace: string | undefined,
  enabled = true,
) {
  const api = useApi();
  return useQuery({
    queryKey: workspaceReviewKeys.skills(sessionId || "", workspace),
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ExternalAgentsWorkspaceSkillsDetail(sessionId!, { workspace });
      return response.data;
    },
    enabled: !!sessionId && enabled,
    staleTime: 30_000,
    refetchOnWindowFocus: false,
    retry: desktopQueryRetry,
  });
}

export function useWorkspaceFile(
  sessionId: string | undefined,
  workspace: string | undefined,
  path: string | null,
) {
  const api = useApi();
  return useQuery({
    queryKey: workspaceReviewKeys.file(sessionId || "", workspace, path || ""),
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ExternalAgentsWorkspaceFileDetail(sessionId!, {
          workspace,
          path: path!,
        });
      return response.data;
    },
    enabled: !!sessionId && !!path,
    staleTime: 5_000,
    refetchOnWindowFocus: false,
    retry: desktopQueryRetry,
  });
}

export function useTurnWorkspaceReview(
  sessionId: string | undefined,
  interactionId: string | undefined,
  ignoreWhitespace: boolean,
) {
  const api = useApi();
  return useQuery({
    queryKey: workspaceReviewKeys.turn(
      sessionId || "",
      interactionId || "",
      ignoreWhitespace,
    ),
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ExternalAgentsWorkspaceReviewTurnDetail(sessionId!, interactionId!, {
          ignore_whitespace: ignoreWhitespace,
        });
      return response.data;
    },
    enabled: !!sessionId && !!interactionId,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    retry: desktopQueryRetry,
  });
}
