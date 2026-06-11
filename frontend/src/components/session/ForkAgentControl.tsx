import React, { FC, useEffect, useMemo, useState } from "react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Checkbox from "@mui/material/Checkbox";
import CircularProgress from "@mui/material/CircularProgress";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import FormControlLabel from "@mui/material/FormControlLabel";
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
  // Checked by default — the safe behaviour is to push uncommitted
  // edits to the child's branch before forking so the new agent sees
  // them. Unchecking is the explicit "I know what I'm doing, abandon
  // these changes" opt-out. Reset to true whenever a new fork is
  // staged so a previously-unchecked state doesn't carry over.
  const [autoCommit, setAutoCommit] = useState(true);
  useEffect(() => {
    if (pendingTargetId) setAutoCommit(true);
  }, [pendingTargetId]);

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
        auto_commit_uncommitted: autoCommit,
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
          {/* container_reachable=false is a transient race during desktop
              startup — the polling will correct it. No need to nag the
              user with an alert that's misleading when fork still
              succeeds (which it does, because the backend's safety net
              also handles unreachable containers silently). */}
          {/* "Cannot save" path: dirty changes exist but the safety
              net has nowhere viable to push them. Surface the reason
              and block the fork — let the user fix git state in the
              terminal first (or explicitly opt to abandon changes via
              the checkbox below). */}
          {!workspaceFetching && workspace?.container_reachable && dirtyRepos.length > 0 && workspace?.can_save_changes === false && (
            <Alert severity="error" sx={{ mt: 2 }}>
              <Box sx={{ fontWeight: 600, mb: 1 }}>
                Outstanding changes can't be saved automatically
              </Box>
              <Box sx={{ mb: 1 }}>
                {workspace?.cannot_save_reason || "The safety net can't decide where to commit your changes."}
              </Box>
              <Box component="ul" sx={{ pl: 2.5, mt: 0, mb: 1 }}>
                {dirtyRepos.map((r) => (
                  <li key={r.repo_id || r.name}>
                    <strong>{r.name}</strong>
                    {r.branch ? ` (on ${r.branch})` : ""}
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

          {/* "Can save" path: normal dirty-state UX with the checkbox.
              Hidden when the can-save block above is shown so the user
              isn't offered conflicting options. */}
          {!workspaceFetching && workspace?.container_reachable && dirtyRepos.length > 0 && workspace?.can_save_changes !== false && (
            <Alert severity="warning" sx={{ mt: 2 }}>
              <Box sx={{ mb: 1 }}>
                The current session has uncommitted changes in the
                workspace. The forked child boots a fresh clone, so
                anything not pushed will be invisible to the new agent:
              </Box>
              <Box component="ul" sx={{ pl: 2.5, mt: 0, mb: 1 }}>
                {dirtyRepos.map((r) => (
                  <li key={r.repo_id || r.name}>
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
              <FormControlLabel
                control={
                  <Checkbox
                    checked={autoCommit}
                    onChange={(e) => setAutoCommit(e.target.checked)}
                    size="small"
                    disabled={pending}
                  />
                }
                label={
                  workspace?.expected_branch
                    ? `Commit and push these changes to ${workspace.expected_branch} before forking (recommended)`
                    : "Commit and push these changes before forking (recommended)"
                }
                sx={{ display: "block", mt: 0, mb: 0 }}
              />
              {!autoCommit && (
                <Box sx={{ fontSize: "0.8rem", color: "warning.dark", mt: 0.5 }}>
                  The fork will proceed but these changes will be
                  abandoned — the new agent won't see them.
                </Box>
              )}
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
          {/* Block the Fork button entirely when we know the safety
              net can't save the user's outstanding changes — better
              UX than letting them click Fork and hit a confusing
              git-push error. They can still cancel + fix git state
              + retry. */}
          {!(workspace?.container_reachable && dirtyRepos.length > 0 && workspace?.can_save_changes === false) && (
            <Button onClick={runFork} disabled={pending || workspaceFetching} variant="contained" autoFocus>
              {pending ? "Forking…" : "Fork"}
            </Button>
          )}
        </DialogActions>
      </Dialog>
    </>
  );
};

export default ForkAgentControl;
