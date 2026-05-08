import React, { FC, useCallback, useEffect, useState } from "react";
import { useRoute } from "react-router5";
import {
  Box,
  IconButton,
  Tooltip,
  CircularProgress,
  Stack,
} from "@mui/material";
import { ViewModule as TiledIcon, Add as AddIcon } from "@mui/icons-material";
import NewSpecTaskForm from "../components/tasks/NewSpecTaskForm";
import { TypesSpecTask } from "../api/api";

import Page from "../components/system/Page";
import SpecTaskDetailContent from "../components/tasks/SpecTaskDetailContent";
import { useSpecTask } from "../services/specTaskService";
import { useGetProject } from "../services";
import useAccount from "../hooks/useAccount";
import { cacheTaskName } from "../lib/navHistory";

/**
 * SpecTaskDetailPage - Standalone page for viewing spec task details
 *
 * This page wraps SpecTaskDetailContent (the same component used in TabsView)
 * providing proper browser navigation (back button, bookmarkable URLs).
 *
 * Route: /projects/:id/tasks/:taskId
 */
const SpecTaskDetailPage: FC = () => {
  const { route } = useRoute();
  const account = useAccount();

  const projectId = route.params.id as string;
  const taskId = route.params.taskId as string;

  const [createDialogOpen, setCreateDialogOpen] = useState(false);

  const { data: task, isLoading: taskLoading } = useSpecTask(taskId, {
    enabled: !!taskId,
  });

  useEffect(() => {
    if (taskId && task?.name) cacheTaskName(taskId, task.name)
  }, [taskId, task?.name])

  const { data: project, isLoading: projectLoading } = useGetProject(
    projectId,
    !!projectId,
  );

  const handleBack = () => {
    account.orgNavigate("project-specs", { id: projectId });
  };

  const handleOpenInWorkspace = () => {
    account.orgNavigate("project-specs", {
      id: projectId,
      tab: "workspace",
      openTask: taskId,
    });
  };

  const handleTaskCreated = useCallback((_task: TypesSpecTask) => {
    setCreateDialogOpen(false);
  }, []);

  // Keyboard shortcut: Enter to toggle new task panel, Escape to close
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        if (createDialogOpen) setCreateDialogOpen(false);
        return;
      }
      if (
        e.key === "Enter" &&
        !e.ctrlKey &&
        !e.metaKey &&
        !e.altKey &&
        !e.shiftKey
      ) {
        const target = e.target as HTMLElement;
        if (
          target.tagName === "INPUT" ||
          target.tagName === "TEXTAREA" ||
          target.isContentEditable ||
          target.hasAttribute("tabindex")
        ) {
          return;
        }
        e.preventDefault();
        setCreateDialogOpen((prev) => !prev);
      }
    };
    window.addEventListener("keydown", handleKeyDown);
    return () => window.removeEventListener("keydown", handleKeyDown);
  }, [createDialogOpen]);

  if (taskLoading || projectLoading) {
    return (
      <Page>
        <Box
          sx={{
            display: "flex",
            justifyContent: "center",
            alignItems: "center",
            minHeight: "50vh",
          }}
        >
          <CircularProgress />
        </Box>
      </Page>
    );
  }

  return (
    <Page
      breadcrumbs={[
        {
          title: "Projects",
          routeName: "projects",
        },
        {
          title: project?.name || "Project",
          routeName: "project-specs",
          params: { id: projectId },
        },
        {
          title: task?.name || "Task",
          tooltip: task?.description || task?.name,
        },
      ]}
      orgBreadcrumbs={true}
      showDrawerButton={true}
      topbarContent={
        <Stack
          direction="row"
          spacing={2}
          sx={{
            justifyContent: "flex-end",
            width: "100%",
            alignItems: "center",
          }}
        >
          <Tooltip title="Create New Task">
            <IconButton onClick={() => setCreateDialogOpen((prev) => !prev)} size="small">
              <AddIcon />
            </IconButton>
          </Tooltip>
          <Tooltip title="Open in Split Screen">
            <IconButton onClick={handleOpenInWorkspace} size="small">
              <TiledIcon />
            </IconButton>
          </Tooltip>
        </Stack>
      }
    >
      <Box sx={{ display: "flex", flex: 1, overflow: "hidden", height: "calc(100vh - 120px)" }}>
        <Box
          sx={{
            flex: 1,
            overflow: "auto",
            px: { xs: 0, sm: 3 },
          }}
        >
          <SpecTaskDetailContent taskId={taskId} onClose={handleBack} />
        </Box>

        {/* Slide-in new spec task panel */}
        <Box
          sx={{
            width: createDialogOpen
              ? { xs: "100%", sm: "450px", md: "500px" }
              : 0,
            flexShrink: 0,
            overflow: "hidden",
            transition: "width 0.3s ease-in-out",
            borderLeft: createDialogOpen ? 1 : 0,
            borderColor: "divider",
            display: "flex",
            flexDirection: "column",
            backgroundColor: "background.paper",
            position: { xs: "fixed", md: "relative" },
            top: { xs: 0, md: "auto" },
            left: { xs: 0, md: "auto" },
            right: { xs: 0, md: "auto" },
            bottom: { xs: 0, md: "auto" },
            zIndex: { xs: 1200, md: "auto" },
          }}
        >
          {createDialogOpen && projectId && (
            <NewSpecTaskForm
              projectId={projectId}
              onTaskCreated={handleTaskCreated}
              onClose={() => setCreateDialogOpen(false)}
              showHeader={true}
              embedded={false}
            />
          )}
        </Box>
      </Box>
    </Page>
  );
};

export default SpecTaskDetailPage;
