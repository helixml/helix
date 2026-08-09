import React, { FC, useMemo, useState } from "react";
import Alert from "@mui/material/Alert";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import ListItemIcon from "@mui/material/ListItemIcon";
import ListItemText from "@mui/material/ListItemText";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import Tooltip from "@mui/material/Tooltip";
import { ChevronDown } from "lucide-react";
import AgentDropdown from "../agent/AgentDropdown";
import AgentHarness, { getAgentHarnessRuntime } from "../agent/AgentHarness";
import useApps from "../../hooks/useApps";
import useSnackbar from "../../hooks/useSnackbar";
import { useListProjectSpecTaskAgents } from "../../services/projectService";
import { useGetSession, useSwitchAgent } from "../../services/sessionService";
import { AGENT_KIND_CODING, IApp } from "../../types";
import { getChatColors } from "./chatStyles";

interface SwitchAgentControlProps {
  /** Session being viewed in the chat panel. The dropdown's current value
   *  reflects this session's parent_app; picking a different agent switches
   *  the agent IN PLACE on this same session (no fork, no new container). */
  sessionId: string;
  /** Project whose access-controlled coding agents can run this spec task. */
  projectId: string;
  /** Optional callback after a successful switch. The session id is
   *  unchanged, so this is just a hook for the parent to refresh/react —
   *  there is nothing to navigate to. */
  onSwitched?: () => void;
  /** Small / medium sizing forwarded to AgentDropdown. */
  size?: "small" | "medium";
  /** Compact action-button presentation for the shared chat composer. */
  displayMode?: "field" | "compact";
}

/**
 * Chat-panel agent selector that switches the agentic framework on the
 * current session IN PLACE. Unlike the old fork flow, the session keeps its
 * id, desktop container, and workspace — only the agent changes. The backend
 * restarts Zed with the new agent's config and seeds a fresh thread with a
 * normalized transcript of the prior conversation.
 *
 * Disabled on paused sessions — switch on the active descendant instead.
 *
 * See design/tasks/002111_so-we-recently-added-a/design.md.
 */
const SwitchAgentControl: FC<SwitchAgentControlProps> = ({
  sessionId,
  projectId,
  onSwitched,
  size = "small",
  displayMode = "field",
}) => {
  const apps = useApps();
  const snackbar = useSnackbar();
  const [pending, setPending] = useState(false);
  // Two-step flow: handleSelect() stages a target and opens the confirm
  // dialog; runSwitch() (the dialog's "Switch" button) fires the mutation.
  const [pendingTargetId, setPendingTargetId] = useState<string | null>(null);
  const [switchError, setSwitchError] = useState<string | null>(null);
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);

  const { data: sessionResponse } = useGetSession(sessionId, {
    enabled: !!sessionId,
    refetchInterval: 5000,
    skipInteractions: true,
  });
  const session = sessionResponse?.data;
  const switchMutation = useSwitchAgent(sessionId);
  const { data: projectAgents = [] } = useListProjectSpecTaskAgents(projectId, !!projectId);

  // The dropdown value reflects the session's parent_app (the helix app the
  // agent was launched from). For sessions without parent_app the dropdown
  // shows "Select Agent" and any selection becomes a switch.
  const currentAppId = session?.parent_app || "";

  // The project endpoint is the authoritative, access-controlled allow-list.
  // Hydrate its minimal records from AppsContext for richer harness metadata.
  // Do not filter AppsContext by agent_kind here: older API deployments omit
  // that field even though the project endpoint correctly identifies coding
  // agents, which previously made this selector empty and disabled.
  const eligibleAgents = useMemo(() => {
    return projectAgents.flatMap((agent): IApp[] => {
      if (!agent.id) return [];
      const fullAgent = apps.apps?.find((app) => app.id === agent.id);
      if (fullAgent) {
        return [{ ...fullAgent, agent_kind: AGENT_KIND_CODING }];
      }
      return [{
        id: agent.id,
        agent_kind: AGENT_KIND_CODING,
        config: {
          helix: {
            name: agent.name || "Unnamed agent",
            description: "",
            external_url: "",
            assistants: [{ code_agent_runtime: agent.code_agent_runtime || "zed_agent" }],
          },
          secrets: {},
          allowed_domains: [],
        },
      } as unknown as IApp];
    });
  }, [apps.apps, projectAgents]);

  const handleSelect = (newAppId: string) => {
    setMenuAnchor(null);
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

  const currentAgent = useMemo(() => {
    if (!currentAppId) return undefined;
    return eligibleAgents.find((a) => a.id === currentAppId);
  }, [currentAppId, eligibleAgents]);
  const currentAgentName = currentAgent?.config?.helix?.name || (currentAppId ? "the current agent" : "");

  const isPaused = !!session?.config?.paused;
  const disabled = pending || isPaused || eligibleAgents.length === 0;
  const tooltip = isPaused
    ? "This session is paused — switch on its active descendant instead"
    : "Pick a different agent to switch this session";

  return (
    <>
      {displayMode === "compact" ? (
        <>
          <Tooltip title={tooltip} placement="top" disableHoverListener={pending}>
            <Box
              component="span"
              sx={{ display: "flex", minWidth: 0, maxWidth: 180, flexShrink: 1 }}
            >
              <Button
                size="small"
                disabled={disabled}
                startIcon={currentAgent ? (
                  <AgentHarness runtime={getAgentHarnessRuntime(currentAgent)} variant="short" size={15} />
                ) : undefined}
                endIcon={<ChevronDown size={13} />}
                aria-label="Switch agent"
                onClick={(event) => setMenuAnchor(event.currentTarget)}
                sx={{
                  minWidth: 0,
                  width: "100%",
                  maxWidth: 180,
                  flexShrink: 1,
                  height: 28,
                  px: 0.75,
                  borderRadius: 1,
                  overflow: "hidden",
                  color: (theme) => getChatColors(theme).subtle,
                  fontSize: "0.75rem",
                  fontWeight: 450,
                  lineHeight: 1,
                  letterSpacing: "-0.005em",
                  textTransform: "none",
                  "& .MuiButton-startIcon": { ml: 0, mr: 0.625 },
                  "& .MuiButton-endIcon": { ml: 0.375, mr: 0 },
                  "&:hover": {
                    color: "text.primary",
                    backgroundColor: "action.hover",
                  },
                }}
              >
                <Box
                  component="span"
                  sx={{
                    minWidth: 0,
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                  }}
                >
                  {currentAgentName || "Select agent"}
                </Box>
              </Button>
            </Box>
          </Tooltip>
          <Menu
            anchorEl={menuAnchor}
            open={!!menuAnchor}
            onClose={() => setMenuAnchor(null)}
          >
            {eligibleAgents.map((agent) => (
              <MenuItem
                key={agent.id}
                selected={agent.id === currentAppId}
                onClick={() => handleSelect(agent.id || "")}
              >
                <ListItemIcon>
                  <AgentHarness runtime={getAgentHarnessRuntime(agent)} variant="short" size={16} />
                </ListItemIcon>
                <ListItemText
                  primary={agent.config?.helix?.name || "Unnamed agent"}
                />
              </MenuItem>
            ))}
          </Menu>
        </>
      ) : (
        <Tooltip title={tooltip} placement="top" disableHoverListener={pending}>
          <Box sx={{ width: "100%", minWidth: 0 }}>
            <AgentDropdown
              value={currentAppId}
              onChange={handleSelect}
              agents={eligibleAgents}
              disabled={disabled}
              size={size}
            />
          </Box>
        </Tooltip>
      )}
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
            task, branch, files, sandbox, and readable conversation history stay
            in place. This starts a new agent thread: agent-native state and the
            provider prompt cache do not carry over. Helix sends prior
            conversation as context, which can substantially increase token
            usage; older history may be truncated. There may be a brief pause
            while the new agent starts.
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
