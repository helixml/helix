import React, { FC } from 'react'
import { useRoute } from 'react-router5'
import {
  Box,
  IconButton,
  Typography,
  Tooltip,
  Breadcrumbs,
  Link,
  CircularProgress,
} from '@mui/material'
import {
  ArrowBack as BackIcon,
  ViewModule as TiledIcon,
} from '@mui/icons-material'

import Page from '../components/system/Page'
import SpecTaskDetailContent from '../components/tasks/SpecTaskDetailContent'
import { useSpecTask } from '../services/specTaskService'
import { useGetProject } from '../services'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'

/**
 * SpecTaskDetailPage - Standalone page for viewing spec task details
 *
 * This page wraps SpecTaskDetailContent (the same component used in TabsView)
 * providing proper browser navigation (back button, bookmarkable URLs).
 *
 * Route: /projects/:id/tasks/:taskId
 */
const SpecTaskDetailPage: FC = () => {
  const { route } = useRoute()
  const router = useRouter()
  const account = useAccount()

  const projectId = route.params.id as string
  const taskId = route.params.taskId as string

  // Fetch task data for breadcrumb
  const { data: task, isLoading: taskLoading } = useSpecTask(taskId, {
    enabled: !!taskId,
  })

  // Fetch project data for breadcrumb
  const { data: project, isLoading: projectLoading } = useGetProject(projectId, !!projectId)

  const handleBack = () => {
    // Navigate back to the project's spec tasks page
    account.orgNavigate('project-specs', { id: projectId })
  }

  const handleOpenInWorkspace = () => {
    // Navigate to project specs page with split screen view and open this task
    account.orgNavigate('project-specs', { id: projectId, tab: 'workspace', openTask: taskId })
  }

  if (taskLoading || projectLoading) {
    return (
      <Page>
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '50vh' }}>
          <CircularProgress />
        </Box>
      </Page>
    )
  }

  return (
    <Page>
      {/* Header with navigation */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 2,
          py: 1,
          borderBottom: 1,
          borderColor: 'divider',
          backgroundColor: 'background.paper',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
          <Tooltip title="Back to tasks">
            <IconButton onClick={handleBack} size="small">
              <BackIcon />
            </IconButton>
          </Tooltip>

          <Breadcrumbs separator="›" sx={{ fontSize: '0.875rem' }}>
            <Link
              component="button"
              underline="hover"
              color="inherit"
              onClick={() => account.orgNavigate('projects')}
              sx={{ cursor: 'pointer' }}
            >
              Projects
            </Link>
            <Link
              component="button"
              underline="hover"
              color="inherit"
              onClick={() => account.orgNavigate('project-specs', { id: projectId })}
              sx={{ cursor: 'pointer' }}
            >
              {project?.name || 'Project'}
            </Link>
            <Typography color="text.primary" sx={{ fontSize: '0.875rem' }}>
              {task?.name || 'Task'}
            </Typography>
          </Breadcrumbs>
        </Box>

        <Tooltip title="Split Screen">
          <IconButton onClick={handleOpenInWorkspace} size="small">
            <TiledIcon />
          </IconButton>
        </Tooltip>
      </Box>

      {/* Task detail content - reusing the same component as TabsView */}
      <Box sx={{ flex: 1, overflow: 'auto', height: 'calc(100vh - 120px)' }}>
        <SpecTaskDetailContent
          taskId={taskId}
          onClose={handleBack}
        />
      </Box>
    </Page>
  )
}

export default SpecTaskDetailPage
