import React, { FC } from 'react'
import {
  TextField,
  Stack,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Box,
  Typography,
  Link,
} from '@mui/material'
import { TypesExternalRepositoryType } from '../../../api/api'

interface ExternalRepoFormProps {
  url: string
  onUrlChange: (url: string) => void
  name: string
  onNameChange: (name: string) => void
  type: TypesExternalRepositoryType
  onTypeChange: (type: TypesExternalRepositoryType) => void
  username: string
  onUsernameChange: (username: string) => void
  password: string
  onPasswordChange: (password: string) => void
  organizationUrl?: string
  onOrganizationUrlChange?: (url: string) => void
  token?: string
  onTokenChange?: (token: string) => void
  size?: 'small' | 'medium'
}

/**
 * Reusable form fields for linking an external repository.
 * Used by both LinkExternalRepositoryDialog and CreateProjectDialog.
 */
const ExternalRepoForm: FC<ExternalRepoFormProps> = ({
  url,
  onUrlChange,
  name,
  onNameChange,
  type,
  onTypeChange,
  username,
  onUsernameChange,
  password,
  onPasswordChange,
  organizationUrl = '',
  onOrganizationUrlChange,
  token = '',
  onTokenChange,
  size = 'small',
}) => {
  return (
    <Stack spacing={2}>
      <FormControl fullWidth size={size}>
        <InputLabel>Repository Type</InputLabel>
        <Select
          value={type}
          label="Repository Type"
          onChange={(e) => onTypeChange(e.target.value as TypesExternalRepositoryType)}
        >
          <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeADO}>Azure DevOps</MenuItem>
          <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeGitHub}>GitHub (coming soon)</MenuItem>
          <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeGitLab}>GitLab (coming soon)</MenuItem>
          <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket}>Bitbucket (coming soon)</MenuItem>
        </Select>
      </FormControl>

      <TextField
        label="Repository URL"
        fullWidth
        size={size}
        value={url}
        onChange={(e) => onUrlChange(e.target.value)}
        placeholder="https://github.com/org/repo.git"
        required
      />

      <TextField
        label="Display Name in Helix (optional)"
        fullWidth
        size={size}
        value={name}
        onChange={(e) => onNameChange(e.target.value)}
        helperText="Name shown in Helix. Leave empty to use the repository name from the URL."
      />

      {type === TypesExternalRepositoryType.ExternalRepositoryTypeADO ? (
        <>
          <TextField
            label="Organization URL"
            fullWidth
            size={size}
            required
            value={organizationUrl}
            onChange={(e) => onOrganizationUrlChange?.(e.target.value)}
            placeholder="https://dev.azure.com/organization"
            helperText="Azure DevOps organization URL"
          />
          <TextField
            label="Personal Access Token"
            fullWidth
            size={size}
            required
            type="password"
            value={token}
            onChange={(e) => onTokenChange?.(e.target.value)}
            placeholder="Personal Access Token"
            helperText={
              <Box>
                <Typography variant="caption" component="span">
                  Personal Access Token for Azure DevOps authentication.{' '}
                </Typography>
                <Link
                  href="https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate"
                  target="_blank"
                  rel="noopener noreferrer"
                  variant="caption"
                >
                  Learn how to create one
                </Link>
              </Box>
            }
          />
        </>
      ) : (
        <>
          <TextField
            label="Username (for private repos)"
            fullWidth
            size={size}
            value={username}
            onChange={(e) => onUsernameChange(e.target.value)}
          />
          <TextField
            label="Password/Token (for private repos)"
            fullWidth
            size={size}
            type="password"
            value={password}
            onChange={(e) => onPasswordChange(e.target.value)}
          />
        </>
      )}
    </Stack>
  )
}

export default ExternalRepoForm
