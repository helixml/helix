import React, { FC } from "react";
import {
  Alert,
  Box,
  Button,
  Stack,
  Typography,
} from "@mui/material";
import ArrowBackIcon from "@mui/icons-material/ArrowBack";
import LockOutlinedIcon from "@mui/icons-material/LockOutlined";

interface ProjectAccessDeniedProps {
  projectId?: string;
  onBackToProjects?: () => void;
}

const ProjectAccessDenied: FC<ProjectAccessDeniedProps> = ({
  projectId,
  onBackToProjects,
}) => {
  return (
    <Box
      sx={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        minHeight: "100%",
        px: 2,
        py: 6,
      }}
    >
      <Stack spacing={2.5} sx={{ width: "100%", maxWidth: 560 }}>
        <Stack direction="row" spacing={1.5} alignItems="center">
          <LockOutlinedIcon color="warning" />
          <Typography variant="h5" sx={{ fontWeight: 700 }}>
            You don't have access to this project
          </Typography>
        </Stack>
        <Alert severity="warning">
          Ask the project owner to add you to this project, or contact an
          organization owner if you think this is a mistake.
        </Alert>
        {projectId && (
          <Typography variant="body2" color="text.secondary">
            Project ID:{" "}
            <Box component="span" sx={{ fontFamily: "monospace" }}>
              {projectId}
            </Box>
          </Typography>
        )}
        {onBackToProjects && (
          <Box>
            <Button
              variant="contained"
              startIcon={<ArrowBackIcon />}
              onClick={onBackToProjects}
            >
              Back to projects
            </Button>
          </Box>
        )}
      </Stack>
    </Box>
  );
};

export default ProjectAccessDenied;
