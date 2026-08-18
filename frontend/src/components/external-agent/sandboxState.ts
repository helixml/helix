import { TypesSessionMetadata } from "../../api/api";

export type SandboxState = "loading" | "running" | "starting" | "absent";

export interface DerivedSandboxState {
  sandboxState: SandboxState;
  statusMessage: string;
  /**
   * A newly-created spec-task session exists before StartDesktop has written
   * any lifecycle fields. Keep that state distinct from a desktop that was
   * provisioned and subsequently stopped so callers can render the launch
   * transition instead of a misleading "Desktop Paused" action.
   *
   * It also distinguishes an external-agent session from a plain LLM chat,
   * which has no sandbox at all and must never be treated as one that stopped.
   */
  hasDesktopLifecycleState: boolean;
}

type SessionConfigWithDesiredState = TypesSessionMetadata & {
  desired_state?: string;
};

/**
 * Maps a session's config metadata onto the sandbox lifecycle state the UI
 * renders. Pure so it can be reused by any component that already holds a
 * polled session, without opening a second query for the same row.
 */
export const deriveSandboxState = (
  config?: TypesSessionMetadata,
): DerivedSandboxState => {
  if (!config) {
    return {
      sandboxState: "loading",
      statusMessage: "",
      hasDesktopLifecycleState: false,
    };
  }

  const status = config.external_agent_status || "";
  const desiredState =
    (config as SessionConfigWithDesiredState)?.desired_state || "";
  const hasContainer = !!config.container_name;
  const statusMessage = config.status_message || "";

  // Map session metadata to sandbox state.
  // Check stopped status first - it takes priority from the backend check.
  // "terminated_idle" is what the idle checker writes when it reaps a sandbox
  // that nobody used; the container name stays on the row, so without this the
  // session reads as running and every workspace request 503s forever.
  let sandboxState: SandboxState;
  if (status === "stopped" || status === "terminated_idle") {
    sandboxState = "absent";
  } else if (status === "running" || (hasContainer && desiredState === "running")) {
    sandboxState = "running";
  } else if (status === "starting") {
    sandboxState = "starting";
  } else if (desiredState === "stopped") {
    sandboxState = "absent";
  } else if (!hasContainer && desiredState === "running") {
    sandboxState = "starting";
  } else if (!hasContainer) {
    sandboxState = "absent";
  } else {
    sandboxState = "running";
  }

  return {
    sandboxState,
    statusMessage,
    hasDesktopLifecycleState: !!(status || desiredState || hasContainer),
  };
};

/**
 * True when this session is backed by a sandbox that is no longer running.
 *
 * Deliberately requires `hasDesktopLifecycleState`: a plain LLM chat session
 * has no container fields at all and derives to "absent", but its agent is not
 * offline — suppressing its streaming indicator would break ordinary chat.
 */
export const isSandboxOffline = (config?: TypesSessionMetadata): boolean => {
  const { sandboxState, hasDesktopLifecycleState } = deriveSandboxState(config);
  return hasDesktopLifecycleState && sandboxState === "absent";
};
