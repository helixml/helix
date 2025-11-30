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
} from '@mui/material'
import { Brain } from 'lucide-react'
import { TypesExternalRepositoryType } from '../../api/api'
import ExternalRepoForm from './forms/ExternalRepoForm'

interface LinkExternalRepositoryDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (url: string, name: string, type: 'github' | 'gitlab' | 'ado' | 'other', koditIndexing: boolean, username?: string, password?: string, organizationUrl?: string, token?: string) => Promise<void>
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
    await onSubmit(url, name, submitType, koditIndexing, username || undefined, password || undefined, organizationUrl || undefined, token || undefined)
  }

  const isSubmitDisabled = !url.trim() || isCreating || (type === TypesExternalRepositoryType.ExternalRepositoryTypeADO && (!organizationUrl.trim() || !token.trim()))

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
            size="medium"
          />

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
