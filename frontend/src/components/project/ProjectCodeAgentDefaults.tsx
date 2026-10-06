import React, { FC } from "react";
import { Box, Stack, Tooltip, Typography } from "@mui/material";
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
    <Stack spacing={1.5} sx={{ width: "100%" }}>
      <Box>
        <Tooltip title="Planning benefits from an intelligent model with high reasoning effort.">
          <Typography
            component="span"
            variant="caption"
            color="text.secondary"
            tabIndex={0}
            sx={{ display: "inline-block", cursor: "help" }}
          >
            Planning
          </Typography>
        </Tooltip>
        <CodeAgentExecutionControls
          value={project.planning_code_agent_config || project.code_agent_config}
          onChange={(nextConfig) =>
            onUpdate({ planning_code_agent_config: nextConfig })
          }
          disabled={disabled}
          spreadAgentControls
        />
      </Box>
      <Box>
        <Tooltip title="Implementation can use a faster, lower-cost model because it follows the approved plan.">
          <Typography
            component="span"
            variant="caption"
            color="text.secondary"
            tabIndex={0}
            sx={{ display: "inline-block", cursor: "help" }}
          >
            Implementation
          </Typography>
        </Tooltip>
        <CodeAgentExecutionControls
          value={project.code_agent_config}
          onChange={(nextConfig) => onUpdate({ code_agent_config: nextConfig })}
          disabled={disabled}
          spreadAgentControls
        />
      </Box>
    </Stack>
  );
};

export default ProjectCodeAgentDefaults;
