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
import LockIcon from '@mui/icons-material/Lock'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { formatDate } from '../../utils/format'
import { 
  PROVIDER_ICONS,
  PROVIDER_COLORS,
  PROVIDER_TYPES,
  BUILT_IN_PROVIDERS
} from '../icons/ProviderIcons'

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

const OAuthConnections: React.FC<{}> = () => {
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
      
      console.log('🍅 TOMATO: Providers API response:', providersResponse)
      console.log('🍅 TOMATO: Connections API response:', connectionsResponse)
      
      // Check if providers response is valid and not empty
      if (!providersResponse || (Array.isArray(providersResponse) && providersResponse.length === 0)) {
        console.warn('🍅 TOMATO: No providers returned from API or invalid response');
      }
      
      // Extract data from response
      setConnections(connectionsResponse || [])
      setProviders(providersResponse || [])
    } catch (err) {
      error('Failed to load OAuth connections')
      console.error('Error loading OAuth data:', err)
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
    // First check if this is a valid provider that exists in the database
    if (!isValidProvider(provider)) {
      error('This provider configuration is invalid or incomplete');
      console.log('🍅 TOMATO: Invalid provider configuration:', provider);
      return;
    }
    
    // Don't proceed if provider is not configured or disabled
    if (provider.notConfigured) {
      error('This provider is not configured by your administrator');
      console.log('🍅 TOMATO: Provider not configured:', provider);
      return;
    }
    
    // Also check for disabled providers that aren't marked as notConfigured
    if (!provider.enabled) {
      error('This provider is currently disabled');
      console.log('🍅 TOMATO: Provider disabled:', provider);
      return;
    }
    
    // Log the provider being selected
    console.log('🍅 TOMATO: Opening connect dialog for provider:', {
      id: provider.id,
      name: provider.name,
      type: provider.type,
      enabled: provider.enabled
    });
    
    setSelectedProvider(provider)
    setConnectDialogOpen(true)
  }

  const handleCloseConnectDialog = () => {
    setConnectDialogOpen(false)
    setSelectedProvider(null)
  }
  
  const startOAuthFlow = async (providerId: string) => {
    try {
      console.log('🍅 TOMATO: Starting OAuth flow with provider ID:', providerId);
      
      // Skip attempting to connect if this is a builtin template provider
      if (providerId.startsWith('builtin-')) {
        error('This provider is not configured by your administrator');
        console.error('🍅 TOMATO: Tried to start OAuth flow with a builtin template provider:', providerId);
        return;
      }
      
      // Get provider details to log for debugging
      const providerDetails = providers.find(p => p.id === providerId);
      console.log('🍅 TOMATO: Found provider in state before API call:', providerDetails);
      
      // Ensure the provider ID is in the exact case as the database
      // This could help if there's a case sensitivity issue with UUIDs
      const normalizedProviderId = providerId.toLowerCase();
      console.log('🍅 TOMATO: Using normalized provider ID:', normalizedProviderId);
      
      console.log('🍅 TOMATO: Calling API endpoint:', `/api/v1/oauth/flow/start/${normalizedProviderId}`);
      const response = await api.get(`/api/v1/oauth/flow/start/${normalizedProviderId}`)
      console.log('🍅 TOMATO: API response for OAuth flow:', response);
      
      // If the response is null or undefined, the provider likely doesn't exist in the database
      if (!response) {
        console.error('🍅 TOMATO: Got null response from OAuth flow API');
        error('OAuth flow failed: Provider not found');
        return;
      }
      
      // Handle both response formats - either response.data.auth_url or response.auth_url
      const authUrl = response.auth_url || (response.data && response.data.auth_url);
      
      if (!authUrl) {
        console.error('🍅 TOMATO: No auth_url found in response:', response);
        error('OAuth flow failed: No authorization URL returned');
        return;
      }
      
      window.location.href = authUrl;
    } catch (err: any) {
      error('Failed to start OAuth flow')
      console.error('🍅 TOMATO: OAuth flow error:', err)
      
      // Add more detailed error information
      if (err.response) {
        console.error('🍅 TOMATO: Error response:', {
          status: err.response.status,
          data: err.response.data
        });
        
        // Handle specific error cases
        if (err.response.status === 500 && 
            err.response.data && 
            typeof err.response.data === 'string' && 
            err.response.data.includes('provider not found')) {
          error('Provider not found by API. Database/API mismatch.');
          console.log('🍅 TOMATO: Provider exists in database but API cannot find it:', providerId);
          console.log('🍅 TOMATO: This might be a case sensitivity issue or an API bug. Check the backend logs for more details.');
          
          // Don't reload data as we already know the provider exists in the DB
        }
      }
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
  
  // Get providers that exist on the server and are not already connected
  const availableProviders = providers.filter(provider => 
    !connections.some(conn => conn.providerId === provider.id)
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
    // Check providers for validity and log warnings for invalid ones
    const validProviders = providers.filter(provider => {
      const isValid = isValidProvider(provider);
      if (!isValid) {
        console.warn(`🍅 TOMATO: Filtering out invalid provider from display:`, provider);
      }
      return isValid;
    });
    
    console.log('🍅 TOMATO: Valid providers from API:', validProviders);
    
    // Get available providers that aren't already connected and are valid
    const validAvailableProviders = validProviders.filter(provider => 
      !connections.some(conn => conn.providerId === provider.id)
    );
    
    const result: any[] = [...validAvailableProviders];
    
    console.log('🍅 TOMATO: Starting getAllProviders with available providers:', validAvailableProviders);
    console.log('🍅 TOMATO: All providers from API:', providers);
    
    // Add built-in providers that aren't already in the list
    BUILT_IN_PROVIDERS.forEach(builtIn => {
      // Skip if this built-in provider is already in the available providers list
      if (!result.some(p => p.type === builtIn.type)) {
        // Check if this type exists but is disabled
        const existingProvider = validProviders.find(p => p.type === builtIn.type);
        
        console.log(`🍅 TOMATO: Processing built-in provider ${builtIn.type}, exists in API: ${!!existingProvider}, enabled: ${existingProvider?.enabled}`);
        
        if (existingProvider) {
          // Show all providers from the API, even if they're not enabled
          result.push({
            ...existingProvider,
            notConfigured: !existingProvider.enabled
          });
          console.log(`🍅 TOMATO: Added existing provider from API: ${existingProvider.name}, id: ${existingProvider.id}, enabled: ${existingProvider.enabled}`);
        } else {
          // Add the built-in template
          const builtInProvider = {
            id: `builtin-${builtIn.type}`,
            name: builtIn.name || PROVIDER_TYPES[builtIn.type as keyof typeof PROVIDER_TYPES] || builtIn.type,
            description: builtIn.description || `Connect to ${builtIn.name || builtIn.type} to enable integration`,
            type: builtIn.type,
            version: '2.0',
            enabled: false,
            notConfigured: true
          };
          result.push(builtInProvider);
          console.log(`🍅 TOMATO: Added built-in template provider: ${builtInProvider.name}, id: ${builtInProvider.id}`);
        }
      }
    });
    
    console.log('🍅 TOMATO: Final providers list:', result);
    
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
    const icon = getProviderIcon(provider.type);
    const color = getProviderColor(provider.type);
    const notConfigured = provider.notConfigured;
    const isDisabled = !provider.enabled && !notConfigured;
    
    console.log(`🍅 TOMATO: Rendering provider card: ${provider.name}, id: ${provider.id}, type: ${provider.type}, enabled: ${provider.enabled}, notConfigured: ${notConfigured}`);
    
    return (
      <Card 
        sx={{ 
          height: '100%', 
          display: 'flex', 
          flexDirection: 'column',
          transition: 'all 0.25s ease-in-out',
          cursor: (notConfigured || isDisabled) ? 'default' : 'pointer',
          borderStyle: 'dashed',
          borderWidth: 1,
          borderColor: 'divider',
          opacity: (notConfigured || isDisabled) ? 0.7 : 1,
          '&:hover': (notConfigured || isDisabled) ? {} : {
            transform: 'translateY(-4px)',
            boxShadow: 2,
            borderColor: 'primary.main',
          }
        }}
        onClick={(notConfigured || isDisabled) ? undefined : () => openConnectDialog(provider)}
      >
        <CardHeader
          avatar={
            <Avatar sx={{ 
              bgcolor: (notConfigured || isDisabled) ? 'transparent' : color,
              color: (notConfigured || isDisabled) ? color : 'white',
              border: (notConfigured || isDisabled) ? `1px solid ${color}` : 'none'
            }}>
              {icon}
            </Avatar>
          }
          title={provider.name}
          titleTypographyProps={{ variant: 'h6' }}
          subheader={PROVIDER_TYPES[provider.type as keyof typeof PROVIDER_TYPES] || provider.type}
          action={(notConfigured || isDisabled) && (
            <Tooltip title={notConfigured ? "Not configured by admin" : "Temporarily disabled"}>
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
          {isDisabled && (
            <Typography variant="caption" color="text.disabled" sx={{ display: 'block', mt: 1 }}>
              This service is temporarily disabled. Try again later.
            </Typography>
          )}
        </CardContent>
        <CardActions sx={{ justifyContent: 'center', pb: 2 }}>
          <Button 
            startIcon={<AddCircleOutlineIcon />}
            color="primary"
            variant={(notConfigured || isDisabled) ? "text" : "contained"}
            size="small"
            disabled={(notConfigured || isDisabled)}
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
  
  // Function to validate if a provider exists and is properly configured
  const isValidProvider = (provider: any): boolean => {
    // Check for the minimum required fields
    if (!provider || !provider.id || !provider.type) {
      console.warn('🍅 TOMATO: Invalid provider - missing required fields:', provider);
      return false;
    }
    
    // Skip builtin providers as they're just templates
    if (provider.id.startsWith('builtin-')) {
      return false;
    }
    
    // Verify the provider has a proper UUID-like ID
    const uuidPattern = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
    if (!uuidPattern.test(provider.id)) {
      console.warn('🍅 TOMATO: Provider has malformed ID (not a UUID):', provider.id);
      return false;
    }
    
    return true;
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