import { TypesSandboxCacheState } from "../api/api";

// Golden build statuses (types.GoldenBuildStatus* on the API).
export const GOLDEN_BUILD_ACTIVE_STATUSES = ["building", "retrying"];

export const isGoldenBuildActive = (state?: TypesSandboxCacheState): boolean =>
  GOLDEN_BUILD_ACTIVE_STATUSES.includes(state?.status ?? "");

// After this long a build is probably waiting on a startup script that never
// exits; the desktop stops such scripts once they go idle.
const SLOW_BUILD_MS = 30 * 60 * 1000;

const buildElapsedMs = (state: TypesSandboxCacheState, now: number): number | undefined =>
  state.last_build_at ? now - new Date(state.last_build_at).getTime() : undefined;

const formatElapsed = (ms: number): string => {
  const minutes = Math.max(0, Math.floor(ms / 60000));
  return minutes < 60 ? `${minutes} min` : `${Math.floor(minutes / 60)} h ${minutes % 60} min`;
};

const attemptLabel = (state: TypesSandboxCacheState): string =>
  state.max_attempts
    ? `attempt ${state.attempt ?? 1} of ${state.max_attempts}`
    : `attempt ${state.attempt ?? 1}`;

// goldenBuildStatusLabel is the one-line status shown for a sandbox's golden
// build, e.g. "Building for 12 min (attempt 2 of 3)".
export const goldenBuildStatusLabel = (state: TypesSandboxCacheState, now = Date.now()): string => {
  switch (state.status) {
    case "ready":
      return "Ready";
    case "building": {
      const elapsed = buildElapsedMs(state, now);
      return elapsed === undefined
        ? `Building (${attemptLabel(state)})`
        : `Building for ${formatElapsed(elapsed)} (${attemptLabel(state)})`;
    }
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
  now = Date.now(),
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
  if (state.status === "building" && (buildElapsedMs(state, now) ?? 0) > SLOW_BUILD_MS) {
    return {
      text:
        "Taking a while. The build finishes when .helix/startup.sh exits — skip dev servers " +
        "when HELIX_GOLDEN_BUILD=true. A script left running is stopped after 20 min without build activity.",
      severity: "warning",
    };
  }
  return undefined;
};
