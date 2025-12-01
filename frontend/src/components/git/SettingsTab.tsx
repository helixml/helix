import React, { FC } from 'react'
import {
  Box,
  Typography,
  CircularProgress,
  Button,
  TextField,
  Stack,
  FormControlLabel,
  Switch,
  InputAdornment,
  Paper,
  Collapse,
  Divider,
  Alert,
  IconButton,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
} from '@mui/material'
import {
  GitBranch,
  ExternalLink,
  Brain,
  ChevronDown,
  Trash2,
  Eye,
  EyeOff,
} from 'lucide-react'
import type { GitRepository } from '../../services/gitRepositoryService'
import { TypesExternalRepositoryType } from '../../api/api'

interface SettingsTabProps {
  repository: GitRepository
  editName: string
  setEditName: (value: string) => void
  editDescription: string
  setEditDescription: (value: string) => void
  editDefaultBranch: string
  setEditDefaultBranch: (value: string) => void
  editKoditIndexing: boolean
  setEditKoditIndexing: (value: boolean) => void
  editExternalUrl: string
  setEditExternalUrl: (value: string) => void
  editExternalType: TypesExternalRepositoryType | undefined
  setEditExternalType: (value: TypesExternalRepositoryType | undefined) => void
  editUsername: string
  setEditUsername: (value: string) => void
  editPassword: string
  setEditPassword: (value: string) => void
  editOrganizationUrl: string
  setEditOrganizationUrl: (value: string) => void
  editPersonalAccessToken: string
  setEditPersonalAccessToken: (value: string) => void
  showPassword: boolean
  setShowPassword: (value: boolean) => void
  showPersonalAccessToken: boolean
  setShowPersonalAccessToken: (value: boolean) => void
  updating: boolean
  dangerZoneExpanded: boolean
  setDangerZoneExpanded: (value: boolean) => void
  onUpdateRepository: () => void
  onDeleteClick: () => void
}

const SettingsTab: FC<SettingsTabProps> = ({
  repository,
  editName,
  setEditName,
  editDescription,
  setEditDescription,
  editDefaultBranch,
  setEditDefaultBranch,
  editKoditIndexing,
  setEditKoditIndexing,
  editExternalUrl,
  setEditExternalUrl,
  editExternalType,
  setEditExternalType,
  editUsername,
  setEditUsername,
  editPassword,
  setEditPassword,
  editOrganizationUrl,
  setEditOrganizationUrl,
  editPersonalAccessToken,
  setEditPersonalAccessToken,
  showPassword,
  setShowPassword,
  showPersonalAccessToken,
  setShowPersonalAccessToken,
  updating,
  dangerZoneExpanded,
  setDangerZoneExpanded,
  onUpdateRepository,
  onDeleteClick,
}) => {
  const isAzureDevOps = editExternalType === TypesExternalRepositoryType.ExternalRepositoryTypeADO
  return (
    <Box sx={{ maxWidth: 800 }}>
      <Paper variant="outlined" sx={{ p: 4, borderRadius: 2 }}>
        <Typography variant="h6" sx={{ mb: 3, fontWeight: 600 }}>
          Repository Settings
        </Typography>

        <Stack spacing={3}>
          <TextField
            label="Repository Name"
            fullWidth
            value={editName || repository.name}
            onChange={(e) => setEditName(e.target.value)}
            helperText="The name of this repository"
          />

          <TextField
            label="Description"
            fullWidth
            multiline
            rows={3}
            value={editDescription || repository.description}
            onChange={(e) => setEditDescription(e.target.value)}
            helperText="A short description of what this repository contains"
          />

          <TextField
            label="Default Branch"
            fullWidth
            value={editDefaultBranch || repository.default_branch || ''}
            onChange={(e) => setEditDefaultBranch(e.target.value)}
            helperText="The default branch for this repository"
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <GitBranch size={16} style={{ color: 'currentColor', opacity: 0.6 }} />
                </InputAdornment>
              ),
            }}
          />

          <FormControlLabel
            disabled={!(repository.is_external || repository.external_url)}
            control={
              <Switch
                checked={editKoditIndexing !== undefined ? editKoditIndexing : (repository.kodit_indexing || false)}
                onChange={(e) => setEditKoditIndexing(e.target.checked)}
                color="primary"
              />
            }
            label={
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Brain size={18} />
                <Box>
                  <Typography variant="body2" sx={{ fontWeight: 500 }}>
                    Code Intelligence
                  </Typography>
                  <Typography variant="caption" color="text.secondary">
                    {repository.is_external || repository.external_url
                      ? 'Index this repository with Kodit for AI-powered code understanding'
                      : 'Code Intelligence is only available for external repositories'}
                  </Typography>
                </Box>
              </Box>
            }
          />

          <Divider />

          {(repository.is_external || repository.external_url) && (
            <>
              <Typography variant="h6" sx={{ mt: 2, mb: 1, fontWeight: 600 }}>
                External Repository Settings
              </Typography>

              <FormControl fullWidth>
                <InputLabel>External Repository Type</InputLabel>
                <Select
                  value={editExternalType || repository.external_type || ''}
                  onChange={(e) => setEditExternalType(e.target.value as TypesExternalRepositoryType)}
                  label="External Repository Type"
                >
                  <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeGitHub}>GitHub</MenuItem>
                  <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeGitLab}>GitLab</MenuItem>
                  <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeADO}>Azure DevOps</MenuItem>
                  <MenuItem value={TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket}>Bitbucket</MenuItem>
                </Select>
              </FormControl>

              <TextField
                label="External URL"
                fullWidth
                value={editExternalUrl || repository.external_url || ''}
                onChange={(e) => setEditExternalUrl(e.target.value)}
                helperText="Full URL to the external repository (e.g., https://github.com/org/repo)"
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <ExternalLink size={16} style={{ color: 'currentColor', opacity: 0.6 }} />
                    </InputAdornment>
                  ),
                }}
              />

              {isAzureDevOps && (
                <>
                  <TextField
                    label="Organization URL"
                    fullWidth
                    value={editOrganizationUrl || repository.azure_devops?.organization_url || ''}
                    onChange={(e) => setEditOrganizationUrl(e.target.value)}
                    helperText="Azure DevOps organization URL (e.g., https://dev.azure.com/your-org)"
                    InputProps={{
                      startAdornment: (
                        <InputAdornment position="start">
                          <ExternalLink size={16} style={{ color: 'currentColor', opacity: 0.6 }} />
                        </InputAdornment>
                      ),
                    }}
                  />

                  <TextField
                    label="Personal Access Token"
                    fullWidth
                    type={showPersonalAccessToken ? 'text' : 'password'}
                    value={editPersonalAccessToken}
                    onChange={(e) => setEditPersonalAccessToken(e.target.value)}
                    helperText={
                      repository.azure_devops?.personal_access_token
                        ? "Leave blank to keep current token"
                        : "Personal Access Token for Azure DevOps. Get yours from https://learn.microsoft.com/en-us/azure/devops/organizations/accounts/use-personal-access-tokens-to-authenticate?view=azure-devops&tabs=Windows"
                    }
                    InputProps={{
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            onClick={() => setShowPersonalAccessToken(!showPersonalAccessToken)}
                            edge="end"
                            size="small"
                          >
                            {showPersonalAccessToken ? <EyeOff size={16} /> : <Eye size={16} />}
                          </IconButton>
                        </InputAdornment>
                      ),
                    }}
                  />
                </>
              )}

              {!isAzureDevOps && (
                <>
                  <TextField
                    label="Username"
                    fullWidth
                    value={editUsername || repository.username || ''}
                    onChange={(e) => setEditUsername(e.target.value)}
                    helperText="Username for authenticating with the external repository"
                  />

                  <TextField
                    label="Password"
                    fullWidth
                    type={showPassword ? 'text' : 'password'}
                    value={editPassword}
                    onChange={(e) => setEditPassword(e.target.value)}
                    helperText={repository.password ? "Leave blank to keep current password" : "Password for authenticating with the external repository"}
                    InputProps={{
                      endAdornment: (
                        <InputAdornment position="end">
                          <IconButton
                            onClick={() => setShowPassword(!showPassword)}
                            edge="end"
                            size="small"
                          >
                            {showPassword ? <EyeOff size={16} /> : <Eye size={16} />}
                          </IconButton>
                        </InputAdornment>
                      ),
                    }}
                  />
                </>
              )}
            </>
          )}

          <Divider />

          <Box sx={{ display: 'flex', gap: 2 }}>
            <Box sx={{ flex: 1 }} />
            <Button
              color="secondary"
              onClick={onUpdateRepository}
              variant="contained"
              disabled={updating}
            >
              {updating ? <CircularProgress size={20} /> : 'Save Changes'}
            </Button>
          </Box>

          <Divider sx={{ my: 2 }} />

          <Box>
            <Box
              onClick={() => setDangerZoneExpanded(!dangerZoneExpanded)}
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                cursor: 'pointer',
                mb: dangerZoneExpanded ? 2 : 0,
                '&:hover': {
                  opacity: 0.8,
                },
              }}
            >
              <Typography variant="h6" sx={{ fontWeight: 600, color: 'error.main' }}>
                Danger Zone
              </Typography>
              <ChevronDown
                size={20}
                style={{
                  color: 'var(--mui-palette-error-main)',
                  transform: dangerZoneExpanded ? 'rotate(180deg)' : 'rotate(0deg)',
                  transition: 'transform 0.2s',
                }}
              />
            </Box>
            <Collapse in={dangerZoneExpanded}>
              <Box>
                <Alert severity="error" sx={{ mb: 2 }}>
                  Once you delete a repository, there is no going back. This action cannot be undone.
                </Alert>
                <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                  <Button
                    onClick={onDeleteClick}
                    variant="outlined"
                    color="error"
                    startIcon={<Trash2 size={16} />}
                  >
                    Delete Repository
                  </Button>
                </Box>
              </Box>
            </Collapse>
          </Box>
        </Stack>
      </Paper>
    </Box>
  )
}

export default SettingsTab

