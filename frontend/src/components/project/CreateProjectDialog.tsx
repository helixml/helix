import React, { FC, useState, useEffect } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Typography,
  Divider,
  Alert,
  ToggleButtonGroup,
  ToggleButton,
  CircularProgress,
} from '@mui/material'
import { FolderGit2, Link as LinkIcon, Plus } from 'lucide-react'
import { TypesExternalRepositoryType } from '../../api/api'
import type { TypesGitRepository, TypesAzureDevOps } from '../../api/api'
import NewRepoForm from './forms/NewRepoForm'
import ExternalRepoForm from './forms/ExternalRepoForm'
import { useCreateProject } from '../../services'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'

type RepoMode = 'select' | 'create' | 'link'

interface CreateProjectDialogProps {
  open: boolean
  onClose: () => void
  onSuccess?: (projectId: string) => void
  // For selecting existing repos
  repositories: TypesGitRepository[]
  reposLoading?: boolean
  // For creating new repos
  onCreateRepo?: (name: string, description: string) => Promise<TypesGitRepository | null>
  // For linking external repos
  onLinkRepo?: (url: string, name: string, type: TypesExternalRepositoryType, username?: string, password?: string, azureDevOps?: TypesAzureDevOps) => Promise<TypesGitRepository | null>
}

const CreateProjectDialog: FC<CreateProjectDialogProps> = ({
  open,
  onClose,
  onSuccess,
  repositories,
  reposLoading,
  onCreateRepo,
  onLinkRepo,
}) => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const createProjectMutation = useCreateProject()
  const [name, setName] = useState('')
  const [description, setDescription] = useState('')
  const [selectedRepoId, setSelectedRepoId] = useState('')
  const [repoMode, setRepoMode] = useState<RepoMode>('select')

  // New repo creation fields
  const [newRepoName, setNewRepoName] = useState('')
  const [newRepoDescription, setNewRepoDescription] = useState('')

  // External repo linking fields
  const [externalUrl, setExternalUrl] = useState('')
  const [externalName, setExternalName] = useState('')
  const [externalType, setExternalType] = useState<TypesExternalRepositoryType>(TypesExternalRepositoryType.ExternalRepositoryTypeGitHub)
  const [externalUsername, setExternalUsername] = useState('')
  const [externalPassword, setExternalPassword] = useState('')
  const [externalOrgUrl, setExternalOrgUrl] = useState('')
  const [externalToken, setExternalToken] = useState('')

  const [creatingRepo, setCreatingRepo] = useState(false)
  const [repoError, setRepoError] = useState('')

  // Filter out internal repos - they're deprecated
  const codeRepos = repositories.filter(r => r.repo_type !== 'internal')

  // Reset form when dialog closes
  useEffect(() => {
    if (!open) {
      setName('')
      setDescription('')
      setSelectedRepoId('')
      setRepoMode('select')
      setNewRepoName('')
      setNewRepoDescription('')
      setExternalUrl('')
      setExternalName('')
      setExternalType(TypesExternalRepositoryType.ExternalRepositoryTypeGitHub)
      setExternalUsername('')
      setExternalPassword('')
      setExternalOrgUrl('')
      setExternalToken('')
      setRepoError('')
    }
  }, [open])

  // Auto-select first repo if available
  useEffect(() => {
    if (open && codeRepos.length > 0 && !selectedRepoId) {
      setSelectedRepoId(codeRepos[0].id || '')
    }
  }, [open, codeRepos, selectedRepoId])

  const handleSubmit = async () => {
    if (!name.trim()) {
      snackbar.error('Project name is required')
      return
    }

    let repoIdToUse = ''
    setRepoError('')

    if (repoMode === 'select') {
      if (!selectedRepoId) {
        setRepoError('Please select a repository')
        return
      }
      repoIdToUse = selectedRepoId
    } else if (repoMode === 'create') {
      if (!newRepoName.trim()) {
        setRepoError('Please enter a repository name')
        return
      }
      if (!onCreateRepo) {
        setRepoError('Repository creation not available')
        return
      }

      setCreatingRepo(true)
      try {
        const newRepo = await onCreateRepo(newRepoName, newRepoDescription)
        if (!newRepo?.id) {
          setRepoError('Failed to create repository')
          return
        }
        repoIdToUse = newRepo.id
      } catch (err) {
        setRepoError('Failed to create repository')
        return
      } finally {
        setCreatingRepo(false)
      }
    } else if (repoMode === 'link') {
      if (!externalUrl.trim()) {
        setRepoError('Please enter a repository URL')
        return
      }
      if (!onLinkRepo) {
        setRepoError('External repository linking not available')
        return
      }

      // ADO validation
      if (externalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO && (!externalOrgUrl.trim() || !externalToken.trim())) {
        setRepoError('Organization URL and Personal Access Token are required for Azure DevOps')
        return
      }

      setCreatingRepo(true)
      try {
        const repoName = externalName || externalUrl.split('/').pop()?.replace('.git', '') || 'external-repo'
        const azureDevOps: TypesAzureDevOps | undefined = externalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO ? {
          organization_url: externalOrgUrl,
          personal_access_token: externalToken,
        } : undefined

        const linkedRepo = await onLinkRepo(
          externalUrl,
          repoName,
          externalType,
          externalUsername || undefined,
          externalPassword || undefined,
          azureDevOps
        )
        if (!linkedRepo?.id) {
          setRepoError('Failed to link repository')
          return
        }
        repoIdToUse = linkedRepo.id
      } catch (err) {
        setRepoError('Failed to link repository')
        return
      } finally {
        setCreatingRepo(false)
      }
    }

    if (!repoIdToUse) {
      snackbar.error('Primary repository is required')
      return
    }

    try {
      // DEBUG: Check org state when creating project
      console.log('[CreateProjectDialog] Creating project with:', {
        orgID: account.organizationTools.orgID,
        organization: account.organizationTools.organization,
        organizationId: account.organizationTools.organization?.id,
      })

      const result = await createProjectMutation.mutateAsync({
        name,
        description,
        default_repo_id: repoIdToUse,
        organization_id: account.organizationTools.organization?.id,
      })
      snackbar.success('Project created successfully')
      onClose()

      if (result?.id) {
        if (onSuccess) {
          onSuccess(result.id)
        } else {
          account.orgNavigate('project-specs', { id: result.id })
        }
      }
    } catch (err) {
      snackbar.error('Failed to create project')
    }
  }

  const isSubmitDisabled = createProjectMutation.isPending || creatingRepo || !name.trim() || (
    repoMode === 'select' ? !selectedRepoId :
    repoMode === 'create' ? !newRepoName.trim() :
    !externalUrl.trim() || (externalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO && (!externalOrgUrl.trim() || !externalToken.trim()))
  )

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Create New Project</DialogTitle>
      <DialogContent>
        <Box sx={{ pt: 2, display: 'flex', flexDirection: 'column', gap: 2 }}>
          <TextField
            label="Project Name"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            autoFocus
            required
          />
          <TextField
            label="Description"
            fullWidth
            multiline
            rows={2}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />

          <Divider sx={{ my: 1 }} />

          <Typography variant="subtitle2" color="text.secondary">
            Primary Repository (required)
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Project configuration and startup scripts will be stored in this repository.
            You can attach additional repositories later in Project Settings.
          </Typography>

          <ToggleButtonGroup
            value={repoMode}
            exclusive
            onChange={(_, v) => v && setRepoMode(v)}
            size="small"
            fullWidth
          >
            <ToggleButton value="select">
              <FolderGit2 size={16} style={{ marginRight: 4 }} />
              Existing
            </ToggleButton>
            <ToggleButton value="create">
              <Plus size={16} style={{ marginRight: 4 }} />
              New
            </ToggleButton>
            <ToggleButton value="link">
              <LinkIcon size={16} style={{ marginRight: 4 }} />
              External
            </ToggleButton>
          </ToggleButtonGroup>

          {repoMode === 'select' && (
            <FormControl fullWidth size="small">
              <InputLabel>Select Repository</InputLabel>
              <Select
                value={selectedRepoId}
                label="Select Repository"
                onChange={(e) => setSelectedRepoId(e.target.value)}
                disabled={reposLoading}
              >
                {codeRepos.map((repo) => (
                  <MenuItem key={repo.id} value={repo.id}>
                    {repo.name}
                    {repo.is_external && ` (${repo.external_type || 'external'})`}
                  </MenuItem>
                ))}
                {codeRepos.length === 0 && (
                  <MenuItem disabled value="">
                    No repositories available
                  </MenuItem>
                )}
              </Select>
            </FormControl>
          )}

          {repoMode === 'create' && (
            <NewRepoForm
              name={newRepoName}
              onNameChange={setNewRepoName}
              description={newRepoDescription}
              onDescriptionChange={setNewRepoDescription}
              size="small"
            />
          )}

          {repoMode === 'link' && (
            <ExternalRepoForm
              url={externalUrl}
              onUrlChange={setExternalUrl}
              name={externalName}
              onNameChange={setExternalName}
              type={externalType}
              onTypeChange={setExternalType}
              username={externalUsername}
              onUsernameChange={setExternalUsername}
              password={externalPassword}
              onPasswordChange={setExternalPassword}
              organizationUrl={externalOrgUrl}
              onOrganizationUrlChange={setExternalOrgUrl}
              token={externalToken}
              onTokenChange={setExternalToken}
              size="small"
            />
          )}

          {repoError && (
            <Alert severity="error" sx={{ mt: 1 }}>
              {repoError}
            </Alert>
          )}
        </Box>
      </DialogContent>
      <DialogActions>
        <Button
          variant="contained"
          color="secondary"
          onClick={handleSubmit}
          disabled={isSubmitDisabled}
          sx={{ mr: 1, mb: 1 }}
        >
          {createProjectMutation.isPending || creatingRepo ? (
            <>
              <CircularProgress size={16} sx={{ mr: 1 }} />
              Creating...
            </>
          ) : (
            'Create Project'
          )}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default CreateProjectDialog
