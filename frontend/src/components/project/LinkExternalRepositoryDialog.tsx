import React, { FC, useState, useEffect } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Stack,
  FormControlLabel,
  Switch,
  Box,
  Typography,
  CircularProgress,
  Tooltip,
  Collapse,
  Divider,
  Alert,
} from '@mui/material'
import { Brain, FolderSearch } from 'lucide-react'
import { TypesExternalRepositoryType, TypesRepositoryInfo } from '../../api/api'
import ExternalRepoForm from './forms/ExternalRepoForm'
import RepositoryBrowser from './RepositoryBrowser'
import { useBrowseRemoteRepositories } from '../../services/gitRepositoryService'

interface LinkExternalRepositoryDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (url: string, name: string, type: 'github' | 'gitlab' | 'ado' | 'other', koditIndexing: boolean, username?: string, password?: string, organizationUrl?: string, token?: string, gitlabBaseUrl?: string) => Promise<void>
  isCreating: boolean
}

const LinkExternalRepositoryDialog: FC<LinkExternalRepositoryDialogProps> = ({
  open,
  onClose,
  onSubmit,
  isCreating,
}) => {
  const [url, setUrl] = useState('')
  const [name, setName] = useState('')
  const [type, setType] = useState<TypesExternalRepositoryType>(TypesExternalRepositoryType.ExternalRepositoryTypeGitHub)
  const [koditIndexing, setKoditIndexing] = useState(true)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [organizationUrl, setOrganizationUrl] = useState('')
  const [token, setToken] = useState('')
  const [gitlabBaseUrl, setGitlabBaseUrl] = useState('')
  const [showBrowser, setShowBrowser] = useState(false)
  const [repositories, setRepositories] = useState<TypesRepositoryInfo[] | undefined>(undefined)
  const [browseError, setBrowseError] = useState<Error | null>(null)

  const browseRemoteMutation = useBrowseRemoteRepositories()

  // Reset form when dialog closes
  useEffect(() => {
    if (!open) {
      setUrl('')
      setName('')
      setType(TypesExternalRepositoryType.ExternalRepositoryTypeGitHub)
      setKoditIndexing(true)
      setUsername('')
      setPassword('')
      setOrganizationUrl('')
      setToken('')
      setGitlabBaseUrl('')
      setShowBrowser(false)
      setRepositories(undefined)
      setBrowseError(null)
    }
  }, [open])

  const handleSubmit = async () => {
    // Map enum to string for backward compatibility with onSubmit signature
    const typeMap: Record<TypesExternalRepositoryType, 'github' | 'gitlab' | 'ado' | 'other'> = {
      [TypesExternalRepositoryType.ExternalRepositoryTypeGitHub]: 'github',
      [TypesExternalRepositoryType.ExternalRepositoryTypeGitLab]: 'gitlab',
      [TypesExternalRepositoryType.ExternalRepositoryTypeADO]: 'ado',
      [TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket]: 'other',
    }
    const submitType = typeMap[type]
    await onSubmit(url, name, submitType, koditIndexing, username || undefined, password || undefined, organizationUrl || undefined, token || undefined, gitlabBaseUrl || undefined)
  }

  const handleBrowseRepositories = async () => {
    if (!token.trim()) {
      setBrowseError(new Error('Personal Access Token is required to browse repositories'))
      return
    }

    // Map type to provider_type
    const providerTypeMap: Record<TypesExternalRepositoryType, 'github' | 'gitlab' | 'ado'> = {
      [TypesExternalRepositoryType.ExternalRepositoryTypeGitHub]: 'github',
      [TypesExternalRepositoryType.ExternalRepositoryTypeGitLab]: 'gitlab',
      [TypesExternalRepositoryType.ExternalRepositoryTypeADO]: 'ado',
      [TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket]: 'github', // Fallback, Bitbucket not supported
    }

    if (type === TypesExternalRepositoryType.ExternalRepositoryTypeADO && !organizationUrl.trim()) {
      setBrowseError(new Error('Organization URL is required for Azure DevOps'))
      return
    }

    setBrowseError(null)
    setRepositories(undefined)
    setShowBrowser(true)

    try {
      const result = await browseRemoteMutation.mutateAsync({
        provider_type: providerTypeMap[type],
        token: token,
        organization_url: type === TypesExternalRepositoryType.ExternalRepositoryTypeADO ? organizationUrl : undefined,
        base_url: type === TypesExternalRepositoryType.ExternalRepositoryTypeGitLab ? gitlabBaseUrl : undefined,
      })
      setRepositories(result?.repositories || [])
    } catch (err) {
      setBrowseError(err instanceof Error ? err : new Error('Failed to browse repositories'))
    }
  }

  const handleSelectRepository = (repo: TypesRepositoryInfo) => {
    setUrl(repo.clone_url || repo.html_url || '')
    setName(repo.name || '')
    setShowBrowser(false)
  }

  const isSubmitDisabled = !url.trim() || isCreating || (type === TypesExternalRepositoryType.ExternalRepositoryTypeADO && (!organizationUrl.trim() || !token.trim()))

  const canBrowse = token.trim() && (
    type === TypesExternalRepositoryType.ExternalRepositoryTypeGitHub ||
    type === TypesExternalRepositoryType.ExternalRepositoryTypeGitLab ||
    (type === TypesExternalRepositoryType.ExternalRepositoryTypeADO && organizationUrl.trim())
  )

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Link External Repository</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Link an existing repository from GitHub, GitLab, or Azure DevOps to enable AI collaboration.
          </Typography>

          <ExternalRepoForm
            url={url}
            onUrlChange={setUrl}
            name={name}
            onNameChange={setName}
            type={type}
            onTypeChange={setType}
            username={username}
            onUsernameChange={setUsername}
            password={password}
            onPasswordChange={setPassword}
            organizationUrl={organizationUrl}
            onOrganizationUrlChange={setOrganizationUrl}
            token={token}
            onTokenChange={setToken}
            gitlabBaseUrl={gitlabBaseUrl}
            onGitlabBaseUrlChange={setGitlabBaseUrl}
            size="medium"
          />

          {/* Browse Repositories Button */}
          {type !== TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket && (
            <Box>
              <Divider sx={{ my: 1 }} />
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
                <Button
                  variant="outlined"
                  startIcon={browseRemoteMutation.isPending ? <CircularProgress size={16} /> : <FolderSearch size={18} />}
                  onClick={handleBrowseRepositories}
                  disabled={!canBrowse || browseRemoteMutation.isPending}
                  size="small"
                >
                  Browse Repositories
                </Button>
                {!canBrowse && token.trim() === '' && (
                  <Typography variant="caption" color="text.secondary">
                    Enter a Personal Access Token to browse repositories
                  </Typography>
                )}
              </Box>

              {browseError && (
                <Alert severity="error" sx={{ mt: 2 }} onClose={() => setBrowseError(null)}>
                  {browseError.message}
                </Alert>
              )}

              <Collapse in={showBrowser}>
                <Box sx={{ mt: 2 }}>
                  <RepositoryBrowser
                    repositories={repositories}
                    isLoading={browseRemoteMutation.isPending}
                    error={browseError}
                    onSelect={handleSelectRepository}
                  />
                </Box>
              </Collapse>
            </Box>
          )}

          <Tooltip
            title={
              koditIndexing
                ? 'Code Intelligence enabled: Kodit will index this external repository to provide code snippets and architectural summaries via MCP server.'
                : 'Code Intelligence disabled: Repository will not be indexed by Kodit.'
            }
            arrow
          >
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
                    Code Intelligence
                  </Typography>
                </Box>
              }
            />
          </Tooltip>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button
          onClick={handleSubmit}
          color="secondary"
          variant="contained"
          disabled={isSubmitDisabled}
        >
          {isCreating ? <CircularProgress size={20} /> : 'Link Repository'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default LinkExternalRepositoryDialog
