import { TypesSandboxCacheState } from "../api/api";

// Golden build statuses (types.GoldenBuildStatus* on the API).
export const GOLDEN_BUILD_ACTIVE_STATUSES = ["building", "retrying"];

export const isGoldenBuildActive = (state?: TypesSandboxCacheState): boolean =>
  GOLDEN_BUILD_ACTIVE_STATUSES.includes(state?.status ?? "");

const attemptLabel = (state: TypesSandboxCacheState): string =>
  state.max_attempts
    ? `attempt ${state.attempt ?? 1} of ${state.max_attempts}`
    : `attempt ${state.attempt ?? 1}`;

// goldenBuildStatusLabel is the one-line status shown for a sandbox's golden
// build, e.g. "Building (attempt 2 of 3)".
export const goldenBuildStatusLabel = (state: TypesSandboxCacheState): string => {
  switch (state.status) {
    case "ready":
      return "Ready";
    case "building":
      return `Building (${attemptLabel(state)})`;
    case "retrying":
      return state.pending_rebuild
        ? "Interrupted, starting queued rebuild"
        : `Retrying after interruption (${attemptLabel(state)} interrupted)`;
    case "failed":
      return "Failed";
    default:
      return "No cache";
  }
};

// goldenBuildStatusDetail explains the status: why it failed, or why the
// previous attempt was interrupted. Severity drives the text colour.
export const goldenBuildStatusDetail = (
  state: TypesSandboxCacheState,
): { text: string; severity: "error" | "warning" } | undefined => {
  if (state.status === "failed" && state.error) {
    return { text: `Failed: ${state.error}`, severity: "error" };
  }
  if (state.status === "retrying" && state.interrupt_reason) {
    const when = state.next_retry_at
      ? ` — next attempt after ${new Date(state.next_retry_at).toLocaleTimeString()}`
      : "";
    return { text: `Retrying after interruption: ${state.interrupt_reason}${when}`, severity: "warning" };
  }
  if (state.status === "building" && (state.attempt ?? 1) > 1 && state.interrupt_reason) {
    return { text: `Previous attempt interrupted: ${state.interrupt_reason}`, severity: "warning" };
  }
  return undefined;
};
