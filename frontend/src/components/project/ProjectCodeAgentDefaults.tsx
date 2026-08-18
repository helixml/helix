import React, { FC } from "react";
import {
  TypesProject,
  TypesProjectUpdateRequest,
} from "../../api/api";
import CodeAgentExecutionControls from "../agent/CodeAgentExecutionControls";

interface ProjectCodeAgentDefaultsProps {
  project: TypesProject;
  disabled?: boolean;
  onUpdate: (request: TypesProjectUpdateRequest) => Promise<unknown>;
}

/**
 * The coding agent and model new tasks in this project start on.
 *
 * A task created without an explicit configuration inherits
 * `project.code_agent_config`, so this is what decides which harness the
 * project's work actually runs on — including tasks filed by an org Bot. It was
 * previously only settable through the agent that provisioned the project,
 * which left no way to see or change it from the project itself.
 *
 * Compute (sandbox size and environment) is configured separately on the
 * Sandbox tab; passing no sandbox handlers here renders the agent row alone.
 * Which harnesses are available at all remains an organization-level decision
 * on the Providers page.
 */
const ProjectCodeAgentDefaults: FC<ProjectCodeAgentDefaultsProps> = ({
  project,
  disabled = false,
  onUpdate,
}) => {
  return (
    <CodeAgentExecutionControls
      value={project.code_agent_config}
      onChange={(nextConfig) => onUpdate({ code_agent_config: nextConfig })}
      disabled={disabled}
      grouped
    />
  );
};

export default ProjectCodeAgentDefaults;
