import { useQuery } from "@tanstack/react-query";
import useApi from "../../hooks/useApi";

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

export function useWorkspaces(sessionId: string | undefined) {
  const api = useApi();
  return useQuery({
    queryKey: workspaceReviewKeys.workspaces(sessionId || ""),
    queryFn: async () => {
      const response = await api
        .getApiClient()
        .v1ExternalAgentsWorkspacesDetail(sessionId!);
      return response.data;
    },
    enabled: !!sessionId,
    staleTime: 30_000,
    refetchOnWindowFocus: false,
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
    refetchInterval: pollInterval,
    refetchOnWindowFocus: false,
  });
}

export function useWorkspaceFiles(
  sessionId: string | undefined,
  workspace: string | undefined,
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
    enabled: !!sessionId,
    staleTime: 10_000,
    refetchOnWindowFocus: false,
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
  });
}
