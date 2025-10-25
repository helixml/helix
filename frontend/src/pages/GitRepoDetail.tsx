import React, { FC, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
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
  TextField,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  IconButton,
  Divider,
  Stack,
  FormControlLabel,
  Switch,
} from '@mui/material'
import { GitBranch, Copy, ExternalLink, ArrowLeft, Edit, Brain, Link, Trash2 } from 'lucide-react'
import { useQueryClient } from '@tanstack/react-query'

import Page from '../components/system/Page'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import { useGitRepository } from '../services/gitRepositoryService'

const GitRepoDetail: FC = () => {
  const { repoId } = useParams<{ repoId: string }>()
  const account = useAccount()
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const api = useApi()

  const currentOrg = account.organizationTools.organization
  const ownerSlug = currentOrg?.name || account.userMeta?.slug || 'user'
  const ownerId = currentOrg?.id || account.user?.id || ''

  const { data: repository, isLoading, error } = useGitRepository(repoId || '')
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  const [editName, setEditName] = useState('')
  const [editDescription, setEditDescription] = useState('')
  const [editKoditIndexing, setEditKoditIndexing] = useState(false)
  const [updating, setUpdating] = useState(false)
  const [deleting, setDeleting] = useState(false)
  const [copiedClone, setCopiedClone] = useState(false)

  const handleOpenEdit = () => {
    if (repository) {
      setEditName(repository.name || '')
      setEditDescription(repository.description || '')
      setEditKoditIndexing(repository.metadata?.kodit_indexing || false)
      setEditDialogOpen(true)
    }
  }

  const handleUpdateRepository = async () => {
    if (!repository || !repoId) return

    setUpdating(true)
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesUpdate(repoId, {
        name: editName,
        description: editDescription,
        metadata: {
          ...repository.metadata,
          kodit_indexing: editKoditIndexing,
        },
      })

      // Invalidate queries
      await queryClient.invalidateQueries({ queryKey: ['git-repository', repoId] })
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      setEditDialogOpen(false)
    } catch (error) {
      console.error('Failed to update repository:', error)
    } finally {
      setUpdating(false)
    }
  }

  const handleDeleteRepository = async () => {
    if (!repoId) return

    setDeleting(true)
    try {
      const apiClient = api.getApiClient()
      await apiClient.v1GitRepositoriesDelete(repoId)

      // Invalidate queries
      await queryClient.invalidateQueries({ queryKey: ['git-repositories', ownerId] })

      // Navigate back to list
      navigate('/git-repos')
    } catch (error) {
      console.error('Failed to delete repository:', error)
      setDeleting(false)
    }
  }

  const handleCopyCloneCommand = (command: string) => {
    navigator.clipboard.writeText(command)
    setCopiedClone(true)
    setTimeout(() => setCopiedClone(false), 2000)
  }

  if (isLoading) {
    return (
      <Page breadcrumbTitle="" orgBreadcrumbs={false}>
        <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
            <CircularProgress />
          </Box>
        </Container>
      </Page>
    )
  }

  if (error || !repository) {
    return (
      <Page breadcrumbTitle="" orgBreadcrumbs={false}>
        <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
          <Alert severity="error" sx={{ mb: 2 }}>
            {error instanceof Error ? error.message : 'Repository not found'}
          </Alert>
          <Button
            startIcon={<ArrowLeft size={16} />}
            onClick={() => navigate('/git-repos')}
          >
            Back to Repositories
          </Button>
        </Container>
      </Page>
    )
  }

  const cloneUrl = repository.clone_url || `git@github.com:${ownerSlug}/${repository.name}.git`
  const isExternal = repository.metadata?.is_external || false

  return (
    <Page breadcrumbTitle="" orgBreadcrumbs={false}>
      <Container maxWidth="lg" sx={{ mt: 4, mb: 4 }}>
        {/* Header with breadcrumb-style navigation */}
        <Box sx={{ mb: 3 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
            <Button
              startIcon={<ArrowLeft size={16} />}
              onClick={() => navigate('/git-repos')}
              sx={{ textTransform: 'none', color: '#0969da' }}
            >
              Repositories
            </Button>
          </Box>

          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
              <GitBranch size={32} color="#656d76" />
              <Box>
                <Typography variant="h4" component="h1" sx={{ fontWeight: 400, display: 'flex', alignItems: 'center', gap: 1 }}>
                  <span style={{ color: '#0969da' }}>{ownerSlug}</span>
                  <span style={{ color: '#656d76', fontWeight: 300 }}>/</span>
                  <span style={{ fontWeight: 600 }}>{repository.name}</span>
                </Typography>
                {repository.description && (
                  <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                    {repository.description}
                  </Typography>
                )}
              </Box>
            </Box>

            <Box sx={{ display: 'flex', gap: 1 }}>
              <IconButton
                onClick={handleOpenEdit}
                size="small"
                sx={{ border: 1, borderColor: 'divider' }}
              >
                <Edit size={16} />
              </IconButton>
              <IconButton
                onClick={() => setDeleteDialogOpen(true)}
                size="small"
                sx={{ border: 1, borderColor: 'divider' }}
              >
                <Trash2 size={16} />
              </IconButton>
            </Box>
          </Box>

          {/* Chips */}
          <Box sx={{ display: 'flex', gap: 1, mt: 2 }}>
            {isExternal && (
              <Chip
                icon={<Link size={12} />}
                label={repository.metadata?.external_type || 'External'}
                size="small"
              />
            )}
            {repository.metadata?.kodit_indexing && (
              <Chip
                icon={<Brain size={12} />}
                label="Code Intelligence"
                size="small"
                color="success"
              />
            )}
            <Chip
              label={repository.repo_type || 'project'}
              size="small"
              variant="outlined"
            />
          </Box>
        </Box>

        <Divider sx={{ mb: 3 }} />

        {/* Clone instructions */}
        <Card sx={{ mb: 3 }}>
          <CardContent>
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
              Clone this repository
            </Typography>

            {isExternal && repository.metadata?.external_url ? (
              <>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                  This is an external repository. Use the URL below to clone it:
                </Typography>
                <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
                  <TextField
                    fullWidth
                    value={repository.metadata.external_url}
                    InputProps={{
                      readOnly: true,
                      sx: { fontFamily: 'monospace', fontSize: '0.875rem' }
                    }}
                  />
                  <Button
                    variant="outlined"
                    startIcon={copiedClone ? undefined : <Copy size={16} />}
                    onClick={() => handleCopyCloneCommand(repository.metadata.external_url)}
                  >
                    {copiedClone ? 'Copied!' : 'Copy'}
                  </Button>
                  <Button
                    variant="outlined"
                    startIcon={<ExternalLink size={16} />}
                    onClick={() => window.open(repository.metadata.external_url, '_blank')}
                  >
                    Open
                  </Button>
                </Box>
              </>
            ) : (
              <>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                  Clone this repository to your local machine using Git:
                </Typography>
                <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
                  <TextField
                    fullWidth
                    value={`git clone ${cloneUrl}`}
                    InputProps={{
                      readOnly: true,
                      sx: { fontFamily: 'monospace', fontSize: '0.875rem' }
                    }}
                  />
                  <Button
                    variant="outlined"
                    startIcon={copiedClone ? undefined : <Copy size={16} />}
                    onClick={() => handleCopyCloneCommand(`git clone ${cloneUrl}`)}
                  >
                    {copiedClone ? 'Copied!' : 'Copy'}
                  </Button>
                </Box>

                <Alert severity="info" sx={{ mt: 2 }}>
                  <Typography variant="body2" sx={{ fontWeight: 600, mb: 1 }}>
                    Setup Instructions:
                  </Typography>
                  <Typography variant="body2" component="div">
                    1. Ensure you have Git installed on your machine
                    <br />
                    2. Configure your SSH key or Git credentials
                    <br />
                    3. Run the clone command above in your terminal
                    <br />
                    4. Navigate to the cloned directory: <code>cd {repository.name}</code>
                  </Typography>
                </Alert>
              </>
            )}
          </CardContent>
        </Card>

        {/* Repository information */}
        <Card>
          <CardContent>
            <Typography variant="h6" sx={{ mb: 2, fontWeight: 600 }}>
              Repository Information
            </Typography>

            <Stack spacing={2}>
              <Box>
                <Typography variant="caption" color="text.secondary">
                  Repository ID
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                  {repository.id}
                </Typography>
              </Box>

              <Box>
                <Typography variant="caption" color="text.secondary">
                  Type
                </Typography>
                <Typography variant="body2">
                  {repository.repo_type || 'project'}
                </Typography>
              </Box>

              {repository.default_branch && (
                <Box>
                  <Typography variant="caption" color="text.secondary">
                    Default Branch
                  </Typography>
                  <Typography variant="body2">
                    {repository.default_branch}
                  </Typography>
                </Box>
              )}

              <Box>
                <Typography variant="caption" color="text.secondary">
                  Created
                </Typography>
                <Typography variant="body2">
                  {repository.created_at ? new Date(repository.created_at).toLocaleString() : 'N/A'}
                </Typography>
              </Box>

              {repository.updated_at && (
                <Box>
                  <Typography variant="caption" color="text.secondary">
                    Last Updated
                  </Typography>
                  <Typography variant="body2">
                    {new Date(repository.updated_at).toLocaleString()}
                  </Typography>
                </Box>
              )}
            </Stack>
          </CardContent>
        </Card>

        {/* Edit Dialog */}
        <Dialog open={editDialogOpen} onClose={() => setEditDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Edit Repository</DialogTitle>
          <DialogContent>
            <Stack spacing={2} sx={{ mt: 1 }}>
              <TextField
                label="Repository Name"
                fullWidth
                value={editName}
                onChange={(e) => setEditName(e.target.value)}
              />

              <TextField
                label="Description"
                fullWidth
                multiline
                rows={3}
                value={editDescription}
                onChange={(e) => setEditDescription(e.target.value)}
              />

              <FormControlLabel
                control={
                  <Switch
                    checked={editKoditIndexing}
                    onChange={(e) => setEditKoditIndexing(e.target.checked)}
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
            </Stack>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setEditDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={handleUpdateRepository}
              variant="contained"
              disabled={!editName.trim() || updating}
            >
              {updating ? <CircularProgress size={20} /> : 'Save Changes'}
            </Button>
          </DialogActions>
        </Dialog>

        {/* Delete Confirmation Dialog */}
        <Dialog open={deleteDialogOpen} onClose={() => setDeleteDialogOpen(false)} maxWidth="sm" fullWidth>
          <DialogTitle>Delete Repository</DialogTitle>
          <DialogContent>
            <Alert severity="warning" sx={{ mb: 2 }}>
              This action cannot be undone. This will permanently delete the repository metadata.
            </Alert>
            <Typography variant="body2">
              Are you sure you want to delete <strong>{repository.name}</strong>?
            </Typography>
          </DialogContent>
          <DialogActions>
            <Button onClick={() => setDeleteDialogOpen(false)}>Cancel</Button>
            <Button
              onClick={handleDeleteRepository}
              variant="contained"
              color="error"
              disabled={deleting}
            >
              {deleting ? <CircularProgress size={20} /> : 'Delete Repository'}
            </Button>
          </DialogActions>
        </Dialog>
      </Container>
    </Page>
  )
}

export default GitRepoDetail
