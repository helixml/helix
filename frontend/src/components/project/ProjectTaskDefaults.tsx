import React, { FC } from "react";
import {
  TypesAgentExecutionConfig,
  TypesCodeAgentExecutionConfig,
  TypesProject,
  TypesProjectUpdateRequest,
} from "../../api/api";
import { IApp } from "../../types";
import {
  codeAgentExecutionConfigFromApp,
  findCodeAgentAppForConfig,
} from "../../utils/codeAgentExecutionConfig";
import SpecTaskExecutionControls from "../tasks/SpecTaskExecutionControls";

interface ProjectTaskDefaultsProps {
  agents: IApp[];
  project: TypesProject;
  disabled?: boolean;
  onUpdate: (request: TypesProjectUpdateRequest) => Promise<unknown>;
}

function findConfigHarness(
  agents: IApp[],
  config?: TypesCodeAgentExecutionConfig,
): IApp | undefined {
  const exact = findCodeAgentAppForConfig(agents, config);
  if (exact || !config) return exact;

  return agents.find((agent) => {
    const candidate = codeAgentExecutionConfigFromApp(agent);
    return candidate?.runtime === config.runtime
      && candidate.credential_type === config.credential_type;
  });
}

const ProjectTaskDefaults: FC<ProjectTaskDefaultsProps> = ({
  agents,
  project,
  disabled = false,
  onUpdate,
}) => {
  const config = project.code_agent_config;
  const harness = findConfigHarness(agents, config);
  const executionConfig: TypesAgentExecutionConfig | undefined = config
    ? {
        agent_available: true,
        agent_id: harness?.id,
        agent_name: harness?.config?.helix?.name,
        runtime: config.runtime,
        credential_type: config.credential_type,
        provider_ref: config.provider_ref,
        model: config.model,
        reasoning_effort: config.reasoning_effort,
        service_tier: config.service_tier,
        code_agent_config: config,
        code_agent_overrides: {
          provider_ref: config.provider_ref,
          model: config.model,
          reasoning_effort: config.reasoning_effort,
          service_tier: config.service_tier,
        },
      }
    : undefined;

  return (
    <SpecTaskExecutionControls
      agents={agents}
      selectedAgentId={harness?.id || ""}
      codeAgentOverrides={executionConfig?.code_agent_overrides}
      currentExecutionConfig={executionConfig}
      sandboxResourceOverrides={project.default_sandbox_resource_overrides}
      sandboxRuntime={project.default_sandbox_runtime}
      onAgentModelChange={async (_agentId, _overrides, nextConfig) => {
        if (!nextConfig) {
          throw new Error("Select a harness and model");
        }
        await onUpdate({ code_agent_config: nextConfig });
      }}
      onSandboxResourceOverridesChange={(resources) =>
        onUpdate({ default_sandbox_resource_overrides: resources })
      }
      onSandboxRuntimeChange={(runtime) =>
        onUpdate({ default_sandbox_runtime: runtime })
      }
      disabled={disabled}
      grouped
    />
  );
};

export default ProjectTaskDefaults;
