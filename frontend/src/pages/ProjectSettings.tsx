import React, { FC, useState, useEffect } from 'react'
import {
  Container,
  Box,
  Paper,
  Typography,
  TextField,
  Button,
  Alert,
  CircularProgress,
  Divider,
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Chip,
} from '@mui/material'
import SaveIcon from '@mui/icons-material/Save'
import StarIcon from '@mui/icons-material/Star'
import StarBorderIcon from '@mui/icons-material/StarBorder'
import CodeIcon from '@mui/icons-material/Code'

import Page from '../components/system/Page'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  useGetProject,
  useUpdateProject,
  useGetProjectRepositories,
  useSetProjectPrimaryRepository,
} from '../services'

const ProjectSettings: FC = () => {
  const { params } = useRouter()
  const snackbar = useSnackbar()
  const projectId = params.id as string

  const { data: project, isLoading, error } = useGetProject(projectId)
  const { data: repositories = [] } = useGetProjectRepositories(projectId)
  const updateProjectMutation = useUpdateProject(projectId)
  const setPrimaryRepoMutation = useSetProjectPrimaryRepository(projectId)

  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [startupScript, setStartupScript] = useState('')

  useEffect(() => {
    if (project) {
      setName(project.name || '')
      setDescription(project.description || '')
      setStartupScript(project.startup_script || '')
    }
  }, [project])

  const handleSave = async () => {
    try {
      await updateProjectMutation.mutateAsync({
        name,
        description,
        startup_script: startupScript,
      })
      snackbar.success('Project settings saved')
    } catch (err) {
      snackbar.error('Failed to save project settings')
    }
  }

  const handleSetPrimaryRepo = async (repoId: string) => {
    try {
      await setPrimaryRepoMutation.mutateAsync(repoId)
      snackbar.success('Primary repository updated')
    } catch (err) {
      snackbar.error('Failed to update primary repository')
    }
  }

  if (isLoading) {
    return (
      <Page breadcrumbTitle="Project Settings" orgBreadcrumbs={true}>
        <Container maxWidth="md">
          <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: '400px' }}>
            <CircularProgress />
          </Box>
        </Container>
      </Page>
    )
  }

  if (error || !project) {
    return (
      <Page breadcrumbTitle="Project Settings" orgBreadcrumbs={true}>
        <Container maxWidth="md">
          <Alert severity="error" sx={{ mt: 4 }}>
            {error instanceof Error ? error.message : 'Project not found'}
          </Alert>
        </Container>
      </Page>
    )
  }

  return (
    <Page
      breadcrumbTitle="Project Settings"
      orgBreadcrumbs={true}
      topbarContent={(
        <Button
          variant="contained"
          color="primary"
          startIcon={<SaveIcon />}
          onClick={handleSave}
          disabled={updateProjectMutation.isPending}
        >
          {updateProjectMutation.isPending ? 'Saving...' : 'Save Changes'}
        </Button>
      )}
    >
      <Container maxWidth="md">
        <Box sx={{ mt: 4, display: 'flex', flexDirection: 'column', gap: 3 }}>
          {/* Basic Information */}
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Basic Information
            </Typography>
            <Divider sx={{ mb: 3 }} />
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
              <TextField
                label="Project Name"
                fullWidth
                value={name}
                onChange={(e) => setName(e.target.value)}
                required
              />
              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                value={description}
                onChange={(e) => setDescription(e.target.value)}
              />
            </Box>
          </Paper>

          {/* Startup Script */}
          <Paper sx={{ p: 3 }}>
            <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
              <CodeIcon sx={{ mr: 1 }} />
              <Typography variant="h6">
                Startup Script
              </Typography>
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              This script runs when an agent starts working on this project. Use it to install dependencies, start dev servers, etc.
            </Typography>
            <Divider sx={{ mb: 3 }} />
            <TextField
              fullWidth
              multiline
              rows={10}
              value={startupScript}
              onChange={(e) => setStartupScript(e.target.value)}
              placeholder={`#!/bin/bash
# Install dependencies
npm install

# Start dev server
npm run dev &

# Run migrations
npm run db:migrate`}
              sx={{
                fontFamily: 'monospace',
                '& textarea': {
                  fontFamily: 'monospace',
                },
              }}
            />
          </Paper>

          {/* Repositories */}
          <Paper sx={{ p: 3 }}>
            <Typography variant="h6" gutterBottom>
              Repositories
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Repositories attached to this project. The primary repository is opened by default when agents start.
            </Typography>
            <Divider sx={{ mb: 2 }} />
            {repositories.length === 0 ? (
              <Typography variant="body2" color="text.secondary" sx={{ textAlign: 'center', py: 4 }}>
                No repositories attached to this project yet
              </Typography>
            ) : (
              <List>
                {repositories.map((repo) => (
                  <ListItem key={repo.id} divider>
                    <ListItemText
                      primary={repo.name}
                      secondary={repo.clone_url}
                    />
                    <ListItemSecondaryAction>
                      {project.default_repo_id === repo.id ? (
                        <Chip
                          icon={<StarIcon />}
                          label="Primary"
                          color="primary"
                          size="small"
                        />
                      ) : (
                        <IconButton
                          edge="end"
                          onClick={() => handleSetPrimaryRepo(repo.id)}
                          disabled={setPrimaryRepoMutation.isPending}
                        >
                          <StarBorderIcon />
                        </IconButton>
                      )}
                    </ListItemSecondaryAction>
                  </ListItem>
                ))}
              </List>
            )}
          </Paper>
        </Box>
      </Container>
    </Page>
  )
}

export default ProjectSettings
