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
import DesignReviewContent from '../components/spec-tasks/DesignReviewContent'
import { useSpecTask } from '../services/specTaskService'
import { useDesignReview } from '../services/designReviewService'
import { useGetProject } from '../services'
import useAccount from '../hooks/useAccount'

/**
 * SpecTaskReviewPage - Standalone page for spec review
 *
 * This page displays the spec review documents (requirements, technical design,
 * implementation plan) with inline commenting functionality.
 *
 * Route: /projects/:id/tasks/:taskId/review/:reviewId
 */
const SpecTaskReviewPage: FC = () => {
  const { route } = useRoute()
  const account = useAccount()

  const projectId = route.params.id as string
  const taskId = route.params.taskId as string
  const reviewId = route.params.reviewId as string

  // Fetch task data for breadcrumb
  const { data: task, isLoading: taskLoading } = useSpecTask(taskId, {
    enabled: !!taskId,
  })

  // Fetch project data for breadcrumb
  const { data: project, isLoading: projectLoading } = useGetProject(projectId, !!projectId)

  // Fetch review data
  const { isLoading: reviewLoading } = useDesignReview(taskId, reviewId, {
    enabled: !!taskId && !!reviewId,
  })

  const handleBack = () => {
    // Navigate back to the task detail page
    account.orgNavigate('project-task-detail', { id: projectId, taskId })
  }

  const handleOpenInWorkspace = () => {
    // Navigate to project specs page with split screen view and open this task
    account.orgNavigate('project-specs', { id: projectId, tab: 'workspace', openTask: taskId })
  }

  if (taskLoading || projectLoading || reviewLoading) {
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
          routeName: 'project-task-detail',
          params: { id: projectId, taskId },
        },
        {
          title: 'Spec Review',
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
      {/* Review content - embedded mode without floating window */}
      <Box sx={{ flex: 1, overflow: 'hidden', minHeight: 0 }}>
        <DesignReviewContent
          specTaskId={taskId}
          reviewId={reviewId}
          onClose={handleBack}
          hideTitle={true}
        />
      </Box>
    </Page>
  )
}

export default SpecTaskReviewPage
