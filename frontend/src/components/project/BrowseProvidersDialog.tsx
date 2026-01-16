import React, { FC, useState, useEffect } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Box,
  Typography,
  CircularProgress,
  Alert,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Divider,
  Avatar,
  Chip,
  TextField,
  InputAdornment,
  FormControlLabel,
  Switch,
} from '@mui/material'
import GitHubIcon from '@mui/icons-material/GitHub'
import { Search, Brain, ExternalLink, CheckCircle, Cloud, Key } from 'lucide-react'
import { SiGitlab, SiBitbucket } from 'react-icons/si'

import {
  useListOAuthConnections,
  useListOAuthProviders,
  useListOAuthConnectionRepositories,
} from '../../services/oauthProvidersService'
import {
  useGitProviderConnections,
  useCreateGitProviderConnection,
  useDeleteGitProviderConnection,
} from '../../services/gitProviderConnectionService'
import { TypesRepositoryInfo, TypesExternalRepositoryType } from '../../api/api'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'

interface BrowseProvidersDialogProps {
  open: boolean
  onClose: () => void
  onSelectRepository: (repo: TypesRepositoryInfo, providerType: string, oauthConnectionId?: string) => void
  isLinking?: boolean
  // If provided, shows warning that repo will be visible to org members
  organizationName?: string
}

type ProviderType = 'github' | 'gitlab' | 'azure-devops' | 'bitbucket'
type ViewMode = 'providers' | 'choose-method' | 'pat-entry' | 'browse-repos' | 'browse-pat-repos'

interface ProviderConfig {
  id: ProviderType
  name: string
  icon: React.ReactNode
  color: string
}

interface PatCredentials {
  pat: string
  username?: string // For Bitbucket
  orgUrl?: string
  gitlabBaseUrl?: string
  githubBaseUrl?: string
  bitbucketBaseUrl?: string
}

const PROVIDERS: ProviderConfig[] = [
  { id: 'github', name: 'GitHub', icon: <GitHubIcon />, color: '#f0f0f0' },
  { id: 'gitlab', name: 'GitLab', icon: <SiGitlab size={24} />, color: '#FC6D26' },
  { id: 'azure-devops', name: 'Azure DevOps', icon: <Cloud size={24} />, color: '#0078D7' },
  { id: 'bitbucket', name: 'Bitbucket', icon: <SiBitbucket size={24} />, color: '#0052CC' },
]

const BrowseProvidersDialog: FC<BrowseProvidersDialogProps> = ({
  open,
  onClose,
  onSelectRepository,
  isLinking = false,
  organizationName,
}) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const router = useRouter()
  const [viewMode, setViewMode] = useState<ViewMode>('providers')
  const [selectedProvider, setSelectedProvider] = useState<ProviderType | null>(null)
  const [selectedConnectionId, setSelectedConnectionId] = useState<string | null>(null)
  const [selectedPatConnectionId, setSelectedPatConnectionId] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [koditIndexing, setKoditIndexing] = useState(true)
  const [selectedRepo, setSelectedRepo] = useState<TypesRepositoryInfo | null>(null)

  // PAT entry state
  const [pat, setPat] = useState('')
  const [orgUrl, setOrgUrl] = useState('') // For Azure DevOps
  const [gitlabBaseUrl, setGitlabBaseUrl] = useState('') // For self-hosted GitLab
  const [githubBaseUrl, setGithubBaseUrl] = useState('') // For GitHub Enterprise
  const [bitbucketUsername, setBitbucketUsername] = useState('') // For Bitbucket
  const [bitbucketBaseUrl, setBitbucketBaseUrl] = useState('') // For Bitbucket Server
  const [patCredentials, setPatCredentials] = useState<PatCredentials | null>(null)
  const [saveConnection, setSaveConnection] = useState(true) // Save PAT for future use

  // PAT-based repos
  const [patRepos, setPatRepos] = useState<TypesRepositoryInfo[]>([])
  const [patReposLoading, setPatReposLoading] = useState(false)
  const [patReposError, setPatReposError] = useState<string | null>(null)

  const { data: oauthConnections, isLoading: oauthConnectionsLoading } = useListOAuthConnections()
  const { data: providers } = useListOAuthProviders()
  const { data: patConnections, isLoading: patConnectionsLoading } = useGitProviderConnections()
  const createPatConnection = useCreateGitProviderConnection()
  const deletePatConnection = useDeleteGitProviderConnection()

  const connectionsLoading = oauthConnectionsLoading || patConnectionsLoading

  // Get repositories for selected OAuth connection
  const { data: repositoriesData, isLoading: reposLoading, error: reposError } =
    useListOAuthConnectionRepositories(selectedConnectionId || '')

  const repositories = repositoriesData?.repositories || []

  // Reset state when dialog closes
  useEffect(() => {
    if (!open) {
      setViewMode('providers')
      setSelectedProvider(null)
      setSelectedConnectionId(null)
      setSelectedPatConnectionId(null)
      setSearchQuery('')
      setSelectedRepo(null)
      setKoditIndexing(true)
      setPat('')
      setOrgUrl('')
      setGitlabBaseUrl('')
      setGithubBaseUrl('')
      setBitbucketUsername('')
      setBitbucketBaseUrl('')
      setPatCredentials(null)
      setPatRepos([])
      setPatReposError(null)
      setSaveConnection(true)
    }
  }, [open])

  // Find OAuth connection for a provider type
  const getOAuthConnectionForProvider = (providerType: ProviderType) => {
    return oauthConnections?.find(conn => {
      const connType = conn.provider?.type?.toLowerCase()
      if (providerType === 'azure-devops') {
        return connType === 'azure-devops' || connType === 'ado'
      }
      if (providerType === 'bitbucket') {
        return connType === 'bitbucket'
      }
      return connType === providerType
    })
  }

  // Find PAT connection for a provider type
  const getPatConnectionForProvider = (providerType: ProviderType) => {
    return patConnections?.find(conn => {
      const connType = conn.provider_type?.toLowerCase()
      if (providerType === 'azure-devops') {
        return connType === 'azure-devops' || connType === 'ado'
      }
      if (providerType === 'bitbucket') {
        return connType === 'bitbucket'
      }
      return connType === providerType
    })
  }

  // Get any connection (OAuth or PAT) for a provider type
  const getConnectionForProvider = (providerType: ProviderType) => {
    return getOAuthConnectionForProvider(providerType) || getPatConnectionForProvider(providerType)
  }

  // Find provider ID for OAuth flow
  const getProviderIdForType = (providerType: ProviderType) => {
    const provider = providers?.find(p => {
      const pType = p.type?.toLowerCase()
      if (providerType === 'azure-devops') {
        return pType === 'azure-devops' || pType === 'ado'
      }
      return pType === providerType
    })
    return provider?.id
  }

  // Map frontend provider type to API provider type
  const mapProviderType = (provider: ProviderType): TypesExternalRepositoryType => {
    switch (provider) {
      case 'github':
        return 'github' as TypesExternalRepositoryType
      case 'gitlab':
        return 'gitlab' as TypesExternalRepositoryType
      case 'azure-devops':
        return 'ado' as TypesExternalRepositoryType
      case 'bitbucket':
        return 'bitbucket' as TypesExternalRepositoryType
    }
  }

  // Fetch repos using PAT via backend API
  const fetchReposWithPat = async (provider: ProviderType, creds: PatCredentials) => {
    setPatReposLoading(true)
    setPatReposError(null)
    setPatRepos([])

    try {
      const apiClient = api.getApiClient()
      const response = await apiClient.v1GitBrowseRemoteCreate({
        provider_type: mapProviderType(provider),
        token: creds.pat,
        username: creds.username,
        organization_url: creds.orgUrl,
        base_url: creds.gitlabBaseUrl || creds.githubBaseUrl || creds.bitbucketBaseUrl,
      })

      const repos = response.data?.repositories || []
      setPatRepos(repos)
    } catch (err: any) {
      const message = err?.response?.data?.message || err?.message || 'Failed to fetch repositories'
      setPatReposError(typeof message === 'string' ? message : JSON.stringify(message))
    } finally {
      setPatReposLoading(false)
    }
  }

  // Fetch repos for a saved PAT connection
  const fetchReposForSavedConnection = async (connectionId: string) => {
    setPatReposLoading(true)
    setPatReposError(null)
    setPatRepos([])

    try {
      const apiClient = api.getApiClient()
      const response = await apiClient.v1GitProviderConnectionsRepositoriesDetail(connectionId)
      const repos = response.data?.repositories || []
      setPatRepos(repos)
    } catch (err: any) {
      const message = err?.response?.data?.message || err?.message || 'Failed to fetch repositories'
      setPatReposError(typeof message === 'string' ? message : JSON.stringify(message))
    } finally {
      setPatReposLoading(false)
    }
  }

  const handleProviderClick = (providerType: ProviderType) => {
    // Always show the choose-method screen so users can pick OAuth or PAT
    setSelectedProvider(providerType)
    setViewMode('choose-method')
  }

  // Handle choosing OAuth connection method
  const handleChooseOAuth = () => {
    if (!selectedProvider) return

    const oauthConnection = getOAuthConnectionForProvider(selectedProvider)
    if (oauthConnection) {
      // Already connected - browse repos
      setSelectedConnectionId(oauthConnection.id || null)
      setViewMode('browse-repos')
    } else {
      // Start OAuth flow
      const providerId = getProviderIdForType(selectedProvider)
      if (providerId) {
        sessionStorage.setItem('oauth_return_url', window.location.href)
        window.location.href = `/api/v1/oauth/flow/start/${providerId}`
      }
    }
  }

  // Handle choosing PAT connection method
  const handleChoosePat = () => {
    if (!selectedProvider) return

    const patConnection = getPatConnectionForProvider(selectedProvider)
    if (patConnection) {
      // Already have saved PAT - browse repos
      setSelectedPatConnectionId(patConnection.id || null)
      setViewMode('browse-pat-repos')
      fetchReposForSavedConnection(patConnection.id || '')
    } else {
      // Show PAT entry form
      setViewMode('pat-entry')
    }
  }

  const handlePatSubmit = async () => {
    if (!pat.trim() || !selectedProvider) return

    const creds: PatCredentials = {
      pat,
      username: selectedProvider === 'bitbucket' ? bitbucketUsername : undefined,
      orgUrl: selectedProvider === 'azure-devops' ? orgUrl : undefined,
      gitlabBaseUrl: selectedProvider === 'gitlab' ? gitlabBaseUrl : undefined,
      githubBaseUrl: selectedProvider === 'github' ? githubBaseUrl : undefined,
      bitbucketBaseUrl: selectedProvider === 'bitbucket' ? bitbucketBaseUrl : undefined,
    }
    setPatCredentials(creds)
    setViewMode('browse-pat-repos')

    // Fetch repos
    await fetchReposWithPat(selectedProvider, creds)

    // Save connection if requested
    if (saveConnection) {
      try {
        await createPatConnection.mutateAsync({
          provider_type: mapProviderType(selectedProvider) as any,
          token: pat,
          auth_username: creds.username,
          organization_url: creds.orgUrl,
          base_url: creds.gitlabBaseUrl || creds.githubBaseUrl || creds.bitbucketBaseUrl,
        })
        snackbar.success('Connection saved for future use')
      } catch (err) {
        // Don't fail the flow if saving fails
        console.error('Failed to save connection:', err)
      }
    }
  }

  const handleSelectRepo = () => {
    if (!selectedRepo || !selectedProvider) return

    // For PAT-based selection, include credentials
    if (patCredentials) {
      const providerWithCreds = JSON.stringify({
        type: selectedProvider,
        pat: patCredentials.pat,
        username: patCredentials.username,
        orgUrl: patCredentials.orgUrl,
        gitlabBaseUrl: patCredentials.gitlabBaseUrl,
        githubBaseUrl: patCredentials.githubBaseUrl,
        bitbucketBaseUrl: patCredentials.bitbucketBaseUrl,
      })
      onSelectRepository(selectedRepo, providerWithCreds)
    } else {
      // OAuth-based selection - pass connection ID
      console.log("XXX", selectedConnectionId)
      console.log("XXX", selectedProvider)
      console.log("XXX", selectedRepo)
      onSelectRepository(selectedRepo, selectedProvider, selectedConnectionId)
    }
  }

  const handleBack = () => {
    if (viewMode === 'browse-pat-repos' && patCredentials) {
      // Coming from PAT entry - go back to PAT entry
      setViewMode('pat-entry')
      setPatRepos([])
      setPatReposError(null)
      setSelectedRepo(null)
    } else if (viewMode === 'browse-pat-repos' || viewMode === 'browse-repos') {
      // Coming from repo browser - go back to choose-method
      setViewMode('choose-method')
      setSelectedConnectionId(null)
      setSelectedPatConnectionId(null)
      setSearchQuery('')
      setSelectedRepo(null)
      setPatRepos([])
      setPatReposError(null)
    } else if (viewMode === 'pat-entry') {
      // Coming from PAT entry - go back to choose-method
      setViewMode('choose-method')
      setPat('')
      setOrgUrl('')
      setGitlabBaseUrl('')
      setGithubBaseUrl('')
      setBitbucketUsername('')
      setBitbucketBaseUrl('')
    } else if (viewMode === 'choose-method') {
      // Coming from choose-method - go back to providers list
      setViewMode('providers')
      setSelectedProvider(null)
    } else {
      // Default: go back to providers list
      setViewMode('providers')
      setSelectedProvider(null)
      setSelectedConnectionId(null)
      setSelectedPatConnectionId(null)
      setSearchQuery('')
      setSelectedRepo(null)
      setPat('')
      setOrgUrl('')
      setGitlabBaseUrl('')
      setGithubBaseUrl('')
      setBitbucketUsername('')
      setBitbucketBaseUrl('')
      setPatCredentials(null)
      setPatRepos([])
      setPatReposError(null)
    }
  }

  // Get the right repo list based on mode
  const currentRepos = viewMode === 'browse-pat-repos' ? patRepos : repositories
  const currentLoading = viewMode === 'browse-pat-repos' ? patReposLoading : reposLoading
  const currentError = viewMode === 'browse-pat-repos' ? patReposError : (reposError instanceof Error ? reposError.message : reposError ? 'Failed to load repositories' : null)

  // Filter repositories by search query
  const filteredRepos = currentRepos.filter(repo => {
    if (!searchQuery) return true
    const query = searchQuery.toLowerCase()
    return (
      repo.name?.toLowerCase().includes(query) ||
      repo.full_name?.toLowerCase().includes(query) ||
      repo.description?.toLowerCase().includes(query)
    )
  })

  const currentProvider = PROVIDERS.find(p => p.id === selectedProvider)

  // Provider selection view
  if (viewMode === 'providers') {
    return (
      <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
        <DialogTitle>Connect & Browse Repositories</DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Select a provider to browse your repositories.
          </Typography>

          {connectionsLoading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
              <CircularProgress />
            </Box>
          ) : (
            <List>
              {PROVIDERS.map((provider, index) => {
                const oauthConnection = getOAuthConnectionForProvider(provider.id)
                const patConnection = getPatConnectionForProvider(provider.id)
                const isConnected = !!(oauthConnection || patConnection)
                const hasOAuth = !!getProviderIdForType(provider.id)

                // Get display name for connection
                const getConnectionDisplayName = () => {
                  if (oauthConnection) {
                    return oauthConnection.profile?.name || oauthConnection.profile?.email || 'user'
                  }
                  if (patConnection) {
                    return patConnection.username || patConnection.email || patConnection.name || 'user'
                  }
                  return 'user'
                }

                return (
                  <React.Fragment key={provider.id}>
                    {index > 0 && <Divider />}
                    <ListItem disablePadding>
                      <ListItemButton onClick={() => handleProviderClick(provider.id)}>
                        <ListItemIcon sx={{ color: provider.color }}>
                          {provider.icon}
                        </ListItemIcon>
                        <ListItemText
                          primary={provider.name}
                          secondary={
                            isConnected
                              ? `Connected as ${getConnectionDisplayName()}`
                              : hasOAuth
                                ? 'Click to connect via OAuth'
                                : 'Click to enter access token'
                          }
                        />
                        {isConnected ? (
                          <Chip
                            icon={<CheckCircle size={14} />}
                            label="Browse"
                            size="small"
                            color="success"
                            variant="outlined"
                          />
                        ) : hasOAuth ? (
                          <Chip
                            icon={<ExternalLink size={14} />}
                            label="Connect"
                            size="small"
                            variant="outlined"
                          />
                        ) : (
                          <Chip
                            icon={<Key size={14} />}
                            label="Enter Token"
                            size="small"
                            variant="outlined"
                          />
                        )}
                      </ListItemButton>
                    </ListItem>
                  </React.Fragment>
                )
              })}
            </List>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={onClose}>Cancel</Button>
        </DialogActions>
      </Dialog>
    )
  }

  // Choose connection method view
  if (viewMode === 'choose-method') {
    const oauthConnection = selectedProvider ? getOAuthConnectionForProvider(selectedProvider) : null
    const patConnection = selectedProvider ? getPatConnectionForProvider(selectedProvider) : null
    const hasOAuth = selectedProvider ? !!getProviderIdForType(selectedProvider) : false

    return (
      <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box sx={{ color: currentProvider?.color }}>{currentProvider?.icon}</Box>
          Connect to {currentProvider?.name}
        </DialogTitle>
        <DialogContent>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
            Choose how you want to connect to {currentProvider?.name}.
          </Typography>

          <List>
            {/* OAuth option */}
            <ListItem disablePadding sx={{ mb: 1 }}>
              <ListItemButton
                onClick={handleChooseOAuth}
                disabled={!hasOAuth}
                sx={{
                  border: 1,
                  borderColor: 'divider',
                  borderRadius: 1,
                  opacity: hasOAuth ? 1 : 0.5,
                }}
              >
                <ListItemIcon>
                  <ExternalLink size={24} />
                </ListItemIcon>
                <ListItemText
                  primary="Connect via OAuth"
                  secondary={
                    oauthConnection
                      ? `Connected as ${oauthConnection.profile?.name || oauthConnection.profile?.email || 'user'}`
                      : hasOAuth
                        ? 'Securely authorize access through your browser'
                        : 'OAuth not configured by administrator'
                  }
                />
                {oauthConnection && (
                  <Chip
                    icon={<CheckCircle size={14} />}
                    label="Connected"
                    size="small"
                    color="success"
                    variant="outlined"
                  />
                )}
              </ListItemButton>
            </ListItem>

            {/* PAT option */}
            <ListItem disablePadding>
              <ListItemButton
                onClick={handleChoosePat}
                sx={{
                  border: 1,
                  borderColor: 'divider',
                  borderRadius: 1,
                }}
              >
                <ListItemIcon>
                  <Key size={24} />
                </ListItemIcon>
                <ListItemText
                  primary="Use Personal Access Token"
                  secondary={
                    patConnection
                      ? `Saved token for ${patConnection.username || patConnection.email || 'user'}`
                      : 'Enter a token manually for authentication'
                  }
                />
                {patConnection && (
                  <Chip
                    icon={<CheckCircle size={14} />}
                    label="Saved"
                    size="small"
                    color="success"
                    variant="outlined"
                  />
                )}
              </ListItemButton>
            </ListItem>
          </List>

          {/* Help text when OAuth is not available */}
          {!hasOAuth && (
            <Alert severity="info" sx={{ mt: 2 }}>
              OAuth is not configured for {currentProvider?.name}.
              {account.admin ? (
                <>
                  {' '}You can{' '}
                  <Button
                    size="small"
                    onClick={() => {
                      onClose()
                      router.navigate('dashboard', { tab: 'oauth_providers' })
                    }}
                    sx={{ textTransform: 'none', p: 0, minWidth: 'auto', verticalAlign: 'baseline' }}
                  >
                    configure OAuth providers
                  </Button>
                  {' '}in the admin dashboard, or use a Personal Access Token.
                </>
              ) : (
                ' Contact your administrator to enable OAuth integration, or use a Personal Access Token.'
              )}
            </Alert>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleBack}>Back</Button>
          <Button onClick={onClose}>Cancel</Button>
        </DialogActions>
      </Dialog>
    )
  }

  // PAT entry view
  if (viewMode === 'pat-entry') {
    return (
      <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
        <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Box sx={{ color: currentProvider?.color }}>{currentProvider?.icon}</Box>
          Connect to {currentProvider?.name}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2, mt: 1 }}>
            <Typography variant="body2" color="text.secondary">
              Enter your personal access token to browse and link repositories.
            </Typography>

            {selectedProvider === 'azure-devops' && (
              <TextField
                label="Organization URL"
                fullWidth
                value={orgUrl}
                onChange={(e) => setOrgUrl(e.target.value)}
                placeholder="https://dev.azure.com/your-org"
                helperText="Your Azure DevOps organization URL"
              />
            )}

            {selectedProvider === 'github' && (
              <TextField
                label="GitHub Enterprise URL (optional)"
                fullWidth
                value={githubBaseUrl}
                onChange={(e) => setGithubBaseUrl(e.target.value)}
                placeholder="https://github.mycompany.com"
                helperText="Leave empty for github.com, or enter your GitHub Enterprise URL"
              />
            )}

            {selectedProvider === 'gitlab' && (
              <TextField
                label="GitLab Base URL (optional)"
                fullWidth
                value={gitlabBaseUrl}
                onChange={(e) => setGitlabBaseUrl(e.target.value)}
                placeholder="https://gitlab.com"
                helperText="Leave empty for gitlab.com, or enter your self-hosted GitLab URL"
              />
            )}

            {selectedProvider === 'bitbucket' && (
              <>
                <TextField
                  label="Username"
                  fullWidth
                  value={bitbucketUsername}
                  onChange={(e) => setBitbucketUsername(e.target.value)}
                  placeholder="your-bitbucket-username"
                  helperText="Your Bitbucket username (required for authentication)"
                />
                <TextField
                  label="Bitbucket Server URL (optional)"
                  fullWidth
                  value={bitbucketBaseUrl}
                  onChange={(e) => setBitbucketBaseUrl(e.target.value)}
                  placeholder="https://bitbucket.mycompany.com"
                  helperText="Leave empty for bitbucket.org, or enter your Bitbucket Server URL"
                />
              </>
            )}

            <TextField
              label={selectedProvider === 'bitbucket' ? 'App Password' : 'Personal Access Token'}
              fullWidth
              type="password"
              value={pat}
              onChange={(e) => setPat(e.target.value)}
              helperText={
                selectedProvider === 'github'
                  ? 'Create a token at GitHub → Settings → Developer settings → Personal access tokens'
                  : selectedProvider === 'gitlab'
                    ? 'Create a token at GitLab → Preferences → Access Tokens'
                    : selectedProvider === 'bitbucket'
                      ? 'Create an App Password at Bitbucket → Personal settings → App passwords'
                      : 'Create a token at Azure DevOps → User settings → Personal access tokens'
              }
            />

            <FormControlLabel
              control={
                <Switch
                  checked={saveConnection}
                  onChange={(e) => setSaveConnection(e.target.checked)}
                  color="primary"
                  size="small"
                />
              }
              label={
                <Typography variant="body2">
                  Save connection for future use (encrypted)
                </Typography>
              }
            />
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleBack}>Back</Button>
          <Button onClick={onClose}>Cancel</Button>
          <Button
            variant="contained"
            color="secondary"
            onClick={handlePatSubmit}
            disabled={
              !pat.trim() ||
              (selectedProvider === 'azure-devops' && !orgUrl.trim()) ||
              (selectedProvider === 'bitbucket' && !bitbucketUsername.trim())
            }
          >
            Browse Repositories
          </Button>
        </DialogActions>
      </Dialog>
    )
  }

  // Repository browser view (OAuth or PAT)
  return (
    <Dialog open={open} onClose={onClose} maxWidth="md" fullWidth>
      <DialogTitle sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
        <Box sx={{ color: currentProvider?.color }}>{currentProvider?.icon}</Box>
        Browse {currentProvider?.name} Repositories
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mb: 2 }}>
          <TextField
            fullWidth
            size="small"
            placeholder="Search repositories..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <Search size={18} />
                </InputAdornment>
              ),
            }}
          />
        </Box>

        {currentError && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {currentError}
          </Alert>
        )}

        {currentLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress />
          </Box>
        ) : filteredRepos.length === 0 ? (
          <Box sx={{ textAlign: 'center', py: 4 }}>
            <Typography color="text.secondary">
              {searchQuery ? 'No repositories match your search' : 'No repositories found'}
            </Typography>
          </Box>
        ) : (
          <List sx={{ maxHeight: 400, overflow: 'auto' }}>
            {filteredRepos.map((repo, index) => (
              <React.Fragment key={repo.id || repo.full_name || index}>
                {index > 0 && <Divider />}
                <ListItem disablePadding>
                  <ListItemButton
                    selected={selectedRepo?.id === repo.id || selectedRepo?.full_name === repo.full_name}
                    onClick={() => setSelectedRepo(repo)}
                  >
                    <ListItemIcon>
                      <Avatar sx={{ width: 32, height: 32, bgcolor: 'action.hover' }}>
                        {repo.name?.[0]?.toUpperCase() || 'R'}
                      </Avatar>
                    </ListItemIcon>
                    <ListItemText
                      primary={repo.full_name || repo.name}
                      secondary={repo.description || 'No description'}
                      secondaryTypographyProps={{ noWrap: true }}
                    />
                    {repo.private && (
                      <Chip label="Private" size="small" variant="outlined" sx={{ ml: 1 }} />
                    )}
                  </ListItemButton>
                </ListItem>
              </React.Fragment>
            ))}
          </List>
        )}

        {selectedRepo && (
          <Box sx={{ mt: 2, p: 2, bgcolor: 'action.hover', borderRadius: 1 }}>
            <Typography variant="subtitle2" gutterBottom>
              Selected: {selectedRepo.full_name || selectedRepo.name}
            </Typography>
            <FormControlLabel
              control={
                <Switch
                  checked={koditIndexing}
                  onChange={(e) => setKoditIndexing(e.target.checked)}
                  color="primary"
                  size="small"
                />
              }
              label={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Brain size={16} />
                  <Typography variant="body2">Enable Code Intelligence</Typography>
                </Box>
              }
            />
            {organizationName && (
              <Alert severity="info" sx={{ mt: 2 }}>
                This repository will be accessible to all members of <strong>{organizationName}</strong>.
              </Alert>
            )}
          </Box>
        )}
      </DialogContent>
      <DialogActions>
        <Button onClick={handleBack}>Back</Button>
        <Button onClick={onClose}>Cancel</Button>
        <Button
          variant="contained"
          color="secondary"
          onClick={handleSelectRepo}
          disabled={!selectedRepo || isLinking}
        >
          {isLinking ? <CircularProgress size={20} /> : 'Link Repository'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default BrowseProvidersDialog
