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

const ProjectTaskDefaults: FC<ProjectTaskDefaultsProps> = ({
  project,
  disabled = false,
  onUpdate,
}) => {
  return (
    <CodeAgentExecutionControls
      value={project.code_agent_config}
      sandboxResourceOverrides={project.default_sandbox_resource_overrides}
      sandboxRuntime={project.default_sandbox_runtime}
      onChange={(nextConfig) => onUpdate({ code_agent_config: nextConfig })}
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
