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
  FormControlLabel,
  Switch,
  Tooltip,
} from '@mui/material'
import { GitBranch, Plus, ExternalLink, Brain } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import Page from '../components/system/Page'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import { useGitRepositories, getSampleTypeIcon } from '../services/gitRepositoryService'
import { useSampleTypes } from '../hooks/useSampleTypes'
import type { ServicesGitRepository, ServerSampleType } from '../api/api'

const GitRepos: FC = () => {
  const account = useAccount()
  const queryClient = useQueryClient()
  const api = useApi()

  // Get current org ID - use org ID for org repos, user ID for personal repos
  const currentOrg = account.organizationTools.organization
  const ownerId = currentOrg?.id || account.user?.id || ''

  const { data: repositories, isLoading, error } = useGitRepositories(ownerId)
  const { data: sampleTypes, loading: sampleTypesLoading, createSampleRepository } = useSampleTypes()

  // Dialog states
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [demoRepoDialogOpen, setDemoRepoDialogOpen] = useState(false)
  const [selectedSampleType, setSelectedSampleType] = useState('')
  const [demoRepoName, setDemoRepoName] = useState('')
  const [demoKoditIndexing, setDemoKoditIndexing] = useState(true)
  const [repoName, setRepoName] = useState('')
  const [repoDescription, setRepoDescription] = useState('')
  const [koditIndexing, setKoditIndexing] = useState(true)
  const [creating, setCreating] = useState(false)

  // Auto-fill name when sample type is selected
  const handleSampleTypeChange = (sampleTypeId: string) => {
    setSelectedSampleType(sampleTypeId)

    // Auto-generate default name from sample type
    if (sampleTypeId) {
      const selectedType = sampleTypes.find((t: ServerSampleType) => t.id === sampleTypeId)
      if (selectedType?.name) {
        // Generate a name like "nodejs-todo-repo" from "Node.js Todo App"
        const defaultName = selectedType.name
          .toLowerCase()
          .replace(/[^a-z0-9\s-]/g, '')
          .replace(/\s+/g, '-')
          .replace(/-+/g, '-')
        setDemoRepoName(defaultName)
      }
    }
  }

  const handleCreateDemoRepo = async () => {
    if (!selectedSampleType || !ownerId || !demoRepoName.trim()) return

    setCreating(true)
    try {
      await createSampleRepository({
        owner_id: ownerId,
        sample_type: selectedSampleType,
        name: demoRepoName,
        kodit_indexing: demoKoditIndexing,
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setDemoRepoDialogOpen(false)
      setSelectedSampleType('')
      setDemoRepoName('')
      setDemoKoditIndexing(true)
    } catch (error) {
      console.error('Failed to create demo repository:', error)
    } finally {
      setCreating(false)
    }
  }

  const handleCreateCustomRepo = async () => {
    if (!repoName.trim() || !ownerId) return

    setCreating(true)
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesCreate({
        name: repoName,
        description: repoDescription,
        owner_id: ownerId,
        repo_type: 'project' as any,
        default_branch: 'main',
        metadata: {
          kodit_indexing: koditIndexing,
        },
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setCreateDialogOpen(false)
      setRepoName('')
      setRepoDescription('')
      setKoditIndexing(true)
    } catch (error) {
      console.error('Failed to create repository:', error)
    } finally {
      setCreating(false)
    }
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
                    <Box sx={{ display: 'flex', gap: 0.5, flexWrap: 'wrap' }}>
                      <Chip
                        label={repo.repo_type || 'unknown'}
                        size="small"
                        color="primary"
                        variant="outlined"
                      />
                      {repo.metadata?.kodit_indexing && (
                        <Tooltip title="Code Intelligence enabled - Kodit indexes this repo for MCP server">
                          <Chip
                            icon={<Brain size={14} />}
                            label="Code Intelligence"
                            size="small"
                            color="success"
                            variant="outlined"
                          />
                        </Tooltip>
                      )}
                    </Box>
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

              <FormControl fullWidth required>
                <InputLabel>Demo Template</InputLabel>
                <Select
                  value={selectedSampleType}
                  onChange={(e) => handleSampleTypeChange(e.target.value)}
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
                <>
                  <TextField
                    label="Repository Name"
                    fullWidth
                    required
                    value={demoRepoName}
                    onChange={(e) => setDemoRepoName(e.target.value)}
                    helperText="Auto-generated from template, customize if needed"
                  />

                  <FormControlLabel
                    control={
                      <Switch
                        checked={demoKoditIndexing}
                        onChange={(e) => setDemoKoditIndexing(e.target.checked)}
                        color="primary"
                      />
                    }
                    label={
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        <Brain size={18} />
                        <Typography variant="body2">
                          Enable Code Intelligence
                        </Typography>
                      </Box>
                    }
                  />

                  <Alert severity="info">
                    {demoKoditIndexing
                      ? 'Code Intelligence enabled: Kodit will index this repository to provide code snippets and architectural summaries via MCP server.'
                      : sampleTypes.find((t: ServerSampleType) => t.id === selectedSampleType)?.description
                    }
                  </Alert>
                </>
              )}
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => {
              setDemoRepoDialogOpen(false)
              setSelectedSampleType('')
              setDemoRepoName('')
              setDemoKoditIndexing(true)
            }}>Cancel</Button>
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

              <FormControlLabel
                control={
                  <Switch
                    checked={koditIndexing}
                    onChange={(e) => setKoditIndexing(e.target.checked)}
                    color="primary"
                  />
                }
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Brain size={18} />
                    <Typography variant="body2">
                      Enable Code Intelligence
                    </Typography>
                  </Box>
                }
              />

              <Alert severity="info">
                {koditIndexing
                  ? 'Code Intelligence enabled: Kodit will index this repository to provide code snippets and architectural summaries via MCP server.'
                  : 'Code Intelligence disabled: Repository will not be indexed by Kodit.'
                }
              </Alert>
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => {
              setCreateDialogOpen(false)
              setRepoName('')
              setRepoDescription('')
              setKoditIndexing(true)
            }}>Cancel</Button>
            <Button
              onClick={handleCreateCustomRepo}
              variant="contained"
              disabled={!repoName.trim() || creating}
            >
              {creating ? <CircularProgress size={20} /> : 'Create'}
            </Button>
          </DialogActions>
        </Dialog>
      </Container>
    </Page>
  )
}

export default GitRepos
