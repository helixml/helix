import React, { FC, useState, useEffect } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Stack,
  FormControlLabel,
  Switch,
  Box,
  Typography,
  CircularProgress,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Link,
  Tooltip,
} from '@mui/material'
import { Brain } from 'lucide-react'

interface LinkExternalRepositoryDialogProps {
  open: boolean
  onClose: () => void
  onSubmit: (url: string, name: string, type: 'github' | 'gitlab' | 'ado' | 'other', koditIndexing: boolean, username?: string, password?: string) => Promise<void>
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
  const [type, setType] = useState<'github' | 'gitlab' | 'ado' | 'other'>('github')
  const [koditIndexing, setKoditIndexing] = useState(true)
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')

  // Reset form when dialog closes
  useEffect(() => {
    if (!open) {
      setUrl('')
      setName('')
      setType('github')
      setKoditIndexing(true)
      setUsername('')
      setPassword('')
    }
  }, [open])

  const handleSubmit = async () => {
    await onSubmit(url, name, type, koditIndexing, username || undefined, password || undefined)
  }

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Link External Repository</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ mt: 1 }}>
          <Typography variant="body2" color="text.secondary">
            Link an existing repository from GitHub, GitLab, or Azure DevOps to enable AI collaboration.
          </Typography>

          <FormControl fullWidth required>
            <InputLabel>Repository Type</InputLabel>
            <Select
              value={type}
              onChange={(e) => setType(e.target.value as 'github' | 'gitlab' | 'ado' | 'other')}
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
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            placeholder="https://github.com/org/repo.git"
            helperText="Full URL to the external repository"
          />

          <TextField
            label="Username"
            fullWidth
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="Repository username"
            helperText="Username for repository authentication (optional)"
          />

          <TextField
            label="Password"
            fullWidth
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            placeholder="Password or Personal Access Token"
            helperText={
              type === 'ado' ? (
                <Box>
                  <Typography variant="caption" component="span">
                    Personal Access Token for Azure DevOps authentication.{' '}
                  </Typography>
                  <Link
                    href="https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate?view=azure-devops&tabs=Windows"
                    target="_blank"
                    rel="noopener noreferrer"
                    variant="caption"
                  >
                    Learn how to create one
                  </Link>
                </Box>
              ) : (
                'Password or Personal Access Token for the repository (optional)'
              )
            }
          />

          <TextField
            label="Repository Name (Optional)"
            fullWidth
            value={name}
            onChange={(e) => setName(e.target.value)}
            helperText="Display name (auto-extracted from URL if empty)"
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
        <Button onClick={onClose}>Cancel</Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={!url.trim() || isCreating}
        >
          {isCreating ? <CircularProgress size={20} /> : 'Link Repository'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default LinkExternalRepositoryDialog
