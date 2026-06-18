import React, { FC, useMemo, useState } from "react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import Tooltip from "@mui/material/Tooltip";
import AgentDropdown from "../agent/AgentDropdown";
import useApps from "../../hooks/useApps";
import useSnackbar from "../../hooks/useSnackbar";
import { useGetSession, useSwitchAgent } from "../../services/sessionService";
import { AGENT_TYPE_ZED_EXTERNAL } from "../../types";

interface SwitchAgentControlProps {
  /** Session being viewed in the chat panel. The dropdown's current value
   *  reflects this session's parent_app; picking a different agent switches
   *  the agent IN PLACE on this same session (no fork, no new container). */
  sessionId: string;
  /** Optional callback after a successful switch. The session id is
   *  unchanged, so this is just a hook for the parent to refresh/react —
   *  there is nothing to navigate to. */
  onSwitched?: () => void;
  /** Small / medium sizing forwarded to AgentDropdown. */
  size?: "small" | "medium";
}

/**
 * Chat-panel agent selector that switches the agentic framework on the
 * current session IN PLACE. Unlike the old fork flow, the session keeps its
 * id, desktop container, and workspace — only the agent changes. The backend
 * restarts Zed with the new agent's config and repopulates a fresh thread with
 * the prior transcript, so the conversation continues seamlessly.
 *
 * Disabled on paused sessions — switch on the active descendant instead.
 *
 * See design/tasks/002111_so-we-recently-added-a/design.md.
 */
const SwitchAgentControl: FC<SwitchAgentControlProps> = ({
  sessionId,
  onSwitched,
  size = "small",
}) => {
  const apps = useApps();
  const snackbar = useSnackbar();
  const [pending, setPending] = useState(false);
  // Two-step flow: handleSelect() stages a target and opens the confirm
  // dialog; runSwitch() (the dialog's "Switch" button) fires the mutation.
  const [pendingTargetId, setPendingTargetId] = useState<string | null>(null);
  const [switchError, setSwitchError] = useState<string | null>(null);

  const { data: sessionResponse } = useGetSession(sessionId, {
    enabled: !!sessionId,
    refetchInterval: 5000,
    skipInteractions: true,
  });
  const session = sessionResponse?.data;
  const switchMutation = useSwitchAgent(sessionId);

  // Filter to zed_external agents — switching only makes sense between
  // external-agent frameworks that run inside Zed.
  const eligibleAgents = useMemo(() => {
    if (!apps.apps) return [];
    return apps.apps.filter((app) =>
      app.config?.helix?.assistants?.some(
        (a) => a.agent_type === AGENT_TYPE_ZED_EXTERNAL,
      ),
    );
  }, [apps.apps]);

  // The dropdown value reflects the session's parent_app (the helix app the
  // agent was launched from). For sessions without parent_app the dropdown
  // shows "Select Agent" and any selection becomes a switch.
  const currentAppId = session?.parent_app || "";

  const handleSelect = (newAppId: string) => {
    if (!sessionId || pending || newAppId === currentAppId) return;
    setPendingTargetId(newAppId);
  };

  const cancelSwitch = () => {
    if (pending) return; // can't back out mid-flight
    setPendingTargetId(null);
    setSwitchError(null);
  };

  const runSwitch = async () => {
    if (!sessionId || pending || !pendingTargetId) return;
    const targetId = pendingTargetId;
    setPending(true);
    setSwitchError(null);
    try {
      await switchMutation.mutateAsync({ helix_app_id: targetId });
      snackbar.success("Switching agent — the conversation will continue with the new agent");
      setPendingTargetId(null);
      if (onSwitched) onSwitched();
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : "Failed to switch agent";
      setSwitchError(message);
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
    ? "This session is paused — switch on its active descendant instead"
    : "Pick a different agent to switch this session";

  return (
    <>
      {/* placement="top" keeps the tooltip above the trigger so it doesn't
          cover the first menu item when the dropdown opens. */}
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
        onClose={cancelSwitch}
        aria-labelledby="switch-confirm-title"
        aria-describedby="switch-confirm-description"
        maxWidth="xs"
        fullWidth
      >
        <DialogTitle id="switch-confirm-title">Switch agent?</DialogTitle>
        <DialogContent>
          <DialogContentText id="switch-confirm-description">
            This switches the agent for this session to{" "}
            <strong>{pendingTargetName}</strong>
            {currentAgentName ? ` (currently ${currentAgentName})` : ""}. Your
            environment, files, and workspace stay exactly as they are — only the
            agent changes. The new agent picks up with the full prior
            conversation as context. There may be a brief pause while the new
            agent starts up.
          </DialogContentText>
          {switchError && (
            <Alert severity="error" sx={{ mt: 2 }} onClose={() => setSwitchError(null)}>
              {switchError}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={cancelSwitch} disabled={pending}>
            Cancel
          </Button>
          <Button onClick={runSwitch} disabled={pending} variant="contained" autoFocus>
            {pending ? "Switching…" : "Switch"}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default SwitchAgentControl;
