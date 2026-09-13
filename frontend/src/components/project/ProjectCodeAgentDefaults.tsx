import React, { FC } from "react";
import { Box, Stack, Typography } from "@mui/material";
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

const ProjectCodeAgentDefaults: FC<ProjectCodeAgentDefaultsProps> = ({
  project,
  disabled = false,
  onUpdate,
}) => {
  return (
    <Stack spacing={1.5}>
      <Box>
        <Typography variant="caption" color="text.secondary">
          Planning
        </Typography>
        <CodeAgentExecutionControls
          value={project.planning_code_agent_config || project.code_agent_config}
          onChange={(nextConfig) =>
            onUpdate({ planning_code_agent_config: nextConfig })
          }
          disabled={disabled}
        />
      </Box>
      <Box>
        <Typography variant="caption" color="text.secondary">
          Implementation
        </Typography>
        <CodeAgentExecutionControls
          value={project.code_agent_config}
          onChange={(nextConfig) => onUpdate({ code_agent_config: nextConfig })}
          disabled={disabled}
        />
      </Box>
    </Stack>
  );
};

export default ProjectCodeAgentDefaults;
