import React, { FC, useMemo, useState } from "react";
import {
  Alert,
  Box,
  Button,
  Divider,
  Dialog,
  DialogActions,
  DialogContent,
  DialogContentText,
  DialogTitle,
  ListSubheader,
  Menu,
  MenuItem,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import { ChevronDown, Cpu } from "lucide-react";
import {
  TypesCodeAgentOverrides,
  TypesSandboxResourceOverrides,
  TypesSpecTaskExecutionConfig,
} from "../../api/api";
import { AGENT_TYPE_ZED_EXTERNAL, IApp, IAssistantConfig } from "../../types";
import useSnackbar from "../../hooks/useSnackbar";
import {
  CODEX_SUBSCRIPTION_MODELS,
  CLAUDE_SUBSCRIPTION_MODELS,
  DEFAULT_CLAUDE_SUBSCRIPTION_MODEL,
  DEFAULT_CODEX_SUBSCRIPTION_MODEL,
} from "../agent/CodingAgentForm";
import { getCodeAgentEffortOptions } from "../agent/CodeAgentEffortSelect";
import SpecTaskModelPicker from "./SpecTaskModelPicker";

type MaybePromise = void | Promise<unknown>;

interface SpecTaskExecutionControlsProps {
  agents: IApp[];
  selectedAgentId: string;
  codeAgentOverrides?: TypesCodeAgentOverrides;
  currentExecutionConfig?: TypesSpecTaskExecutionConfig;
  sandboxResourceOverrides?: TypesSandboxResourceOverrides;
  onAgentModelChange: (agentId: string, value: TypesCodeAgentOverrides) => MaybePromise;
  onSandboxResourceOverridesChange: (value: TypesSandboxResourceOverrides) => MaybePromise;
  confirmCodeAgentChanges?: boolean;
  disabled?: boolean;
  compact?: boolean;
}

const SANDBOX_PRESETS = [
  { vcpus: 1, memory_mb: 2048, label: "1 CPU", description: "2 GB RAM" },
  { vcpus: 4, memory_mb: 8192, label: "4 CPU", description: "8 GB RAM" },
  { vcpus: 8, memory_mb: 16384, label: "8 CPU", description: "16 GB RAM" },
] as const;

const DEFAULT_SANDBOX_PRESET = SANDBOX_PRESETS[1];

const compactButtonSx = {
  height: 28,
  minWidth: 0,
  px: 0.75,
  borderRadius: 1,
  color: "text.secondary",
  fontSize: "0.75rem",
  fontWeight: 450,
  lineHeight: 1,
  letterSpacing: "-0.005em",
  textTransform: "none",
  "& .MuiButton-startIcon": { ml: 0, mr: 0.625 },
  "& .MuiButton-endIcon": { ml: 0.375, mr: 0 },
  "&:hover": { color: "text.primary", backgroundColor: "action.hover" },
} as const;

function getAssistant(agent?: IApp): IAssistantConfig | undefined {
  return agent?.config?.helix?.assistants?.find(
    (assistant) => assistant.agent_type === AGENT_TYPE_ZED_EXTERNAL,
  ) || agent?.config?.helix?.assistants?.[0];
}

function getBaseModel(assistant?: IAssistantConfig): string {
  if (!assistant) return "";
  if (
    assistant.code_agent_runtime === "claude_code" &&
    assistant.code_agent_credential_type === "subscription"
  ) {
    return assistant.claude_subscription_model || DEFAULT_CLAUDE_SUBSCRIPTION_MODEL;
  }
  if (
    assistant.code_agent_runtime === "codex_cli" &&
    assistant.code_agent_credential_type === "subscription"
  ) {
    return assistant.model || DEFAULT_CODEX_SUBSCRIPTION_MODEL;
  }
  if (assistant.code_agent_runtime === "claude_code") {
    return assistant.generation_model || assistant.model || "";
  }
  return assistant.model || assistant.generation_model || "";
}

function getBaseProvider(assistant?: IAssistantConfig): string {
  if (!assistant || assistant.code_agent_credential_type === "subscription") return "";
  if (assistant.code_agent_runtime === "claude_code") {
    return assistant.generation_model_provider || assistant.provider || "";
  }
  return assistant.provider || assistant.generation_model_provider || "";
}

function formatModel(model: string): string {
  const known = [...CODEX_SUBSCRIPTION_MODELS, ...CLAUDE_SUBSCRIPTION_MODELS]
    .find((option) => option.id === model);
  if (known) return known.label.replace(/ \(.+\)$/, "");
  return model.split("/").pop() || "Model";
}

const SpecTaskExecutionControls: FC<SpecTaskExecutionControlsProps> = ({
  agents,
  selectedAgentId,
  codeAgentOverrides = {},
  currentExecutionConfig,
  sandboxResourceOverrides,
  onAgentModelChange,
  onSandboxResourceOverridesChange,
  confirmCodeAgentChanges = false,
  disabled = false,
  compact = false,
}) => {
  const snackbar = useSnackbar();
  const [agentSettingsAnchor, setAgentSettingsAnchor] = useState<HTMLElement | null>(null);
  const [cpuAnchor, setCpuAnchor] = useState<HTMLElement | null>(null);
  const [pendingChange, setPendingChange] = useState<{
    agentId: string;
    overrides: TypesCodeAgentOverrides;
    kind: "model" | "agent" | "settings";
    previousModel?: string;
  } | null>(null);
  const [pendingDescription, setPendingDescription] = useState("");
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState("");

  const agent = useMemo(
    () => agents.find((candidate) => candidate.id === selectedAgentId),
    [agents, selectedAgentId],
  );
  const assistant = useMemo(() => getAssistant(agent), [agent]);
  const runtime = assistant?.code_agent_runtime || currentExecutionConfig?.runtime || "zed_agent";
  const effectiveModel = codeAgentOverrides.model || getBaseModel(assistant) || currentExecutionConfig?.model || "";
  const effectiveProvider = codeAgentOverrides.provider_ref || getBaseProvider(assistant) || currentExecutionConfig?.provider_ref || "";
  const effectiveEffort = codeAgentOverrides.reasoning_effort
    || assistant?.reasoning_effort
    || currentExecutionConfig?.reasoning_effort
    || "default";
  const effectiveTier = codeAgentOverrides.service_tier || currentExecutionConfig?.service_tier || "standard";
  const effortOptions = getCodeAgentEffortOptions(runtime);
  const effectiveSandboxResources = sandboxResourceOverrides?.vcpus
    ? sandboxResourceOverrides
    : DEFAULT_SANDBOX_PRESET;
  const sandboxLabel = `${effectiveSandboxResources.vcpus} vCPU`;
  const effortLabel = effortOptions.find((option) => option.value === effectiveEffort)?.label || effectiveEffort;
  const agentSettingsLabel = runtime === "codex_cli" && effectiveTier === "fast"
    ? `${effortLabel} · Fast`
    : effortLabel;
  const controlsDisabled = disabled || isSaving;

  const applyCodeAgentChange = async (
    agentId: string,
    next: TypesCodeAgentOverrides,
    description: string,
    kind: "model" | "agent" | "settings",
    previousModel?: string,
  ) => {
    setError("");
    if (confirmCodeAgentChanges) {
      setPendingChange({ agentId, overrides: next, kind, previousModel });
      setPendingDescription(description);
      return;
    }
    try {
      await onAgentModelChange(agentId, next);
    } catch (err) {
      snackbar.error(err instanceof Error ? err.message : "Failed to update model configuration");
    }
  };

  const confirmChange = async () => {
    if (!pendingChange) return;
    setIsSaving(true);
    setError("");
    try {
      await onAgentModelChange(pendingChange.agentId, pendingChange.overrides);
      setPendingChange(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to update model configuration");
    } finally {
      setIsSaving(false);
    }
  };

  const selectModel = (agentId: string, provider: string, model: string) => {
    const targetAgent = agents.find((candidate) => candidate.id === agentId);
    const targetAssistant = getAssistant(targetAgent);
    const targetUsesSubscription = targetAssistant?.code_agent_credential_type === "subscription";
    const baseOverrides = agentId === selectedAgentId ? codeAgentOverrides : {};
    const targetName = targetAgent?.config?.helix?.name || "coding agent";
    const sameAgent = agentId === selectedAgentId;
    void applyCodeAgentChange(
      agentId,
      { ...baseOverrides, provider_ref: targetUsesSubscription ? "" : provider, model },
      sameAgent
        ? `Switch ${formatModel(effectiveModel)} to ${formatModel(model)}`
        : `Switch to ${targetName} using ${formatModel(model)}`,
      sameAgent ? "model" : "agent",
      sameAgent ? effectiveModel : undefined,
    );
  };

  const selectSandbox = async (preset: typeof SANDBOX_PRESETS[number]) => {
    setCpuAnchor(null);
    setIsSaving(true);
    try {
      await onSandboxResourceOverridesChange({
        vcpus: preset.vcpus,
        memory_mb: preset.memory_mb,
      });
    } catch (err) {
      snackbar.error(err instanceof Error ? err.message : "Failed to resize sandbox");
    } finally {
      setIsSaving(false);
    }
  };

  return (
    <>
      <Stack
        direction="row"
        alignItems="center"
        spacing={0.25}
        sx={{ minWidth: 0, flexWrap: compact ? "nowrap" : "wrap" }}
      >
        {(agent || currentExecutionConfig || agents.length > 0) && (
          <SpecTaskModelPicker
            agents={agents}
            selectedAgentId={selectedAgentId}
            model={effectiveModel}
            providerRefValue={effectiveProvider}
            currentExecutionConfig={currentExecutionConfig}
            disabled={controlsDisabled}
            onSelectAgentModel={selectModel}
          />
        )}

        {agent && (
          <Tooltip title="Change reasoning and service tier">
            <Box component="span" sx={{ display: "inline-flex" }}>
              <Button
                size="small"
                disabled={controlsDisabled}
                aria-label="Change reasoning and service tier"
                onClick={(event) => setAgentSettingsAnchor(event.currentTarget)}
                endIcon={<ChevronDown size={13} />}
                sx={compactButtonSx}
              >
                {agentSettingsLabel}
              </Button>
            </Box>
          </Tooltip>
        )}

        <Tooltip title={`Change sandbox size (${sandboxLabel})`}>
          <Box component="span" sx={{ display: "inline-flex" }}>
            <Button
              size="small"
              disabled={controlsDisabled}
              aria-label="Change sandbox size"
              onClick={(event) => setCpuAnchor(event.currentTarget)}
              startIcon={<Cpu size={15} />}
              endIcon={<ChevronDown size={13} />}
              sx={compactButtonSx}
            >
              {sandboxLabel}
            </Button>
          </Box>
        </Tooltip>
      </Stack>

      <Menu
        anchorEl={agentSettingsAnchor}
        open={!!agentSettingsAnchor}
        onClose={() => setAgentSettingsAnchor(null)}
        slotProps={{ paper: { sx: { minWidth: 190 } } }}
      >
        <ListSubheader disableSticky>Reasoning</ListSubheader>
        {effortOptions.map((option) => (
          <MenuItem
            key={option.value}
            selected={option.value === effectiveEffort}
            onClick={() => {
              setAgentSettingsAnchor(null);
              const reasoning_effort = option.value === "default" ? "" : option.value;
              void applyCodeAgentChange(
                selectedAgentId,
                { ...codeAgentOverrides, reasoning_effort },
                `Change reasoning effort to ${option.label}`,
                "settings",
              );
            }}
          >
            <Typography variant="body2" sx={{ flex: 1 }}>{option.label}</Typography>
            {option.value === (assistant?.reasoning_effort || "default") && (
              <Typography variant="caption" color="text.secondary">Agent default</Typography>
            )}
          </MenuItem>
        ))}
        {runtime === "codex_cli" && [
          <Divider key="service-divider" sx={{ my: 0.5 }} />,
          <ListSubheader key="service-heading" disableSticky>Service tier</ListSubheader>,
          ...[
            { value: "", label: "Standard" },
            { value: "fast", label: "Fast" },
          ].map((option) => (
            <MenuItem
              key={option.label}
              selected={(codeAgentOverrides.service_tier || "") === option.value}
              onClick={() => {
                setAgentSettingsAnchor(null);
                void applyCodeAgentChange(
                  selectedAgentId,
                  { ...codeAgentOverrides, service_tier: option.value },
                  `Change service tier to ${option.label}`,
                  "settings",
                );
              }}
            >
              <Typography variant="body2" sx={{ flex: 1 }}>{option.label}</Typography>
              {option.value === "" && (
                <Typography variant="caption" color="text.secondary">Default</Typography>
              )}
            </MenuItem>
          )),
        ]}
      </Menu>

      <Menu anchorEl={cpuAnchor} open={!!cpuAnchor} onClose={() => setCpuAnchor(null)}>
        {SANDBOX_PRESETS.map((preset) => (
          <MenuItem
            key={preset.vcpus}
            selected={preset.vcpus === effectiveSandboxResources.vcpus}
            onClick={() => void selectSandbox(preset)}
          >
            <Box>
              <Typography variant="body2">{preset.vcpus} vCPU</Typography>
              <Typography variant="caption" color="text.secondary">
                {preset.description}{preset.vcpus === DEFAULT_SANDBOX_PRESET.vcpus ? " · Default" : ""}
              </Typography>
            </Box>
          </MenuItem>
        ))}
      </Menu>

      <Dialog
        open={!!pendingChange}
        onClose={() => !isSaving && setPendingChange(null)}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>
          {pendingChange?.kind === "model"
            ? "Switch model?"
            : pendingChange?.kind === "agent"
              ? "Switch coding agent?"
              : "Change coding configuration?"}
        </DialogTitle>
        <DialogContent>
          <DialogContentText>
            {pendingDescription}. {pendingChange?.kind === "model" ? (
              <>
                The new model cannot reuse {formatModel(pendingChange.previousModel || "the current model")}&apos;s
                provider prompt cache. Helix starts a new {agent?.config?.helix?.name || "coding agent"} thread
                and sends the prior conversation as readable context. The task, branch, files, and sandbox stay
                in place, but agent-native state does not carry over. This can substantially increase token usage;
                older history may be truncated.
              </>
            ) : (
              <>
                This starts a new agent thread. The task, branch, files, sandbox, and readable conversation
                history stay in place, but agent-native state and the provider prompt cache do not carry over.
                Helix sends prior conversation as context, which can substantially increase token usage; older
                history may be truncated.
              </>
            )}
          </DialogContentText>
          {error && <Alert severity="error" sx={{ mt: 2 }}>{error}</Alert>}
        </DialogContent>
        <DialogActions>
          <Button disabled={isSaving} onClick={() => setPendingChange(null)}>Cancel</Button>
          <Button disabled={isSaving} variant="contained" onClick={() => void confirmChange()}>
            {isSaving ? "Changing…" : "Change"}
          </Button>
        </DialogActions>
      </Dialog>
    </>
  );
};

export default SpecTaskExecutionControls;
