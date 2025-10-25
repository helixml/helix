import React, { FC, useState } from 'react'
import Container from '@mui/material/Container'
import {
  Box,
  Typography,
  Card,
  CardContent,
  CircularProgress,
  Alert,
  Button,
  Chip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  TextField,
  Stack,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material'
import { GitBranch, Plus, ExternalLink } from 'lucide-react'

import Page from '../components/system/Page'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'

import useAccount from '../hooks/useAccount'
import { useGitRepositories, getSampleTypeIcon } from '../services/gitRepositoryService'
import { useSampleTypes } from '../hooks/useSampleTypes'
import type { ServicesGitRepository, ServerSampleType } from '../api/api'

const GitRepos: FC = () => {
  const account = useAccount()
  const { data: repositories, isLoading, error } = useGitRepositories(account.user?.id)
  const { data: sampleTypes, loading: sampleTypesLoading, createSampleRepository } = useSampleTypes()

  // Dialog states
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [demoRepoDialogOpen, setDemoRepoDialogOpen] = useState(false)
  const [selectedSampleType, setSelectedSampleType] = useState('')
  const [demoRepoName, setDemoRepoName] = useState('')
  const [repoName, setRepoName] = useState('')
  const [repoDescription, setRepoDescription] = useState('')
  const [creating, setCreating] = useState(false)

  const handleCreateDemoRepo = async () => {
    if (!selectedSampleType || !account.user?.id || !demoRepoName.trim()) return

    setCreating(true)
    try {
      await createSampleRepository({
        owner_id: account.user.id,
        sample_type: selectedSampleType,
        name: demoRepoName,
      })
      setDemoRepoDialogOpen(false)
      setSelectedSampleType('')
      setDemoRepoName('')
    } catch (error) {
      console.error('Failed to create demo repository:', error)
    } finally {
      setCreating(false)
    }
  }

  const handleCreateCustomRepo = () => {
    // TODO: Implement custom repository creation
    console.log('Create custom repository:', { repoName, repoDescription })
    setCreateDialogOpen(false)
    setRepoName('')
    setRepoDescription('')
  }

  // Show logged out state if user is not authenticated
  if (!account.user) {
    return (
      <Page
        breadcrumbTitle="Git Repositories"
        orgBreadcrumbs={true}
      >
        <Container maxWidth="xl" sx={{ mb: 4 }}>
          <Card sx={{ textAlign: 'center', py: 8 }}>
            <CardContent>
              <GitBranch size={64} style={{ color: 'gray', marginBottom: 24 }} />
              <Typography variant="h4" component="h1" gutterBottom>
                Git Repositories
              </Typography>
              <Typography variant="h6" color="text.secondary" gutterBottom>
                Manage your git repositories for AI agent development
              </Typography>
              <Typography variant="body1" color="text.secondary" sx={{ mb: 4, maxWidth: 600, mx: 'auto' }}>
                Please log in to view and manage your git repositories.
              </Typography>
              <LaunchpadCTAButton />
            </CardContent>
          </Card>
        </Container>
      </Page>
    )
  }

  return (
    <Page
      breadcrumbTitle="Git Repositories"
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl" sx={{ mb: 4 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Typography variant="h4" component="h1">
            Git Repositories
          </Typography>
          <Box sx={{ display: 'flex', gap: 1 }}>
            <Button
              variant="outlined"
              startIcon={<GitBranch size={18} />}
              onClick={() => setDemoRepoDialogOpen(true)}
            >
              From Demo Repos
            </Button>
            <Button
              variant="contained"
              startIcon={<Plus size={18} />}
              onClick={() => setCreateDialogOpen(true)}
            >
              New Repository
            </Button>
          </Box>
        </Box>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error instanceof Error ? error.message : 'Failed to load repositories'}
          </Alert>
        )}

        {isLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
            <CircularProgress />
          </Box>
        ) : repositories && repositories.length > 0 ? (
          <Box sx={{ display: 'grid', gap: 2, gridTemplateColumns: 'repeat(auto-fill, minmax(350px, 1fr))' }}>
            {repositories.map((repo: ServicesGitRepository) => (
              <Card key={repo.id} sx={{
                '&:hover': {
                  boxShadow: 3,
                  transform: 'translateY(-2px)',
                  transition: 'all 0.2s'
                }
              }}>
                <CardContent>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <GitBranch size={20} />
                      <Typography variant="h6" component="h2">
                        {repo.name || repo.id}
                      </Typography>
                    </Box>
                    <Chip
                      label={repo.repo_type || 'unknown'}
                      size="small"
                      color="primary"
                      variant="outlined"
                    />
                  </Box>

                  {repo.description && (
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                      {repo.description}
                    </Typography>
                  )}

                  <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                    {repo.clone_url && (
                      <Button
                        size="small"
                        variant="outlined"
                        startIcon={<ExternalLink size={14} />}
                        onClick={() => {
                          // TODO: Show clone instructions
                          console.log('Clone:', repo.clone_url)
                        }}
                      >
                        Clone
                      </Button>
                    )}
                    <Button
                      size="small"
                      variant="text"
                      onClick={() => {
                        // TODO: Navigate to repository details
                        console.log('View repo:', repo.id)
                      }}
                    >
                      Details
                    </Button>
                  </Box>

                  {repo.created_at && (
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 2, display: 'block' }}>
                      Created {new Date(repo.created_at).toLocaleDateString()}
                    </Typography>
                  )}
                </CardContent>
              </Card>
            ))}
          </Box>
        ) : (
          <Card sx={{ textAlign: 'center', py: 8 }}>
            <CardContent>
              <GitBranch size={48} style={{ color: 'gray', marginBottom: 16 }} />
              <Typography variant="h6" gutterBottom>
                No repositories yet
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Create your first git repository to start collaborating with AI agents.
              </Typography>
              <Button
                variant="contained"
                startIcon={<Plus size={18} />}
                onClick={() => setCreateDialogOpen(true)}
              >
                Create Repository
              </Button>
            </CardContent>
          </Card>
        )}

        {/* Demo Repository Dialog */}
        <Dialog open={demoRepoDialogOpen} onClose={() => setDemoRepoDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Create from Demo Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              <Typography variant="body2" color="text.secondary">
                Choose a demo repository template to get started quickly with common project types.
              </Typography>

              <TextField
                label="Repository Name"
                fullWidth
                required
                value={demoRepoName}
                onChange={(e) => setDemoRepoName(e.target.value)}
                helperText="Enter a name for your repository"
              />

              <FormControl fullWidth>
                <InputLabel>Demo Template</InputLabel>
                <Select
                  value={selectedSampleType}
                  onChange={(e) => setSelectedSampleType(e.target.value)}
                  disabled={sampleTypesLoading}
                >
                  <MenuItem value="">
                    <em>Select a demo template</em>
                  </MenuItem>
                  {sampleTypes.map((type: ServerSampleType) => (
                    <MenuItem key={type.id} value={type.id}>
                      {getSampleTypeIcon(type.id || '')} {type.name}
                    </MenuItem>
                  ))}
                </Select>
              </FormControl>

              {selectedSampleType && (
                <Alert severity="info" sx={{ mt: 2 }}>
                  {sampleTypes.find((t: ServerSampleType) => t.id === selectedSampleType)?.description}
                </Alert>
              )}
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setDemoRepoDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={handleCreateDemoRepo}
              variant="contained"
              disabled={!selectedSampleType || !demoRepoName.trim() || creating}
            >
              {creating ? <CircularProgress size={20} /> : 'Create'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Custom Repository Dialog */}
        <Dialog open={createDialogOpen} onClose={() => setCreateDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Create New Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              <TextField
                label="Repository Name"
                fullWidth
                value={repoName}
                onChange={(e) => setRepoName(e.target.value)}
                helperText="Enter a name for your repository"
              />

              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                value={repoDescription}
                onChange={(e) => setRepoDescription(e.target.value)}
                helperText="Describe the purpose of this repository"
              />
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setCreateDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={handleCreateCustomRepo}
              variant="contained"
              disabled={!repoName.trim()}
            >
              Create
            </Button>
          </DialogActions>
        </Dialog>
      </Container>
    </Page>
  )
}

export default GitRepos
