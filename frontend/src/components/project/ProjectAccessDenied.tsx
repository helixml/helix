import React, { FC } from "react";
import {
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
      <Stack
        spacing={2.5}
        alignItems="center"
        sx={{ width: "100%", maxWidth: 520, textAlign: "center" }}
      >
        <Box
          sx={{
            width: 56,
            height: 56,
            borderRadius: "50%",
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            bgcolor: "action.hover",
            color: "text.secondary",
          }}
        >
          <LockOutlinedIcon />
        </Box>
        <Stack spacing={1}>
          <Typography variant="h5" sx={{ fontWeight: 700 }}>
            You don't have access to this project
          </Typography>
          <Typography variant="body1" color="text.secondary">
            This project is private. Ask someone with access to invite you, then
            refresh this page.
          </Typography>
        </Stack>
        {projectId && (
          <Typography
            variant="body2"
            color="text.secondary"
            sx={{
              px: 1.5,
              py: 0.75,
              border: "1px solid",
              borderColor: "divider",
              borderRadius: 1,
              bgcolor: "background.paper",
            }}
          >
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
              color="secondary"
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
