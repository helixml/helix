import React, { FC, useMemo, useState } from "react";
import {
  Box,
  Button,
  Divider,
  ListSubheader,
  Menu,
  MenuItem,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import { ChevronDown, Cpu, Monitor } from "lucide-react";
import {
  TypesCodeAgentOverrides,
  TypesCodeAgentExecutionConfig,
  TypesSandboxResourceOverrides,
  TypesAgentExecutionConfig,
  TypesSandboxRuntime,
} from "../../api/api";
import { AGENT_TYPE_ZED_EXTERNAL, IApp, IAssistantConfig } from "../../types";
import useSnackbar from "../../hooks/useSnackbar";
import {
  DEFAULT_CLAUDE_SUBSCRIPTION_MODEL,
  DEFAULT_CODEX_SUBSCRIPTION_MODEL,
} from "../agent/CodingAgentForm";
import AgentHarness, { getAgentHarnessLabel } from "../agent/AgentHarness";
import { getCodeAgentEffortOptions } from "../agent/CodeAgentEffortSelect";
import { useModelReasoningEfforts } from "../../hooks/useModelReasoningEfforts";
import SpecTaskModelPicker from "./SpecTaskModelPicker";
import { codeAgentExecutionConfigFromApp } from "../../utils/codeAgentExecutionConfig";

type MaybePromise = void | Promise<unknown>;

interface SpecTaskExecutionControlsProps {
  agents: IApp[];
  selectedAgentId: string;
  codeAgentOverrides?: TypesCodeAgentOverrides;
  currentExecutionConfig?: TypesAgentExecutionConfig;
  sandboxResourceOverrides?: TypesSandboxResourceOverrides;
  sandboxRuntime?: TypesSandboxRuntime;
  onAgentModelChange: (
    agentId: string,
    value: TypesCodeAgentOverrides,
    config?: TypesCodeAgentExecutionConfig,
  ) => MaybePromise;
  // Omitted by surfaces that don't own a resizable sandbox (plain chat
  // sessions); the compute control is then hidden rather than inert.
  onSandboxResourceOverridesChange?: (value: TypesSandboxResourceOverrides) => MaybePromise;
  onSandboxRuntimeChange?: (value: TypesSandboxRuntime) => MaybePromise;
  disabled?: boolean;
  compact?: boolean;
  grouped?: boolean;
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

const SpecTaskExecutionControls: FC<SpecTaskExecutionControlsProps> = ({
  agents,
  selectedAgentId,
  codeAgentOverrides = {},
  currentExecutionConfig,
  sandboxResourceOverrides,
  sandboxRuntime,
  onAgentModelChange,
  onSandboxResourceOverridesChange,
  onSandboxRuntimeChange,
  disabled = false,
  compact = false,
  grouped = false,
}) => {
  const snackbar = useSnackbar();
  const [agentSettingsAnchor, setAgentSettingsAnchor] = useState<HTMLElement | null>(null);
  const [cpuAnchor, setCpuAnchor] = useState<HTMLElement | null>(null);
  const [isSaving, setIsSaving] = useState(false);

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
  // Narrow the harness tier list to what the selected model actually accepts.
  // Undefined means Helix has no profile for the model, in which case the full
  // runtime list stands. See api/pkg/model/reasoning_efforts.go.
  const supportedEfforts = useModelReasoningEfforts(effectiveModel);
  const effortOptions = getCodeAgentEffortOptions(runtime, supportedEfforts);
  const effectiveSandboxResources = sandboxResourceOverrides?.vcpus
    ? sandboxResourceOverrides
    : DEFAULT_SANDBOX_PRESET;
  const sandboxLabel = `${effectiveSandboxResources.vcpus} vCPU`;
  const effectiveSandboxRuntime = sandboxRuntime
    || TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop;
  const showSandboxRuntime = sandboxRuntime !== undefined || !!onSandboxRuntimeChange;
  const sandboxRuntimeLocked = showSandboxRuntime && !onSandboxRuntimeChange;
  const effortLabel = effortOptions.find((option) => option.value === effectiveEffort)?.label || effectiveEffort;
  const agentSettingsLabel = runtime === "codex_cli" && effectiveTier === "fast"
    ? `${effortLabel} · Fast`
    : effortLabel;
  const controlsDisabled = disabled || isSaving;

  const applyCodeAgentChange = async (
    agentId: string,
    next: TypesCodeAgentOverrides,
  ) => {
    setIsSaving(true);
    try {
      const targetAgent = agents.find((candidate) => candidate.id === agentId);
      const taskConfig = targetAgent
        ? codeAgentExecutionConfigFromApp(targetAgent, next)
        : currentExecutionConfig?.code_agent_config
          ? {
              ...currentExecutionConfig.code_agent_config,
              provider_ref: next.provider_ref || currentExecutionConfig.code_agent_config.provider_ref,
              model: next.model || currentExecutionConfig.code_agent_config.model,
              reasoning_effort: next.reasoning_effort,
              service_tier: next.service_tier,
            }
          : undefined;
      await onAgentModelChange(agentId, next, taskConfig);
    } catch (err) {
      snackbar.error(err instanceof Error ? err.message : "Failed to update model configuration");
    } finally {
      setIsSaving(false);
    }
  };

  const selectModel = (agentId: string, provider: string, model: string) => {
    const targetAgent = agents.find((candidate) => candidate.id === agentId);
    const targetAssistant = getAssistant(targetAgent);
    const targetUsesSubscription = targetAssistant?.code_agent_credential_type === "subscription";
    const baseOverrides = agentId === selectedAgentId ? codeAgentOverrides : {};
    void applyCodeAgentChange(
      agentId,
      { ...baseOverrides, provider_ref: targetUsesSubscription ? "" : provider, model },
    );
  };

  const selectSandbox = async (preset: typeof SANDBOX_PRESETS[number]) => {
    if (!onSandboxResourceOverridesChange) return;
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

  const selectSandboxRuntime = async (runtime: TypesSandboxRuntime) => {
    if (!onSandboxRuntimeChange) return;
    setCpuAnchor(null);
    setIsSaving(true);
    try {
      await onSandboxRuntimeChange(runtime);
    } catch (err) {
      snackbar.error(err instanceof Error ? err.message : "Failed to update sandbox runtime");
    } finally {
      setIsSaving(false);
    }
  };

  const modelControl = (agent || currentExecutionConfig || agents.length > 0) ? (
    <SpecTaskModelPicker
      agents={agents}
      selectedAgentId={selectedAgentId}
      model={effectiveModel}
      providerRefValue={effectiveProvider}
      currentExecutionConfig={currentExecutionConfig}
      disabled={controlsDisabled}
      onSelectAgentModel={selectModel}
    />
  ) : null;

  const reasoningControl = agent || currentExecutionConfig ? (
    agent ? (
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
    ) : (
      <Box sx={{ height: 28, px: 0.75, display: "inline-flex", alignItems: "center" }}>
        <Typography variant="body2" color="text.secondary">
          {agentSettingsLabel}
        </Typography>
      </Box>
    )
  ) : null;

  const computeControl = !onSandboxResourceOverridesChange ? null : (
    <Tooltip title={`Change sandbox size (${sandboxLabel})`}>
      <Box component="span" sx={{ display: "inline-flex" }}>
        <Button
          size="small"
          disabled={controlsDisabled}
          aria-label="Change sandbox size"
          onClick={(event) => setCpuAnchor(event.currentTarget)}
          startIcon={(
            <Stack direction="row" spacing={0.375} alignItems="center">
              <Cpu size={15} />
              {effectiveSandboxRuntime === TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop && (
                <Monitor size={15} />
              )}
            </Stack>
          )}
          endIcon={<ChevronDown size={13} />}
          sx={compactButtonSx}
        >
          {sandboxLabel}
        </Button>
      </Box>
    </Tooltip>
  );

  return (
    <>
      {grouped ? (
        <Box
          aria-label="Execution configuration"
          sx={{
            display: "grid",
            gridTemplateColumns: "64px minmax(0, 1fr)",
            alignItems: "center",
            columnGap: 0.75,
            rowGap: 0.5,
            minWidth: 0,
          }}
        >
          <Typography variant="body2" color="text.secondary">Harness:</Typography>
          <Box
            aria-label={`Harness: ${getAgentHarnessLabel(runtime)}`}
            sx={{ height: 28, px: 0.75, display: "inline-flex", alignItems: "center" }}
          >
            <AgentHarness runtime={runtime} variant="long" size={16} showTooltip={false} />
          </Box>

          <Typography variant="body2" color="text.secondary">Model:</Typography>
          <Stack direction="row" alignItems="center" spacing={0.25} sx={{ minWidth: 0, flexWrap: "wrap" }}>
            {modelControl}
            {reasoningControl}
          </Stack>

          {computeControl && (
            <>
              <Typography variant="body2" color="text.secondary">Compute:</Typography>
              <Box sx={{ minWidth: 0 }}>{computeControl}</Box>
            </>
          )}
        </Box>
      ) : (
        <Stack
          direction="row"
          alignItems="center"
          spacing={0.25}
          sx={{ minWidth: 0, flexWrap: compact ? "nowrap" : "wrap" }}
        >
          {modelControl}
          {reasoningControl}
          {computeControl}
        </Stack>
      )}

      <Menu
        anchorEl={agentSettingsAnchor}
        open={!!agentSettingsAnchor}
        onClose={() => setAgentSettingsAnchor(null)}
        anchorOrigin={{ vertical: "top", horizontal: "left" }}
        transformOrigin={{ vertical: "bottom", horizontal: "left" }}
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

      <Menu
        anchorEl={cpuAnchor}
        open={!!cpuAnchor}
        onClose={() => setCpuAnchor(null)}
        anchorOrigin={{ vertical: "top", horizontal: "left" }}
        transformOrigin={{ vertical: "bottom", horizontal: "left" }}
      >
        <ListSubheader disableSticky>Compute</ListSubheader>
        {SANDBOX_PRESETS.map((preset) => (
          <MenuItem
            key={preset.vcpus}
            selected={preset.vcpus === effectiveSandboxResources.vcpus}
            disabled={controlsDisabled}
            onClick={() => void selectSandbox(preset)}
            sx={{ columnGap: 2 }}
          >
            <Typography variant="body2" sx={{ flex: 1, whiteSpace: "nowrap" }}>{preset.vcpus} vCPU</Typography>
            <Typography variant="caption" color="text.secondary" sx={{ whiteSpace: "nowrap" }}>
              {preset.description}
              {preset.vcpus === DEFAULT_SANDBOX_PRESET.vcpus ? " · Default" : ""}
            </Typography>
          </MenuItem>
        ))}

        {showSandboxRuntime && [
          <Divider key="runtime-divider" sx={{ my: 0.5 }} />,
          <ListSubheader key="runtime-heading" disableSticky>Environment</ListSubheader>,
          ...[
            {
              value: TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop,
              label: "Full Desktop",
            },
            {
              value: TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu,
              label: "Headless",
            },
          ].map((option) => {
            const selected = option.value === effectiveSandboxRuntime;
            const item = (
              <MenuItem
                key={option.value}
                selected={selected}
                disabled={controlsDisabled}
                aria-disabled={sandboxRuntimeLocked || undefined}
                onClick={sandboxRuntimeLocked
                  ? undefined
                  : () => void selectSandboxRuntime(option.value)}
                sx={{
                  columnGap: 2,
                  ...(sandboxRuntimeLocked ? { cursor: "not-allowed" } : {}),
                }}
              >
                <Typography variant="body2" sx={{ flex: 1 }}>{option.label}</Typography>
                {selected && (
                  <Typography variant="caption" color="text.secondary">Selected</Typography>
                )}
              </MenuItem>
            );
            if (!sandboxRuntimeLocked) return item;
            return (
              <Tooltip
                key={option.value}
                title="Sandbox environment can't be changed after the task starts. Start a new task to use a different environment."
                placement="right"
                describeChild
              >
                {item}
              </Tooltip>
            );
          }),
        ]}
      </Menu>
    </>
  );
};

export default SpecTaskExecutionControls;
