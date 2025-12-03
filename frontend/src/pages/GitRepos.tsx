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
} from '@mui/material'
import { GitBranch, Plus, Brain, Link } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import Page from '../components/system/Page'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'
import CreateRepositoryDialog from '../components/project/CreateRepositoryDialog'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import { useGitRepositories } from '../services/gitRepositoryService'
import { useSampleTypes } from '../hooks/useSampleTypes'
import { getSampleProjectIcon } from '../utils/sampleProjectIcons'
import type { TypesGitRepository, ServerSampleType, TypesExternalRepositoryType } from '../api/api'

const GitRepos: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const { navigate } = router
  const queryClient = useQueryClient()
  const api = useApi()

  // Get current org context for repo filtering
  const currentOrg = account.organizationTools.organization
  const ownerId = account.user?.id || ''

  // Get owner slug for GitHub-style URLs (org name or user slug)
  const ownerSlug = currentOrg?.name || account.userMeta?.slug || 'user'

  // List repos by organization_id when in org context, or by owner_id for personal workspace
  const { data: repositories, isLoading, error } = useGitRepositories(
    currentOrg?.id
      ? { organizationId: currentOrg.id }
      : { ownerId: account.user?.id }
  )
  const { data: sampleTypes, loading: sampleTypesLoading, createSampleRepository } = useSampleTypes()

  // Dialog states
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [demoRepoDialogOpen, setDemoRepoDialogOpen] = useState(false)
  const [linkRepoDialogOpen, setLinkRepoDialogOpen] = useState(false)
  const [selectedSampleType, setSelectedSampleType] = useState('')
  const [demoRepoName, setDemoRepoName] = useState('')
  const [demoKoditIndexing, setDemoKoditIndexing] = useState(true)

  // External repository states
  const [externalRepoName, setExternalRepoName] = useState('')
  const [externalRepoUrl, setExternalRepoUrl] = useState('')
  const [externalRepoType, setExternalRepoType] = useState<TypesExternalRepositoryType>('github' as TypesExternalRepositoryType)
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
      await queryClient.invalidateQueries({ queryKey: ['git-repositories'] })

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

  const handleCreateCustomRepo = async (name: string, description: string, koditIndexing: boolean) => {
    if (!name.trim() || !ownerId) return

    setCreating(true)
    setCreateError('')
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesCreate({
        name,
        description,
        owner_id: ownerId,
        repo_type: 'code' as any, // Helix-hosted code repository
        default_branch: 'main',
        kodit_indexing: koditIndexing,
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories'] })

      setCreateDialogOpen(false)
      setCreateError('')
    } catch (error) {
      console.error('Failed to create repository:', error)
      setCreateError(error instanceof Error ? error.message : 'Failed to create repository')
      throw error // Re-throw so CreateRepositoryDialog knows it failed
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
        repo_type: 'code' as any,
        default_branch: 'main',
        is_external: true,
        external_url: externalRepoUrl,
        external_type: externalRepoType,
        kodit_indexing: externalKoditIndexing,
      })

      // Invalidate and refetch git repositories query
      await queryClient.invalidateQueries({ queryKey: ['git-repositories'] })

      setLinkRepoDialogOpen(false)
      setExternalRepoName('')
      setExternalRepoUrl('')
      setExternalRepoType('github' as TypesExternalRepositoryType)
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
                  <span style={{ color: '#3b82f6', cursor: 'pointer' }}>{ownerSlug}</span>
                  <span style={{ color: 'text.secondary', fontWeight: 300 }}>/</span>
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
                color="secondary"
                size="small"
                startIcon={<Plus size={16} />}
                onClick={() => setCreateDialogOpen(true)}
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
            {repositories.map((repo: TypesGitRepository) => (
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
                onClick={() => account.orgNavigate('git-repo-detail', { repoId: repo.id })}
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
                          color: '#3b82f6',
                          display: 'flex',
                          alignItems: 'center',
                          gap: 0.5,
                          '&:hover': {
                            textDecoration: 'underline'
                          }
                        }}
                      >
                        {ownerSlug}
                        <span style={{ color: 'text.secondary', fontWeight: 400 }}>/</span>
                        {repo.name || repo.id}
                      </Typography>

                      {/* Chips */}
                      {repo.is_external && (
                        <Chip
                          icon={<Link size={12} />}
                          label={repo.external_type || 'External'}
                          size="small"
                          sx={{ height: 20, fontSize: '0.75rem' }}
                        />
                      )}
                      {repo.kodit_indexing && (
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
                        {getSampleProjectIcon(type.id, undefined, 18)}
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
        <CreateRepositoryDialog
          open={createDialogOpen}
          onClose={() => {
            setCreateDialogOpen(false)
            setCreateError('')
          }}
          onSubmit={handleCreateCustomRepo}
          isCreating={creating}
          error={createError}
        />
      </Container>
    </Page>
  )
}

export default GitRepos
