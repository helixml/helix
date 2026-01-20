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
  ViewModule as WorkspaceIcon,
} from '@mui/icons-material'

import Page from '../components/system/Page'
import ExternalAgentDesktopViewer from '../components/external-agent/ExternalAgentDesktopViewer'
import { useGetProject } from '../services'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'

/**
 * TeamDesktopPage - Standalone page for the Team Desktop (exploratory session)
 *
 * This page displays the remote desktop stream with an integrated chat panel,
 * allowing users to interact with the AI agent in a shared desktop environment.
 *
 * Route: /projects/:id/desktop/:sessionId
 */
const TeamDesktopPage: FC = () => {
  const { route } = useRoute()
  const account = useAccount()
  const api = useApi()

  const projectId = route.params.id as string
  const sessionId = route.params.sessionId as string

  // Fetch project data for breadcrumb
  const { data: project, isLoading: projectLoading } = useGetProject(projectId, !!projectId)

  const handleBack = () => {
    // Navigate back to the project's spec tasks page
    account.orgNavigate('project-specs', { id: projectId })
  }

  const handleOpenInWorkspace = () => {
    // Navigate to project specs page with workspace view and open this desktop
    account.orgNavigate('project-specs', { id: projectId, tab: 'workspace', openDesktop: sessionId })
  }

  if (projectLoading) {
    return (
      <Page>
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '50vh' }}>
          <CircularProgress />
        </Box>
      </Page>
    )
  }

  if (!sessionId) {
    return (
      <Page>
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '50vh' }}>
          <Typography color="error">No session ID provided</Typography>
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
              Team Desktop
            </Typography>
          </Breadcrumbs>
        </Box>

        <Tooltip title="Open in Workspace">
          <IconButton onClick={handleOpenInWorkspace} size="small">
            <WorkspaceIcon />
          </IconButton>
        </Tooltip>
      </Box>

      {/* Desktop viewer content */}
      <Box sx={{ flex: 1, overflow: 'hidden', height: 'calc(100vh - 120px)' }}>
        <ExternalAgentDesktopViewer
          sessionId={sessionId}
          sandboxId={sessionId}
          mode="stream"
          showSessionPanel={true}
          defaultPanelOpen={true}
          projectId={projectId}
          apiClient={api.getApiClient()}
        />
      </Box>
    </Page>
  )
}

export default TeamDesktopPage
