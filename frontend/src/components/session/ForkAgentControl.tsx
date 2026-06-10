import React, { FC, useMemo, useState } from "react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import CircularProgress from "@mui/material/CircularProgress";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import Tooltip from "@mui/material/Tooltip";
import AgentDropdown from "../agent/AgentDropdown";
import useApps from "../../hooks/useApps";
import useRouter from "../../hooks/useRouter";
import useSnackbar from "../../hooks/useSnackbar";
import { useGetSession, useForkSession, useWorkspaceStatus } from "../../services/sessionService";
import { AGENT_TYPE_ZED_EXTERNAL } from "../../types";

interface ForkAgentControlProps {
  /** Session being viewed in the chat panel. The dropdown's current
   *  value reflects this session's parent_app (when present) and its
   *  CodeAgentRuntime; picking a different agent forks. */
  sessionId: string;
  /** Optional override of the post-fork behaviour. When set, called with
   *  the new session id instead of the default navigation. SpecTaskDetailContent
   *  uses this to remount EmbeddedSessionView on the child without
   *  taking the user out of the spec-task page. */
  onForked?: (newSessionId: string) => void;
  /** Small / medium sizing forwarded to AgentDropdown. */
  size?: "small" | "medium";
}

/**
 * Chat-panel agent selector that forks the session when the user picks
 * a different agent. Designed to sit in the session header so the user
 * can switch frameworks mid-conversation without losing context (the
 * backend seeds the child with the parent's full transcript).
 *
 * Disabled on paused sessions — the user must fork from the active
 * descendant, not from a frozen checkpoint.
 *
 * See design/tasks/002081_kickoff-mid-session/design.md.
 */
const ForkAgentControl: FC<ForkAgentControlProps> = ({
  sessionId,
  onForked,
  size = "small",
}) => {
  const apps = useApps();
  const router = useRouter();
  const snackbar = useSnackbar();
  const [pending, setPending] = useState(false);
  // Two-step flow: pickAgent() stages a target and opens the confirm
  // dialog; runFork() (called from the dialog's "Fork" button) actually
  // fires the mutation. Keeps the destructive-feeling action behind a
  // single explicit user click.
  const [pendingTargetId, setPendingTargetId] = useState<string | null>(null);

  const { data: sessionResponse } = useGetSession(sessionId, {
    enabled: !!sessionId,
    refetchInterval: 5000,
    skipInteractions: true,
  });
  const session = sessionResponse?.data;
  const forkMutation = useForkSession(sessionId);

  // Workspace status check fires only when the confirm modal is open
  // (pendingTargetId != null). It execs `git status` in each repo of
  // the parent's desktop container — too expensive to do speculatively
  // on every dropdown open without user intent.
  const { data: workspaceResp, isFetching: workspaceFetching, isError: workspaceErrored } = useWorkspaceStatus(
    sessionId,
    { enabled: !!pendingTargetId },
  );
  const workspace = workspaceResp?.data;
  const dirtyRepos = useMemo(
    () => (workspace?.repos || []).filter((r) => (r.uncommitted_files || 0) + (r.unpushed_commits || 0) > 0),
    [workspace],
  );
  const [forkError, setForkError] = useState<string | null>(null);

  // Filter to zed_external agents. Forking only makes sense between
  // external-agent frameworks; non-zed_external apps would have no
  // agent_servers config in the desktop container.
  const eligibleAgents = useMemo(() => {
    if (!apps.apps) return [];
    return apps.apps.filter((app) =>
      app.config?.helix?.assistants?.some(
        (a) => a.agent_type === AGENT_TYPE_ZED_EXTERNAL,
      ),
    );
  }, [apps.apps]);

  // The dropdown value reflects the session's parent_app (the helix
  // app the agent was launched from). For sessions without parent_app
  // (legacy / direct chat), the dropdown shows "Select Agent" and any
  // selection becomes a fork.
  const currentAppId = session?.parent_app || "";

  // Dropdown's onChange — just stage the target and open the confirm
  // dialog. The actual fork doesn't fire until the user clicks "Fork" in
  // runFork() below.
  const handleSelect = (newAppId: string) => {
    if (!sessionId || pending || newAppId === currentAppId) return;
    setPendingTargetId(newAppId);
  };

  const cancelFork = () => {
    if (pending) return; // can't back out mid-flight
    setPendingTargetId(null);
    setForkError(null);
  };

  const runFork = async () => {
    if (!sessionId || pending || !pendingTargetId) return;
    const targetId = pendingTargetId;
    setPending(true);
    setForkError(null);
    try {
      // Always opt into the pre-fork commit+push safety net. Backend
      // defaults to it too; passing explicit true makes the request
      // self-documenting and lets future UI add an "abandon changes"
      // opt-out without a separate API change.
      const result = await forkMutation.mutateAsync({
        helix_app_id: targetId,
        auto_commit_uncommitted: true,
      });
      const newId = result?.new_session_id;
      if (!newId) {
        throw new Error("server did not return new_session_id");
      }
      snackbar.success("Forked to new session");
      setPendingTargetId(null);
      if (onForked) {
        onForked(newId);
      } else {
        // Default: navigate to the standalone session page for the child.
        // Caller-specific contexts (spec task page) pass onForked instead
        // so the user stays on their task while the chat panel remounts.
        const orgId = router.params?.org_id as string | undefined;
        if (orgId) {
          router.navigate("org_session", { org_id: orgId, session_id: newId });
        } else {
          router.navigate("session", { session_id: newId });
        }
      }
    } catch (err: unknown) {
      // Backend returns HTTP 400 / 409 with a clear message; surface
      // it INSIDE the modal so the user sees what went wrong and can
      // either fix git state and retry, or cancel.
      const message =
        err instanceof Error
          ? err.message
          : "Failed to fork session";
      setForkError(message);
    } finally {
      setPending(false);
    }
  };

  const pendingTargetName = useMemo(() => {
    if (!pendingTargetId) return "";
    const app = eligibleAgents.find((a) => a.id === pendingTargetId);
    return app?.config?.helix?.name || "the selected agent";
  }, [pendingTargetId, eligibleAgents]);

  const currentAgentName = useMemo(() => {
    if (!currentAppId) return "";
    const app = eligibleAgents.find((a) => a.id === currentAppId);
    return app?.config?.helix?.name || "the current agent";
  }, [currentAppId, eligibleAgents]);

  const isPaused = !!session?.config?.paused;
  const disabled = pending || isPaused || eligibleAgents.length === 0;
  const tooltip = isPaused
    ? "This session is paused — fork from its active descendant instead"
    : "Pick a different agent to fork this session";

  return (
    <>
      {/* placement="top" keeps the tooltip above the trigger. The default
          "bottom" placement put the tooltip directly over the first menu
          item when the dropdown opened, hiding it from view. */}
      <Tooltip title={tooltip} placement="top" disableHoverListener={pending}>
        <Box sx={{ minWidth: 200 }}>
          <AgentDropdown
            value={currentAppId}
            onChange={handleSelect}
            agents={eligibleAgents}
            label="Agent"
            disabled={disabled}
            size={size}
          />
        </Box>
      </Tooltip>
      <Dialog
        open={!!pendingTargetId}
        onClose={cancelFork}
        aria-labelledby="fork-confirm-title"
        aria-describedby="fork-confirm-description"
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle id="fork-confirm-title">Switch agent?</DialogTitle>
        <DialogContent>
          <DialogContentText id="fork-confirm-description">
            Switching to <strong>{pendingTargetName}</strong> will fork this
            session into a new conversation. The current session
            {currentAgentName ? ` (with ${currentAgentName})` : ""} will be
            paused as a frozen checkpoint, and the new agent will pick up
            with the full prior transcript as context. Do you want to
            continue?
          </DialogContentText>

          {/* Workspace status panel: only surfaces when there's
              something the user needs to know (uncommitted/unpushed
              changes, an unreachable container, or a check failure).
              A clean workspace is silent — the modal stays focused on
              the agent-switch decision. */}
          {workspaceFetching && (
            <Box sx={{ mt: 2, display: "flex", alignItems: "center", gap: 1 }}>
              <CircularProgress size={14} />
              <Box sx={{ fontSize: "0.85rem", color: "text.secondary" }}>
                Checking workspace for uncommitted changes…
              </Box>
            </Box>
          )}
          {!workspaceFetching && workspaceErrored && (
            <Alert severity="warning" sx={{ mt: 2 }}>
              Couldn't check the workspace for uncommitted changes. The
              fork will still try to commit & push if anything's dirty.
            </Alert>
          )}
          {!workspaceFetching && workspace && !workspace.container_reachable && (
            <Alert severity="info" sx={{ mt: 2 }}>
              Desktop container isn't running — nothing to commit. The
              child will start clean.
            </Alert>
          )}
          {!workspaceFetching && workspace?.container_reachable && dirtyRepos.length > 0 && (
            <Alert severity="info" sx={{ mt: 2 }}>
              Pre-fork checkpoint: the following changes will be
              committed and pushed before the fork so the new agent
              sees them:
              <Box component="ul" sx={{ pl: 2.5, mt: 0.5, mb: 0 }}>
                {dirtyRepos.map((r) => (
                  <li key={r.repo_id}>
                    <strong>{r.name}</strong>
                    {r.branch ? ` (${r.branch})` : ""}
                    {": "}
                    {(r.uncommitted_files || 0) > 0 && (
                      <>{r.uncommitted_files} file{r.uncommitted_files === 1 ? "" : "s"} uncommitted</>
                    )}
                    {(r.uncommitted_files || 0) > 0 && (r.unpushed_commits || 0) > 0 && ", "}
                    {(r.unpushed_commits || 0) > 0 && (
                      <>{r.unpushed_commits} commit{r.unpushed_commits === 1 ? "" : "s"} unpushed</>
                    )}
                  </li>
                ))}
              </Box>
            </Alert>
          )}
          {forkError && (
            <Alert severity="error" sx={{ mt: 2 }} onClose={() => setForkError(null)}>
              {forkError}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={cancelFork} disabled={pending}>
            Cancel
          </Button>
          <Button onClick={runFork} disabled={pending || workspaceFetching} variant="contained" autoFocus>
            {pending ? "Forking…" : "Fork"}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default ForkAgentControl;
