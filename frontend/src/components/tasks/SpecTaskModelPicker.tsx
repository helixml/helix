import React, { FC, useEffect, useMemo, useRef, useState } from "react";
import {
  Box,
  Button,
  CircularProgress,
  IconButton,
  InputBase,
  Popover,
  Stack,
  Tooltip,
  Typography,
} from "@mui/material";
import { ChevronDown, Search } from "lucide-react";
import type {
  TypesProviderEndpoint,
  TypesAgentExecutionConfig,
  TypesOrgCodeAgentHarnessStatus,
} from "../../api/api";
import { AGENT_TYPE_ZED_EXTERNAL, IApp, IAssistantConfig } from "../../types";
import { useGetOrgByName } from "../../services/orgService";
import { useListProviders } from "../../services/providersService";
import {
  findHarnessStatus,
  useOrgCodeAgentHarnesses,
} from "../../services/codeAgentHarnessesService";
import useRouter from "../../hooks/useRouter";
import { matchesAllTokens } from "../../utils/searchUtils";
import AgentHarness from "../agent/AgentHarness";
import {
  CLAUDE_SUBSCRIPTION_MODELS,
  CODEX_SUBSCRIPTION_MODELS,
} from "../agent/CodingAgentForm";
import {
  matchesStoredRef,
  ProviderIcon,
  providerRef,
} from "../create/AdvancedModelPicker";
import {
  providerEndpointIsConnected,
  providersForCodeAgentHarness,
  providersForCodeAgentRuntime,
} from "../../utils/codeAgentProviders";

export type TaskModelOption = { id: string; label: string };

interface SpecTaskModelPickerProps {
  agents: IApp[];
  selectedAgentId: string;
  model: string;
  providerRefValue: string;
  currentExecutionConfig?: TypesAgentExecutionConfig;
  disabled?: boolean;
  onSelectAgentModel: (agentId: string, provider: string, model: string) => void;
}

interface PickerModel {
  key: string;
  label: string;
  provider?: TypesProviderEndpoint;
  providerLabel: string;
  id: string;
}

interface PickerAgent {
  agent: IApp;
  assistant?: IAssistantConfig;
  models: PickerModel[];
}

const triggerSx = {
  height: 28,
  minWidth: 0,
  maxWidth: 190,
  px: 1,
  gap: 0.75,
  borderRadius: 1,
  color: "text.secondary",
  fontSize: "0.75rem",
  fontWeight: 450,
  lineHeight: 1,
  letterSpacing: "-0.005em",
  textTransform: "none",
  "&:hover": { color: "text.primary", backgroundColor: "action.hover" },
} as const;

const SpecTaskModelPickerView: FC<{
  agents: PickerAgent[];
  selectedAgentId: string;
  model: string;
  providerRefValue: string;
  currentExecutionConfig?: TypesAgentExecutionConfig;
  loading?: boolean;
  disabled?: boolean;
  onSelectAgentModel: (agentId: string, provider: string, model: string) => void;
}> = ({ agents, selectedAgentId, model, providerRefValue, currentExecutionConfig, loading = false, disabled = false, onSelectAgentModel }) => {
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const [query, setQuery] = useState("");
  const [browsedAgentId, setBrowsedAgentId] = useState("");
  const searchRef = useRef<HTMLInputElement>(null);

  const activeAgent = agents.find(({ agent }) => agent.id === selectedAgentId);
  const browsedAgent = agents.find(({ agent }) => agent.id === browsedAgentId) || activeAgent || agents[0];
  const searching = query.trim().length > 0;
  const visibleModels = useMemo(() => {
    const source = searching
      ? agents.flatMap((agent) => agent.models.map((option) => ({ agent, option })))
      : (browsedAgent?.models || []).map((option) => ({ agent: browsedAgent, option }));
    const seen = new Set<string>();
    return source.filter(({ agent, option }) => {
      const key = `${agent.agent.id}:${option.key}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return !searching || matchesAllTokens(
        query,
        option.label,
        option.id,
        option.providerLabel,
        agent.agent.config?.helix?.name,
      );
    });
  }, [agents, browsedAgent, query, searching]);

  useEffect(() => {
    if (!anchor) return;
    setBrowsedAgentId(activeAgent?.agent.id || agents[0]?.agent.id || "");
    setQuery("");
    requestAnimationFrame(() => searchRef.current?.focus());
  }, [activeAgent?.agent.id, agents, anchor]);

  const activeRuntime = activeAgent?.assistant?.code_agent_runtime
    || currentExecutionConfig?.runtime
    || "zed_agent";
  const activeModel = activeAgent?.models.find((option) => option.id === model
    && (!option.provider || matchesStoredRef(option.provider, providerRefValue)))
    || activeAgent?.models.find((option) => option.id === model);
  const activeModelLabel = activeModel?.label.replace(/ \(.+\)$/, "")
    || model.split("/").pop()
    || (currentExecutionConfig?.agent_name
      ? `${currentExecutionConfig.agent_name} model`
      : "Current model");

  return (
    <>
      <Tooltip
        title={(
          <AgentHarness
            runtime={activeRuntime}
            variant="long"
            size={16}
            showTooltip={false}
          />
        )}
        placement="top"
      >
        <Box component="span" sx={{ display: "inline-flex", minWidth: 0 }}>
          <Button
            aria-label="Change coding model"
            disabled={disabled}
            onClick={(event) => setAnchor(event.currentTarget)}
            sx={triggerSx}
          >
            <Box component="span" sx={{ display: "inline-flex", flexShrink: 0 }}>
              {activeModel?.provider
                ? <ProviderIcon provider={activeModel.provider} size={16} />
                : <AgentHarness runtime={activeRuntime} variant="short" size={16} showTooltip={false} />}
            </Box>
            <Box component="span" sx={{ minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
              {activeModelLabel}
            </Box>
            <ChevronDown size={13} aria-hidden="true" />
          </Button>
        </Box>
      </Tooltip>

      <Popover
        open={!!anchor}
        anchorEl={anchor}
        onClose={() => setAnchor(null)}
        anchorOrigin={{ vertical: "top", horizontal: "left" }}
        transformOrigin={{ vertical: "bottom", horizontal: "left" }}
        slotProps={{
          paper: {
            sx: {
              width: 370,
              height: 390,
              maxWidth: "calc(100vw - 16px)",
              borderRadius: 1.5,
              border: "1px solid",
              borderColor: "divider",
              boxShadow: 12,
              overflow: "hidden",
            },
          },
        }}
      >
        <Stack direction="row" sx={{ height: "100%" }}>
          {!searching && (
            <Stack
              spacing={0.5}
              sx={{
                width: 46,
                minHeight: 0,
                flexShrink: 0,
                p: 0.5,
                bgcolor: "background.paper",
                borderRight: "1px solid",
                borderColor: "divider",
                overflowY: "auto",
                overflowX: "hidden",
                overscrollBehavior: "contain",
              }}
            >
              {agents.map(({ agent, assistant }) => {
                const selected = agent.id === browsedAgent?.agent.id;
                const name = agent.config?.helix?.name || "Unnamed agent";
                return (
                  <IconButton
                    key={agent.id}
                    aria-label={name}
                    onClick={() => setBrowsedAgentId(agent.id)}
                    sx={{
                      width: 38,
                      height: 38,
                      borderRadius: 1,
                      color: selected ? "text.primary" : "text.secondary",
                      bgcolor: selected ? "action.selected" : "transparent",
                      "&:hover": { bgcolor: "action.selected" },
                    }}
                  >
                    <AgentHarness
                      runtime={assistant?.code_agent_runtime || "zed_agent"}
                      variant="short"
                      size={20}
                      tooltipPlacement="left"
                    />
                  </IconButton>
                );
              })}
            </Stack>
          )}

          <Stack sx={{ minWidth: 0, flex: 1, bgcolor: "background.paper" }}>
            <Box sx={{ px: 1.5, pt: 1 }}>
              <Stack
                direction="row"
                alignItems="center"
                spacing={1}
                sx={{ height: 38, borderBottom: "1px solid", borderColor: "divider" }}
              >
                <Search size={17} color="currentColor" />
                <InputBase
                  inputRef={searchRef}
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                  placeholder="Search models…"
                  fullWidth
                  inputProps={{ "aria-label": "Search models" }}
                  sx={{ fontSize: "0.875rem" }}
                />
                {loading && <CircularProgress size={15} />}
              </Stack>
            </Box>

            <Box sx={{ minHeight: 0, flex: 1, overflowY: "auto", p: 1 }}>
              {visibleModels.map(({ agent, option }) => {
                const selected = agent.agent.id === activeAgent?.agent.id
                  && option.id === model
                  && (!option.provider || matchesStoredRef(option.provider, providerRefValue));
                const agentName = agent.agent.config?.helix?.name || "Unnamed agent";
                return (
                  <Button
                    key={`${agent.agent.id}:${option.key}`}
                    fullWidth
                    onClick={() => {
                      onSelectAgentModel(
                        agent.agent.id,
                        option.provider ? providerRef(option.provider) : "",
                        option.id,
                      );
                      setAnchor(null);
                    }}
                    sx={{
                      minHeight: 52,
                      px: 1.25,
                      py: 0.75,
                      mb: 0.25,
                      borderRadius: 1,
                      justifyContent: "flex-start",
                      textAlign: "left",
                      textTransform: "none",
                      bgcolor: selected ? "action.selected" : "transparent",
                      "&:hover": { bgcolor: "action.hover" },
                    }}
                  >
                    <Box sx={{ minWidth: 0, flex: 1 }}>
                      <Typography variant="body2" color="text.primary" noWrap>
                        {option.label}
                      </Typography>
                      <Stack direction="row" alignItems="center" spacing={0.75} sx={{ mt: 0.5 }}>
                        {option.provider
                          ? <ProviderIcon provider={option.provider} size={13} />
                          : <AgentHarness
                              runtime={agent.assistant?.code_agent_runtime || "zed_agent"}
                              variant="short"
                              size={13}
                              showTooltip={false}
                            />}
                        <Typography variant="caption" color="text.secondary" noWrap>
                          {agentName}{option.providerLabel && option.providerLabel !== agentName ? ` · ${option.providerLabel}` : ""}
                        </Typography>
                      </Stack>
                    </Box>
                  </Button>
                );
              })}
              {!loading && visibleModels.length === 0 && (
                <Typography variant="body2" color="text.secondary" sx={{ px: 1, py: 2 }}>
                  No models found
                </Typography>
              )}
            </Box>
          </Stack>
        </Stack>
      </Popover>
    </>
  );
};

function getAssistant(agent: IApp): IAssistantConfig | undefined {
  return agent.config?.helix?.assistants?.find(
    (assistant) => assistant.agent_type === AGENT_TYPE_ZED_EXTERNAL,
  ) || agent.config?.helix?.assistants?.[0];
}

export function buildPickerAgents(
  agents: IApp[],
  providers: TypesProviderEndpoint[],
  harnesses: TypesOrgCodeAgentHarnessStatus[] = [],
  enforceOrgPolicy = false,
): PickerAgent[] {
  return agents.map((agent) => {
    const assistant = getAssistant(agent);
    const runtime = assistant?.code_agent_runtime;
    const usesSubscription = assistant?.code_agent_credential_type === "subscription";
    const harness = findHarnessStatus(harnesses, runtime);
    let models: PickerModel[];

    const subscriptionAllowed = !enforceOrgPolicy
      || (!!harness?.enabled
        && harness.subscription_enabled === true
        && !!harness.viewer_has_subscription);
    if (usesSubscription) {
      const subscriptionModels = runtime === "claude_code"
        ? CLAUDE_SUBSCRIPTION_MODELS
        : runtime === "codex_cli"
          ? CODEX_SUBSCRIPTION_MODELS
          : [];
      models = subscriptionAllowed ? subscriptionModels.map((option) => ({
        ...option,
        key: option.id,
        providerLabel: runtime === "claude_code" ? "Claude Code" : "Codex",
      })) : [];
    } else {
      const allowedProviders = enforceOrgPolicy
        ? providersForCodeAgentHarness(providers, harness, runtime)
        : providersForCodeAgentRuntime(providers, runtime).filter(providerEndpointIsConnected);
      models = allowedProviders.flatMap((provider) => (provider.available_models || [])
        .filter((availableModel) => availableModel.enabled
          && (!availableModel.type || availableModel.type === "chat" || availableModel.type === "text"))
        .map((availableModel) => ({
          key: `${providerRef(provider)}:${availableModel.id}`,
          id: availableModel.id || "",
          label: availableModel.id || "Unnamed model",
          provider,
          providerLabel: provider.name || "Provider",
        }))
        .filter((availableModel) => availableModel.id));
    }

    return { agent, assistant, models };
  });
}

const ProviderAwareSpecTaskModelPicker: FC<SpecTaskModelPickerProps> = (props) => {
  const router = useRouter();
  const orgName = router.params.org_id;
  const needsProviders = props.agents.some((agent) =>
    getAssistant(agent)?.code_agent_credential_type !== "subscription");
  const { data: org, isLoading: loadingOrg } = useGetOrgByName(orgName, orgName !== undefined);
  const { data: providers = [], isLoading } = useListProviders({
    loadModels: true,
    orgId: org?.id,
    enabled: !loadingOrg && needsProviders,
  });
  const { data: harnesses = [], isLoading: loadingHarnesses } = useOrgCodeAgentHarnesses(
    org?.id,
    { enabled: !loadingOrg },
  );
  const pickerAgents = useMemo(
    () => buildPickerAgents(props.agents, providers, harnesses, !!orgName),
    [props.agents, providers, harnesses, orgName],
  );

  return (
    <SpecTaskModelPickerView
      {...props}
      agents={pickerAgents}
      loading={(needsProviders && isLoading) || loadingOrg || loadingHarnesses}
    />
  );
};

const SpecTaskModelPicker: FC<SpecTaskModelPickerProps> = (props) => {
  return <ProviderAwareSpecTaskModelPicker {...props} />;
};

export default SpecTaskModelPicker;
