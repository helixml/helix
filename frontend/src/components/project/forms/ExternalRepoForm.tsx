import React, { FC, useCallback } from 'react'
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
 * Parse an Azure DevOps URL to extract organization URL and repository name.
 * Supports formats:
 * - https://dev.azure.com/{org}/{project}/_git/{repo}
 * - https://{org}.visualstudio.com/{project}/_git/{repo}
 */
function parseAdoUrl(url: string): { orgUrl: string; repoName: string } | null {
  // Try dev.azure.com format: https://dev.azure.com/{org}/{project}/_git/{repo}
  const devAzureMatch = url.match(/^(https:\/\/dev\.azure\.com\/[^/]+)\/[^/]+\/_git\/([^/]+)/i)
  if (devAzureMatch) {
    return {
      orgUrl: devAzureMatch[1],
      repoName: devAzureMatch[2],
    }
  }

  // Try visualstudio.com format: https://{org}.visualstudio.com/{project}/_git/{repo}
  const vsMatch = url.match(/^(https:\/\/[^.]+\.visualstudio\.com)\/[^/]+\/_git\/([^/]+)/i)
  if (vsMatch) {
    return {
      orgUrl: vsMatch[1],
      repoName: vsMatch[2],
    }
  }

  return null
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
  // Handle URL change with auto-fill for ADO repos
  const handleUrlChange = useCallback((newUrl: string) => {
    onUrlChange(newUrl)

    // Auto-fill org URL and display name for ADO repos
    if (type === TypesExternalRepositoryType.ExternalRepositoryTypeADO) {
      const parsed = parseAdoUrl(newUrl)
      if (parsed) {
        // Only auto-fill if the fields are empty (don't overwrite user input)
        if (!organizationUrl && onOrganizationUrlChange) {
          onOrganizationUrlChange(parsed.orgUrl)
        }
        if (!name) {
          onNameChange(parsed.repoName)
        }
      }
    }
  }, [type, organizationUrl, name, onUrlChange, onOrganizationUrlChange, onNameChange])

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
        onChange={(e) => handleUrlChange(e.target.value)}
        placeholder={
          type === TypesExternalRepositoryType.ExternalRepositoryTypeADO
            ? "https://dev.azure.com/organization/project/_git/repository"
            : "https://github.com/org/repo.git"
        }
        helperText={
          type === TypesExternalRepositoryType.ExternalRepositoryTypeADO
            ? "Paste the full URL - organization and name will be auto-filled"
            : undefined
        }
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
