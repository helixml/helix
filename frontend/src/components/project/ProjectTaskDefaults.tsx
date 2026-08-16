import React, { FC } from "react";
import {
  TypesProject,
  TypesProjectUpdateRequest,
} from "../../api/api";
import CodeAgentExecutionControls from "../agent/CodeAgentExecutionControls";

interface ProjectTaskDefaultsProps {
  project: TypesProject;
  disabled?: boolean;
  onUpdate: (request: TypesProjectUpdateRequest) => Promise<unknown>;
}

/**
 * Compute defaults for new tasks in this project.
 *
 * Which coding agent and model a task may use is an organization decision now,
 * configured on the Providers page, so this tab no longer offers a harness or
 * model. Leaving a project-level provider choice here would let a project pick
 * an agent the org has not enabled.
 */
const ProjectTaskDefaults: FC<ProjectTaskDefaultsProps> = ({
  project,
  disabled = false,
  onUpdate,
}) => {
  return (
    <CodeAgentExecutionControls
      computeOnly
      // Still passed so the control can render the project's stored value; the
      // code-agent portion is not rendered in computeOnly mode.
      value={project.code_agent_config}
      onChange={(nextConfig) => onUpdate({ code_agent_config: nextConfig })}
      sandboxResourceOverrides={project.default_sandbox_resource_overrides}
      sandboxRuntime={project.default_sandbox_runtime}
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
