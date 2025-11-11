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
import { GitBranch, Plus, ExternalLink, Brain, Link } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import Page from '../components/system/Page'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import { useGitRepositories } from '../services/gitRepositoryService'
import { useSampleTypes } from '../hooks/useSampleTypes'
import { getSampleProjectIcon } from '../utils/sampleProjectIcons'
import type { ServicesGitRepository, ServerSampleType } from '../api/api'

const GitRepos: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const { navigate } = router
  const queryClient = useQueryClient()
  const api = useApi()

  // Get current org ID - use org ID for org repos, user ID for personal repos
  const currentOrg = account.organizationTools.organization
  const ownerId = currentOrg?.id || account.user?.id || ''

  // Get owner slug for GitHub-style URLs (org name or user slug)
  const ownerSlug = currentOrg?.name || account.userMeta?.slug || 'user'

  const { data: repositories, isLoading, error } = useGitRepositories(ownerId)
  const { data: sampleTypes, loading: sampleTypesLoading, createSampleRepository } = useSampleTypes()

  // Dialog states
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [demoRepoDialogOpen, setDemoRepoDialogOpen] = useState(false)
  const [linkRepoDialogOpen, setLinkRepoDialogOpen] = useState(false)
  const [selectedSampleType, setSelectedSampleType] = useState('')
  const [demoRepoName, setDemoRepoName] = useState('')
  const [demoKoditIndexing, setDemoKoditIndexing] = useState(true)
  const [repoName, setRepoName] = useState('')
  const [repoDescription, setRepoDescription] = useState('')
  const [koditIndexing, setKoditIndexing] = useState(true)

  // External repository states
  const [externalRepoName, setExternalRepoName] = useState('')
  const [externalRepoUrl, setExternalRepoUrl] = useState('')
  const [externalRepoType, setExternalRepoType] = useState<'github' | 'gitlab' | 'ado' | 'other'>('github')
  const [externalKoditIndexing, setExternalKoditIndexing] = useState(true)

  const [creating, setCreating] = useState(false)
  const [createError, setCreateError] = useState<string>('')

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
    setCreateError('')
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesCreate({
        name: repoName,
        description: repoDescription,
        owner_id: ownerId,
        repo_type: 'code' as any, // Helix-hosted code repository
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
      setCreateError('')
    } catch (error) {
      console.error('Failed to create repository:', error)
      setCreateError(error instanceof Error ? error.message : 'Failed to create repository')
    } finally {
      setCreating(false)
    }
  }

  const handleLinkExternalRepo = async () => {
    if (!externalRepoUrl.trim() || !ownerId) return

    setCreating(true)
    try {
      const apiClient = api.getApiClient()

      // Extract repo name from URL if not provided
      let repoName = externalRepoName.trim()
      if (!repoName) {
        // Try to extract from URL (e.g., github.com/org/repo.git -> repo)
        const match = externalRepoUrl.match(/\/([^\/]+?)(\.git)?$/)
        repoName = match ? match[1] : 'external-repo'
      }

      await apiClient.v1GitRepositoriesCreate({
        name: repoName,
        description: `External ${externalRepoType} repository`,
        owner_id: ownerId,
        repo_type: 'project' as any,
        default_branch: 'main',
        metadata: {
          is_external: true,
          external_url: externalRepoUrl,
          external_type: externalRepoType,
          kodit_indexing: externalKoditIndexing,
        },
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setLinkRepoDialogOpen(false)
      setExternalRepoName('')
      setExternalRepoUrl('')
      setExternalRepoType('github')
      setExternalKoditIndexing(true)
    } catch (error) {
      console.error('Failed to link external repository:', error)
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
      breadcrumbTitle=""
      orgBreadcrumbs={false}
    >
      <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
        {/* GitHub-style header with owner/repositories */}
        <Box sx={{ mb: 3, borderBottom: 1, borderColor: 'divider', pb: 2 }}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Box>
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                <Typography variant="h4" component="h1" sx={{ fontWeight: 400, display: 'flex', alignItems: 'center', gap: 1 }}>
                  <span style={{ color: '#0969da', cursor: 'pointer' }}>{ownerSlug}</span>
                  <span style={{ color: '#656d76', fontWeight: 300 }}>/</span>
                  <span style={{ fontWeight: 600 }}>repositories</span>
                </Typography>
              </Box>
            </Box>
            <Box sx={{ display: 'flex', gap: 1 }}>
              <Button
                variant="outlined"
                size="small"
                startIcon={<GitBranch size={16} />}
                onClick={() => setDemoRepoDialogOpen(true)}
                sx={{ textTransform: 'none' }}
              >
                From demo
              </Button>
              <Button
                variant="outlined"
                size="small"
                startIcon={<Link size={16} />}
                onClick={() => setLinkRepoDialogOpen(true)}
                sx={{ textTransform: 'none' }}
              >
                Link external
              </Button>
              <Button
                variant="contained"
                size="small"
                startIcon={<Plus size={16} />}
                onClick={() => setCreateDialogOpen(true)}
                sx={{
                  bgcolor: '#1a7f37',
                  '&:hover': { bgcolor: '#1a7f37dd' },
                  textTransform: 'none'
                }}
              >
                New
              </Button>
            </Box>
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
          /* GitHub-style list view */
          <Box>
            {repositories.map((repo: ServicesGitRepository) => (
              <Box
                key={repo.id}
                sx={{
                  py: 3,
                  px: 2,
                  borderBottom: 1,
                  borderColor: 'divider',
                  '&:hover': {
                    bgcolor: 'rgba(0, 0, 0, 0.02)',
                  },
                  cursor: 'pointer'
                }}
                onClick={() => navigate('git-repo-detail', { repoId: repo.id })}
              >
                <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                  <Box sx={{ flex: 1 }}>
                    {/* Repo name as owner/repo */}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                      <GitBranch size={16} color="#656d76" />
                      <Typography
                        variant="h6"
                        sx={{
                          fontSize: '1.25rem',
                          fontWeight: 600,
                          color: '#0969da',
                          display: 'flex',
                          alignItems: 'center',
                          gap: 0.5,
                          '&:hover': {
                            textDecoration: 'underline'
                          }
                        }}
                      >
                        {ownerSlug}
                        <span style={{ color: '#656d76', fontWeight: 400 }}>/</span>
                        {repo.name || repo.id}
                      </Typography>

                      {/* Chips */}
                      {repo.metadata?.is_external && (
                        <Chip
                          icon={<Link size={12} />}
                          label={repo.metadata.external_type || 'External'}
                          size="small"
                          sx={{ height: 20, fontSize: '0.75rem' }}
                        />
                      )}
                      {repo.metadata?.kodit_indexing && (
                        <Chip
                          icon={<Brain size={12} />}
                          label="Code Intelligence"
                          size="small"
                          color="success"
                          sx={{ height: 20, fontSize: '0.75rem' }}
                        />
                      )}
                      <Chip
                        label={repo.repo_type || 'project'}
                        size="small"
                        variant="outlined"
                        sx={{ height: 20, fontSize: '0.75rem', borderRadius: '12px' }}
                      />
                    </Box>

                    {/* Description */}
                    {repo.description && (
                      <Typography variant="body2" color="text.secondary" sx={{ mb: 1, ml: 3 }}>
                        {repo.description}
                      </Typography>
                    )}

                    {/* Metadata row */}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, ml: 3, mt: 1 }}>
                      {repo.created_at && (
                        <Typography variant="caption" color="text.secondary">
                          Updated {new Date(repo.created_at).toLocaleDateString()}
                        </Typography>
                      )}
                    </Box>
                  </Box>

                  {/* Action buttons */}
                  <Box onClick={(e) => e.stopPropagation()} sx={{ display: 'flex', gap: 1 }}>
                    <Button
                      size="small"
                      variant="contained"
                      startIcon={<Plus size={14} />}
                      onClick={() => navigate('spec-tasks', { new: 'true', repo_id: repo.id })}
                      sx={{ textTransform: 'none' }}
                    >
                      New Spec Task
                    </Button>
                    {repo.metadata?.is_external && repo.metadata?.external_url ? (
                      <Button
                        size="small"
                        variant="outlined"
                        startIcon={<ExternalLink size={14} />}
                        onClick={() => window.open(repo.metadata.external_url, '_blank')}
                        sx={{ textTransform: 'none' }}
                      >
                        View
                      </Button>
                    ) : (
                      <Button
                        size="small"
                        variant="outlined"
                        onClick={() => navigate('git-repo-detail', { repoId: repo.id })}
                        sx={{ textTransform: 'none' }}
                      >
                        Clone
                      </Button>
                    )}
                  </Box>
                </Box>
              </Box>
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
                Create your first git repository to start collaborating with AI agents and your team.
              </Typography>
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
                      <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                        {getSampleProjectIcon(type.id, type.category, 18)}
                        <span>{type.name}</span>
                      </Box>
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
        <Dialog open={createDialogOpen} onClose={() => {
          setCreateDialogOpen(false)
          setCreateError('')
        }} maxWidth="sm" fullWidth>
          <DialogTitle>Create New Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              {createError && (
                <Alert severity="error" onClose={() => setCreateError('')}>
                  {createError}
                </Alert>
              )}

              <TextField
                label="Repository Name"
                fullWidth
                value={repoName}
                onChange={(e) => setRepoName(e.target.value)}
                helperText="Enter a name for your repository"
                autoFocus
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
              setCreateError('')
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

        {/* Link External Repository Dialog */}
        <Dialog open={linkRepoDialogOpen} onClose={() => setLinkRepoDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Link External Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              <Typography variant="body2" color="text.secondary">
                Link an existing repository from GitHub, GitLab, or Azure DevOps to enable AI collaboration.
              </Typography>

              <FormControl fullWidth required>
                <InputLabel>Repository Type</InputLabel>
                <Select
                  value={externalRepoType}
                  onChange={(e) => setExternalRepoType(e.target.value as 'github' | 'gitlab' | 'ado' | 'other')}
                  label="Repository Type"
                >
                  <MenuItem value="github">GitHub</MenuItem>
                  <MenuItem value="gitlab">GitLab</MenuItem>
                  <MenuItem value="ado">Azure DevOps</MenuItem>
                  <MenuItem value="other">Other (Bitbucket, Gitea, Self-hosted, etc.)</MenuItem>
                </Select>
              </FormControl>

              <TextField
                label="Repository URL"
                fullWidth
                required
                value={externalRepoUrl}
                onChange={(e) => setExternalRepoUrl(e.target.value)}
                placeholder="https://github.com/org/repo.git"
                helperText="Full URL to the external repository"
              />

              <TextField
                label="Repository Name (Optional)"
                fullWidth
                value={externalRepoName}
                onChange={(e) => setExternalRepoName(e.target.value)}
                helperText="Display name (auto-extracted from URL if empty)"
              />

              <FormControlLabel
                control={
                  <Switch
                    checked={externalKoditIndexing}
                    onChange={(e) => setExternalKoditIndexing(e.target.checked)}
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

              <Alert severity="warning">
                Authentication to external repositories is not yet implemented. You can link repositories for reference, but cloning and syncing will require manual setup.
              </Alert>

              <Alert severity="info">
                {externalKoditIndexing
                  ? 'Code Intelligence enabled: Kodit will index this external repository to provide code snippets and architectural summaries via MCP server.'
                  : 'Code Intelligence disabled: Repository will not be indexed by Kodit.'
                }
              </Alert>
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => {
              setLinkRepoDialogOpen(false)
              setExternalRepoName('')
              setExternalRepoUrl('')
              setExternalRepoType('github')
              setExternalKoditIndexing(true)
            }}>Cancel</Button>
            <Button
              onClick={handleLinkExternalRepo}
              variant="contained"
              disabled={!externalRepoUrl.trim() || creating}
            >
              {creating ? <CircularProgress size={20} /> : 'Link Repository'}
            </Button>
          </DialogActions>
        </Dialog>
      </Container>
    </Page>
  )
}

export default GitRepos
