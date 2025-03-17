import React, { useState, useEffect } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  CircularProgress,
  IconButton,
  Typography,
  Paper,
  Chip,
  Grid,
  Divider,
  CardHeader,
  Avatar,
  CardActions,
  Tooltip,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  SvgIcon,
} from '@mui/material'
import DeleteIcon from '@mui/icons-material/Delete'
import RefreshIcon from '@mui/icons-material/Refresh'
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline'
import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import GitHubIcon from '@mui/icons-material/GitHub'
import GoogleIcon from '@mui/icons-material/Google'
import CloudIcon from '@mui/icons-material/Cloud'
import AppleIcon from '@mui/icons-material/Apple'
import CodeIcon from '@mui/icons-material/Code'
import LockIcon from '@mui/icons-material/Lock'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { formatDate } from '../../utils/format'

// Microsoft Logo SVG
const MicrosoftLogo = (props: any) => (
  <SvgIcon {...props} viewBox="0 0 23 23">
    <path fill="#f25022" d="M1 1h10v10H1z"/>
    <path fill="#00a4ef" d="M1 12h10v10H1z"/>
    <path fill="#7fba00" d="M12 1h10v10H12z"/>
    <path fill="#ffb900" d="M12 12h10v10H12z"/>
  </SvgIcon>
);

// Slack Logo SVG
const SlackLogo = (props: any) => (
  <SvgIcon {...props} viewBox="0 0 24 24">
    <path fill="#E01E5A" d="M5.042 15.165a2.528 2.528 0 0 1-2.52 2.523A2.528 2.528 0 0 1 0 15.165a2.527 2.527 0 0 1 2.522-2.52h2.52v2.52zM6.313 15.165a2.527 2.527 0 0 1 2.521-2.52 2.527 2.527 0 0 1 2.521 2.52v6.313A2.528 2.528 0 0 1 8.834 24a2.528 2.528 0 0 1-2.521-2.522v-6.313zM8.834 5.042a2.528 2.528 0 0 1-2.521-2.52A2.528 2.528 0 0 1 8.834 0a2.528 2.528 0 0 1 2.521 2.522v2.52H8.834zM8.834 6.313a2.528 2.528 0 0 1 2.521 2.521 2.528 2.528 0 0 1-2.521 2.521H2.522A2.528 2.528 0 0 1 0 8.834a2.528 2.528 0 0 1 2.522-2.521h6.312zM18.956 8.834a2.528 2.528 0 0 1 2.522-2.521A2.528 2.528 0 0 1 24 8.834a2.528 2.528 0 0 1-2.522 2.521h-2.522V8.834zM17.688 8.834a2.528 2.528 0 0 1-2.523 2.521 2.527 2.527 0 0 1-2.52-2.521V2.522A2.527 2.527 0 0 1 15.165 0a2.528 2.528 0 0 1 2.523 2.522v6.312zM15.165 18.956a2.528 2.528 0 0 1 2.523 2.522A2.528 2.528 0 0 1 15.165 24a2.527 2.527 0 0 1-2.52-2.522v-2.522h2.52zM15.165 17.688a2.527 2.527 0 0 1-2.52-2.523 2.526 2.526 0 0 1 2.52-2.52h6.313A2.527 2.527 0 0 1 24 15.165a2.528 2.528 0 0 1-2.522 2.523h-6.313z"/>
  </SvgIcon>
);

// LinkedIn Logo SVG
const LinkedInLogo = (props: any) => (
  <SvgIcon {...props} viewBox="0 0 24 24">
    <path fill="#0A66C2" d="M20.447 20.452h-3.554v-5.569c0-1.328-.027-3.037-1.852-3.037-1.853 0-2.136 1.445-2.136 2.939v5.667H9.351V9h3.414v1.561h.046c.477-.9 1.637-1.85 3.37-1.85 3.601 0 4.267 2.37 4.267 5.455v6.286zM5.337 7.433c-1.144 0-2.063-.926-2.063-2.065 0-1.138.92-2.063 2.063-2.063 1.14 0 2.064.925 2.064 2.063 0 1.139-.925 2.065-2.064 2.065zm1.782 13.019H3.555V9h3.564v11.452zM22.225 0H1.771C.792 0 0 .774 0 1.729v20.542C0 23.227.792 24 1.771 24h20.454C23.2 24 24 23.227 24 22.271V1.729C24 .774 23.2 0 22.225 0z"/>
  </SvgIcon>
);

// Atlassian Logo SVG
const AtlassianLogo = (props: any) => (
  <SvgIcon {...props} viewBox="0 0 24 24">
    <path fill="#0052CC" d="M7.12 11.084c-.294-.486-.891-.642-1.37-.347-.479.294-.628.891-.347 1.37l4.723 8.25c.294.487.89.636 1.37.347.48-.294.629-.89.347-1.377l-4.723-8.243z"/>
  </SvgIcon>
);

interface OAuthProvider {
  id: string
  name: string
  description: string
  type: string
  version: string
  enabled: boolean
}

interface OAuthConnection {
  id: string
  createdAt: string
  updatedAt: string
  userId: string
  providerId: string
  expiresAt: string
  providerUserId: string
  provider: OAuthProvider
  profile: {
    id: string
    name: string
    email: string
    displayName: string
    avatarUrl: string
  }
}

// All known provider types with their display names
const PROVIDER_TYPES = {
  github: 'GitHub',
  google: 'Google',
  microsoft: 'Microsoft',
  atlassian: 'Atlassian',
  slack: 'Slack',
  linkedin: 'LinkedIn',
  facebook: 'Facebook',
  twitter: 'Twitter',
  apple: 'Apple',
  custom: 'Custom',
}

// Icons for all known provider types
const PROVIDER_ICONS: Record<string, React.ReactNode> = {
  github: <GitHubIcon sx={{ fontSize: 30 }} />,
  google: <GoogleIcon sx={{ fontSize: 30 }} />,
  microsoft: <MicrosoftLogo sx={{ fontSize: 30 }} />,
  atlassian: <AtlassianLogo sx={{ fontSize: 30 }} />,
  slack: <SlackLogo sx={{ fontSize: 30 }} />,
  linkedin: <LinkedInLogo sx={{ fontSize: 30 }} />,
  facebook: <SvgIcon sx={{ fontSize: 30 }} viewBox="0 0 24 24">
    <path fill="#1877F2" d="M24 12.073c0-5.8-4.698-10.5-10.497-10.5s-10.5 4.7-10.5 10.5c0 5.237 3.8 9.585 8.8 10.38v-7.344H8.262v-3.036h3.542V9.458c0-3.494 2.084-5.426 5.265-5.426 1.526 0 3.124.273 3.124.273v3.427h-1.76c-1.732 0-2.273 1.076-2.273 2.18v2.625h3.868l-.618 3.036h-3.25v7.344c5-0.795 8.8-5.143 8.8-10.38z"/>
  </SvgIcon>,
  twitter: <SvgIcon sx={{ fontSize: 30 }} viewBox="0 0 24 24">
    <path fill="#1DA1F2" d="M23.953 4.57a10 10 0 01-2.825.775 4.958 4.958 0 002.163-2.723c-.951.555-2.005.959-3.127 1.184a4.92 4.92 0 00-8.384 4.482C7.69 8.095 4.067 6.13 1.64 3.162a4.822 4.822 0 00-.666 2.475c0 1.71.87 3.213 2.188 4.096a4.904 4.904 0 01-2.228-.616v.06a4.923 4.923 0 003.946 4.827 4.996 4.996 0 01-2.212.085 4.936 4.936 0 004.604 3.417 9.867 9.867 0 01-6.102 2.105c-.39 0-.779-.023-1.17-.067a13.995 13.995 0 007.557 2.209c9.053 0 13.998-7.496 13.998-13.985 0-.21 0-.42-.015-.63A9.935 9.935 0 0024 4.59z"/>
  </SvgIcon>,
  apple: <AppleIcon sx={{ fontSize: 30 }} />,
  custom: <CodeIcon sx={{ fontSize: 30 }} />,
}

// Brand colors for all known provider types
const PROVIDER_COLORS: Record<string, string> = {
  github: '#24292e',
  google: '#4285F4',
  microsoft: '#00a1f1',
  atlassian: '#0052CC',
  slack: '#4A154B',
  linkedin: '#0A66C2',
  facebook: '#1877F2',
  twitter: '#1DA1F2',
  apple: '#000000',
  custom: '#6c757d',
}

// Pre-defined list of common OAuth providers to display even if not configured
const BUILT_IN_PROVIDERS: Partial<OAuthProvider>[] = [
  {
    type: 'github',
    name: 'GitHub',
    description: 'Connect to GitHub to access repositories and collaborate on code'
  },
  {
    type: 'google',
    name: 'Google',
    description: 'Access Google services like Drive, Gmail, and Calendar'
  },
  {
    type: 'microsoft',
    name: 'Microsoft',
    description: 'Connect to Microsoft services including Office 365 and Teams'
  },
  {
    type: 'slack',
    name: 'Slack',
    description: "Integrate with your team's Slack workspace for notifications and commands"
  },
  {
    type: 'linkedin',
    name: 'LinkedIn',
    description: 'Connect your professional profile and network'
  },
  {
    type: 'atlassian',
    name: 'Atlassian',
    description: 'Link to Jira, Confluence and other Atlassian products'
  }
]

const OAuthConnections: React.FC = () => {
  const { error, success } = useSnackbar()
  const api = useApi()
  
  const [connections, setConnections] = useState<OAuthConnection[]>([])
  const [providers, setProviders] = useState<OAuthProvider[]>([])
  const [loading, setLoading] = useState(true)
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false)
  const [connectionToDelete, setConnectionToDelete] = useState<string | null>(null)
  const [connectDialogOpen, setConnectDialogOpen] = useState(false)
  const [selectedProvider, setSelectedProvider] = useState<any>(null)
  
  useEffect(() => {
    loadData()
  }, [])
  
  const loadData = async () => {
    try {
      setLoading(true)
      const [connectionsResponse, providersResponse] = await Promise.all([
        api.get('/api/v1/oauth/connections'),
        api.get('/api/v1/oauth/providers')
      ])
      
      console.log('Providers API response:', providersResponse)
      console.log('Connections API response:', connectionsResponse)
      
      setConnections(connectionsResponse.data || [])
      setProviders(providersResponse.data || [])
    } catch (err) {
      error('Failed to load OAuth connections')
      console.error(err)
      // Ensure we have empty arrays if API calls fail
      setConnections([])
      setProviders([])
    } finally {
      setLoading(false)
    }
  }
  
  const confirmDeleteConnection = (id: string) => {
    setConnectionToDelete(id)
    setConfirmDialogOpen(true)
  }
  
  const handleCloseConfirmDialog = () => {
    setConfirmDialogOpen(false)
    setConnectionToDelete(null)
  }
  
  const handleDeleteConnection = async () => {
    if (!connectionToDelete) return
    
    try {
      await api.delete(`/api/v1/oauth/connections/${connectionToDelete}`)
      success('Connection removed')
      loadData()
      handleCloseConfirmDialog()
    } catch (err) {
      error('Failed to remove connection')
      console.error(err)
    }
  }
  
  const handleRefreshConnection = async (id: string) => {
    try {
      await api.post(`/api/v1/oauth/connections/${id}/refresh`, {})
      success('Connection refreshed')
      loadData()
    } catch (err) {
      error('Failed to refresh connection')
      console.error(err)
    }
  }

  const openConnectDialog = (provider: any) => {
    if (provider.notConfigured) return;
    setSelectedProvider(provider)
    setConnectDialogOpen(true)
  }

  const handleCloseConnectDialog = () => {
    setConnectDialogOpen(false)
    setSelectedProvider(null)
  }
  
  const startOAuthFlow = async (providerId: string) => {
    try {
      const response = await api.get(`/api/v1/oauth/flow/start/${providerId}`)
      window.location.href = response.data.auth_url
    } catch (err) {
      error('Failed to start OAuth flow')
      console.error(err)
    }
  }
  
  const getProviderName = (providerId: string) => {
    const provider = providers.find(p => p.id === providerId)
    return provider ? provider.name : 'Unknown Provider'
  }
  
  const isExpired = (expiresAt: string) => {
    if (!expiresAt) return false
    return new Date(expiresAt) < new Date()
  }
  
  // Get providers that are enabled and not already connected
  const availableProviders = providers.filter(provider => 
    provider.enabled && !connections.some(conn => conn.providerId === provider.id)
  )

  const getProviderIcon = (type: string) => {
    return PROVIDER_ICONS[type] || PROVIDER_ICONS.custom;
  }

  const getProviderColor = (type: string) => {
    return PROVIDER_COLORS[type] || PROVIDER_COLORS.custom;
  }

  const getProfile = (connection: OAuthConnection) => {
    return connection.profile?.displayName || 
           connection.profile?.name || 
           connection.profile?.email || 
           connection.providerUserId || 
           'Connected Account';
  }

  // Combine server providers with built-in providers to show all possibilities
  const getAllProviders = () => {
    const result: any[] = [...availableProviders];
    
    // Add built-in providers that aren't already in the list
    BUILT_IN_PROVIDERS.forEach(builtIn => {
      // Skip if this built-in provider is already in the available providers list
      if (!result.some(p => p.type === builtIn.type)) {
        // Check if this type exists but is disabled
        const existingProvider = providers.find(p => p.type === builtIn.type && !p.enabled);
        
        if (existingProvider) {
          // Use the existing provider but mark it as not configured
          result.push({
            ...existingProvider,
            notConfigured: true
          });
        } else {
          // Add the built-in template
          result.push({
            id: `builtin-${builtIn.type}`,
            name: builtIn.name || PROVIDER_TYPES[builtIn.type as keyof typeof PROVIDER_TYPES] || builtIn.type,
            description: builtIn.description || `Connect to ${builtIn.name || builtIn.type} to enable integration`,
            type: builtIn.type,
            version: '2.0',
            enabled: false,
            notConfigured: true
          });
        }
      }
    });
    
    // Sort by available vs not configured
    return result.sort((a, b) => {
      // Sort by configuration status
      if (a.notConfigured && !b.notConfigured) return 1;
      if (!a.notConfigured && b.notConfigured) return -1;
      
      // If all else is equal, sort alphabetically
      return a.name.localeCompare(b.name);
    });
  }

  const renderConnectionCard = (connection: OAuthConnection) => {
    const { provider } = connection;
    const icon = getProviderIcon(provider?.type || 'custom');
    const color = getProviderColor(provider?.type || 'custom');
    const profileName = getProfile(connection);
    const expired = isExpired(connection.expiresAt);
    
    return (
      <Card sx={{ 
        height: '100%', 
        display: 'flex', 
        flexDirection: 'column',
        transition: 'all 0.2s ease-in-out',
        boxShadow: 2,
        '&:hover': {
          transform: 'translateY(-4px)',
          boxShadow: 4
        }
      }}>
        <CardHeader
          avatar={
            <Avatar sx={{ bgcolor: color }}>
              {icon}
            </Avatar>
          }
          title={provider?.name || getProviderName(connection.providerId)}
          titleTypographyProps={{ variant: 'h6' }}
          subheader={profileName}
          action={
            <Chip 
              color={expired ? 'error' : 'success'} 
              label={expired ? 'Expired' : 'Active'} 
              size="small"
              sx={{ mt: 1 }}
              icon={expired ? undefined : <CheckCircleIcon />}
            />
          }
        />
        <CardContent sx={{ flexGrow: 1 }}>
          <Typography variant="body2" color="text.secondary" gutterBottom>
            Connected on {formatDate(connection.createdAt)}
          </Typography>
          {connection.expiresAt && (
            <Typography variant="body2" color={expired ? "error" : "text.secondary"}>
              {expired ? 'Expired on' : 'Expires on'} {formatDate(connection.expiresAt)}
            </Typography>
          )}
        </CardContent>
        <CardActions sx={{ justifyContent: 'flex-end' }}>
          {connection.expiresAt && (
            <Tooltip title="Refresh token">
              <IconButton 
                onClick={() => handleRefreshConnection(connection.id)} 
                disabled={!connection.expiresAt}
                size="small"
                color="primary"
              >
                <RefreshIcon />
              </IconButton>
            </Tooltip>
          )}
          <Tooltip title="Remove connection">
            <IconButton 
              onClick={() => confirmDeleteConnection(connection.id)} 
              size="small"
              color="default"
            >
              <DeleteIcon />
            </IconButton>
          </Tooltip>
        </CardActions>
      </Card>
    );
  }

  const renderAvailableProviderCard = (provider: any) => {
    const icon = getProviderIcon(provider?.type || 'custom');
    const color = getProviderColor(provider?.type || 'custom');
    const notConfigured = provider.notConfigured;
    
    return (
      <Card 
        sx={{ 
          height: '100%', 
          display: 'flex', 
          flexDirection: 'column',
          transition: 'all 0.25s ease-in-out',
          cursor: notConfigured ? 'default' : 'pointer',
          borderStyle: 'dashed',
          borderWidth: 1,
          borderColor: 'divider',
          opacity: notConfigured ? 0.7 : 1,
          '&:hover': notConfigured ? {} : {
            transform: 'translateY(-4px)',
            boxShadow: 2,
            borderColor: 'primary.main',
          }
        }}
        onClick={notConfigured ? undefined : () => openConnectDialog(provider)}
      >
        <CardHeader
          avatar={
            <Avatar sx={{ 
              bgcolor: 'transparent', 
              color: color,
              border: `1px solid ${color}`
            }}>
              {icon}
            </Avatar>
          }
          title={provider.name}
          titleTypographyProps={{ variant: 'h6' }}
          subheader={PROVIDER_TYPES[provider.type as keyof typeof PROVIDER_TYPES] || provider.type}
          action={notConfigured && (
            <Tooltip title="Not configured by admin">
              <LockIcon sx={{ color: 'text.disabled', mt: 1, mr: 1 }} />
            </Tooltip>
          )}
        />
        <CardContent sx={{ flexGrow: 1 }}>
          <Typography variant="body2" color="text.secondary">
            {provider.description || `Connect your ${provider.name} account to access additional features.`}
          </Typography>
          {notConfigured && (
            <Typography variant="caption" color="text.disabled" sx={{ display: 'block', mt: 1 }}>
              This service is not yet configured. Contact your administrator to enable it.
            </Typography>
          )}
        </CardContent>
        <CardActions sx={{ justifyContent: 'center', pb: 2 }}>
          <Button 
            startIcon={<AddCircleOutlineIcon />}
            color="primary"
            variant={notConfigured ? "text" : "outlined"}
            size="small"
            disabled={notConfigured}
            onClick={(e) => {
              e.stopPropagation(); // Prevent the card click from triggering
              openConnectDialog(provider);
            }}
          >
            Connect
          </Button>
        </CardActions>
      </Card>
    );
  }
  
  return (
    <Box>
      {loading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress />
        </Box>
      ) : (
        <>
          {/* Connected Services Section */}
          {connections.length > 0 && (
            <Box sx={{ mb: 6 }}>
              <Typography variant="h5" sx={{ mb: 2 }}>
                Your Connected Services
              </Typography>
              
              <Grid container spacing={3}>
                {connections.map((connection) => (
                  <Grid item xs={12} sm={6} md={4} key={connection.id}>
                    {renderConnectionCard(connection)}
                  </Grid>
                ))}
              </Grid>
            </Box>
          )}
          
          {/* Available Services Section */}
          <Box>
            {connections.length > 0 && <Divider sx={{ my: 4 }} />}
            
            <Typography variant="h5" sx={{ mb: 2 }}>
              Available Integrations
            </Typography>
            
            <Grid container spacing={3}>
              {getAllProviders().map((provider) => (
                <Grid item xs={12} sm={6} md={4} key={provider.id}>
                  {renderAvailableProviderCard(provider)}
                </Grid>
              ))}
            </Grid>
            
            {/* If no providers are available or configured */}
            {getAllProviders().length === 0 && (
              <Paper sx={{ p: 3, textAlign: 'center', backgroundColor: 'rgba(0,0,0,0.02)' }}>
                <Typography color="text.secondary">
                  No available services to connect. Please contact your administrator to enable OAuth providers.
                </Typography>
              </Paper>
            )}
          </Box>
        </>
      )}
      
      {/* Delete Connection Confirmation Dialog */}
      <Dialog
        open={confirmDialogOpen}
        onClose={handleCloseConfirmDialog}
      >
        <DialogTitle>Remove Connection</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to remove this connection? This action cannot be undone.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseConfirmDialog} color="inherit">Cancel</Button>
          <Button onClick={handleDeleteConnection} color="error" variant="contained">
            Remove
          </Button>
        </DialogActions>
      </Dialog>

      {/* Connect Provider Dialog */}
      <Dialog
        open={connectDialogOpen}
        onClose={handleCloseConnectDialog}
      >
        <DialogTitle>
          Connect to {selectedProvider?.name}
        </DialogTitle>
        <DialogContent>
          <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', pt: 2, pb: 1 }}>
            {selectedProvider && (
              <Avatar 
                sx={{ 
                  width: 60, 
                  height: 60, 
                  mb: 2,
                  bgcolor: 'transparent', 
                  color: getProviderColor(selectedProvider?.type || 'custom'),
                  border: `1px solid ${getProviderColor(selectedProvider?.type || 'custom')}`
                }}
              >
                {getProviderIcon(selectedProvider?.type || 'custom')}
              </Avatar>
            )}
            
            <DialogContentText>
              {selectedProvider?.description || 
                `You will be redirected to ${selectedProvider?.name} to authorize access to your account. No passwords are stored by this application.`}
            </DialogContentText>
          </Box>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseConnectDialog} color="inherit">Cancel</Button>
          <Button 
            onClick={() => {
              handleCloseConnectDialog();
              startOAuthFlow(selectedProvider.id);
            }} 
            color="primary" 
            variant="contained"
            startIcon={<AddCircleOutlineIcon />}
          >
            Connect with {selectedProvider?.name}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default OAuthConnections 