import React, { FC } from 'react'
import { useRoute } from 'react-router5'
import {
  Box,
  IconButton,
  Tooltip,
  CircularProgress,
  Stack,
} from '@mui/material'
import {
  ViewModule as TiledIcon,
} from '@mui/icons-material'

import Page from '../components/system/Page'
import SpecTaskDetailContent from '../components/tasks/SpecTaskDetailContent'
import { useSpecTask } from '../services/specTaskService'
import { useGetProject } from '../services'
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
  const account = useAccount()

  const projectId = route.params.id as string
  const taskId = route.params.taskId as string

  const { data: task, isLoading: taskLoading } = useSpecTask(taskId, {
    enabled: !!taskId,
  })

  const { data: project, isLoading: projectLoading } = useGetProject(projectId, !!projectId)

  const handleBack = () => {
    account.orgNavigate('project-specs', { id: projectId })
  }

  const handleOpenInWorkspace = () => {
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
    <Page
      breadcrumbs={[
        {
          title: 'Projects',
          routeName: 'projects',
        },
        {
          title: project?.name || 'Project',
          routeName: 'project-specs',
          params: { id: projectId },
        },
        {
          title: task?.name || 'Task',
        },
      ]}
      orgBreadcrumbs={true}
      showDrawerButton={true}
      topbarContent={
        <Stack direction="row" spacing={2} sx={{ justifyContent: 'flex-end', width: '100%', alignItems: 'center' }}>
          <Tooltip title="Open in Split Screen">
            <IconButton onClick={handleOpenInWorkspace} size="small">
              <TiledIcon />
            </IconButton>
          </Tooltip>
        </Stack>
      }
    >
      <Box sx={{ flex: 1, overflow: 'auto', height: 'calc(100vh - 120px)', px: 3 }}>
        <SpecTaskDetailContent
          taskId={taskId}
          onClose={handleBack}
        />
      </Box>
    </Page>
  )
}

export default SpecTaskDetailPage
