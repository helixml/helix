import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState } from "react";
import useApi from "../../hooks/useApi";

/**
 * The workspace endpoints answer 503 when the session's desktop bridge cannot
 * be dialled, which is the ordinary state of a finished or paused task — and
 * of one whose container is still coming up — rather than a failure. Callers
 * render a placeholder for this instead of an error, so neither a stopped nor
 * a starting sandbox reads as a broken feature.
 */
export function isDesktopUnavailableError(error: unknown): boolean {
  return (error as { response?: { status?: number } } | null)?.response?.status === 503;
}

export function getWorkspaceFileSaveError(error: unknown, path: string): string {
  const status = (error as { response?: { status?: number } } | null)?.response
    ?.status;
  if (status === 409) {
    return `${path} changed outside the editor. Reload it before saving.`;
  }
  if (status === 405) {
    return "This desktop was started before file editing was available. Copy your unsaved changes, then stop and start the desktop.";
  }
  return `Could not save ${path}`;
}

/**
 * A stopped sandbox is a settled state, not a flaky one. Retrying its 503
 * immediately produces a burst of requests against a container we already
 * know is not answering, each of which dials RevDial and logs "container not
 * ready" server-side. Only that status skips the retries; anything else keeps
 * normal retry behaviour. Recovery comes from the slow poll below, not from
 * hammering a single fetch.
 */
export const desktopQueryRetry = (failureCount: number, error: unknown) =>
  !isDesktopUnavailableError(error) && failureCount < 2;

/**
 * How often to re-check a sandbox that answered 503.
 *
 * The status covers two situations that are indistinguishable over HTTP: a
 * sandbox that is stopped, and one that is still registering RevDial in the
 * seconds after the task started. Treating it as terminal left the changes
 * view frozen on "Desktop not running" for a task that was already working —
 * the bridge came up one second after the 503, and nothing asked again until
 * the page was reloaded. These queries are already gated on the session's own
 * desktop state, so a query that runs at all belongs to a sandbox that is
 * meant to be up; keep checking it, just slowly.
 */
export const DESKTOP_RECOVERY_POLL_INTERVAL = 5_000;

/**
 * How long a 503 reads as "still connecting" before it reads as stopped.
 *
 * Bounds the optimistic state: a session row that claims to be running while
 * its container is really gone — a host that restarted, a sandbox reaped
 * without the row catching up — still reaches the placeholder that offers to
 * start the desktop, instead of spinning forever.
 *
 * Measured in elapsed time rather than failed attempts on purpose. React
 * Query resets a query's failureCount on every new fetch, so it counts
 * retries within one fetch and never accumulates across polls.
 */
export const DESKTOP_RECONNECT_GRACE_MS = 30_000;

export const desktopPollInterval =
  (interval: number | false) =>
  (query: { state: { error: unknown } }): number | false =>
    isDesktopUnavailableError(query.state.error)
      ? DESKTOP_RECOVERY_POLL_INTERVAL
      : interval;

/**
 * How a sandbox is currently answering the workspace endpoints.
 *
 * "reachable" covers both a sandbox that is answering and one nothing has
 * asked about yet; the two placeholders only care about the failure states.
 */
export type DesktopReachability = "reachable" | "connecting" | "unreachable";

/**
 * Classifies an unreachable sandbox as still coming up or as gone, by how
 * long it has been failing.
 *
 * `settled` — not the mere absence of a 503 — is what ends the streak. React
 * Query clears a query's error at the start of every refetch when it holds no
 * cached data, so a poll against an unreachable sandbox flickers through
 * "no error" every few seconds. Keying off that restarted the grace period on
 * every poll (so a dead sandbox never stopped saying "connecting") and blanked
 * the placeholder for the duration of each request.
 *
 * The streak start is recorded during render rather than in an effect so the
 * first unreachable render already reads as connecting — an effect would let
 * the "sandbox is stopped" placeholder paint for a frame first.
 */
export function useDesktopReachability({
  unavailable,
  settled,
}: {
  /** The sandbox answered 503 just now. */
  unavailable: boolean;
  /** The sandbox answered, or is no longer expected to. */
  settled: boolean;
}): DesktopReachability {
  const unavailableSince = useRef<number | null>(null);
  const [, rerender] = useState(0);

  if (settled) {
    unavailableSince.current = null;
  } else if (unavailable && unavailableSince.current === null) {
    unavailableSince.current = Date.now();
  }

  const since = unavailableSince.current;

  // Polling re-renders this anyway, but only a timer guarantees the hand-off
  // to the stopped placeholder happens on time.
  useEffect(() => {
    if (since === null) return;
    const remaining = DESKTOP_RECONNECT_GRACE_MS - (Date.now() - since);
    if (remaining <= 0) return;
    const timer = setTimeout(() => rerender((tick) => tick + 1), remaining);
    return () => clearTimeout(timer);
  }, [since]);

  if (since === null) return "reachable";
  return Date.now() - since < DESKTOP_RECONNECT_GRACE_MS
    ? "connecting"
    : "unreachable";
}

export const workspaceReviewKeys = {
  all: (sessionId: string) => ["workspace-review", sessionId] as const,
  reviewPrefix: (sessionId: string) =>
    [...workspaceReviewKeys.all(sessionId), "review"] as const,
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
    // The workspace list is static once loaded, so it polls only while the
    // sandbox is unreachable — otherwise the surface stays on the
    // start-desktop placeholder for the whole life of a running task.
    refetchInterval: desktopPollInterval(false),
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
    refetchInterval: desktopPollInterval(false),
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

export function useUpdateWorkspaceFile(
  sessionId: string,
  workspace: string | undefined,
  path: string,
) {
  const api = useApi();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ contents, expectedContentHash }: {
      contents: string;
      expectedContentHash: string;
    }) => {
      const response = await api
        .getApiClient()
        .v1ExternalAgentsWorkspaceFileUpdate(sessionId, {
          workspace,
          path,
          contents,
          expected_content_hash: expectedContentHash,
        });
      return response.data;
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({
        queryKey: workspaceReviewKeys.reviewPrefix(sessionId),
      });
      void queryClient.invalidateQueries({
        queryKey: workspaceReviewKeys.files(sessionId, workspace),
      });
    },
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
