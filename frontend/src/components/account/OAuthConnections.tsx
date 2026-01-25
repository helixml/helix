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
} from '@mui/material'

import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline'
import LockIcon from '@mui/icons-material/Lock'

import { RefreshCcw, Trash, Info } from 'lucide-react'

import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { formatDate } from '../../utils/format'
import { 
  PROVIDER_ICONS,
  PROVIDER_COLORS,
  PROVIDER_TYPES,
  BUILT_IN_PROVIDERS
} from '../icons/ProviderIcons'
import { 
  useListOAuthConnections, 
  useListOAuthProviders, 
  useDeleteOAuthConnection, 
  useRefreshOAuthConnection 
} from '../../services/oauthProvidersService'

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
  
  // React Query hooks
  const { 
    data: connectionsData, 
    isLoading: connectionsLoading, 
    error: connectionsError 
  } = useListOAuthConnections(5 * 1000) // 5 seconds
  
  const { 
    data: providersData, 
    isLoading: providersLoading, 
    error: providersError 
  } = useListOAuthProviders()
  
  const deleteConnectionMutation = useDeleteOAuthConnection()
  const refreshConnectionMutation = useRefreshOAuthConnection()
  
  const [connections, setConnections] = useState<OAuthConnection[]>([])
  const [providers, setProviders] = useState<OAuthProvider[]>([])
  const [confirmDialogOpen, setConfirmDialogOpen] = useState(false)
  const [connectionToDelete, setConnectionToDelete] = useState<string | null>(null)
  const [connectDialogOpen, setConnectDialogOpen] = useState(false)
  const [selectedProvider, setSelectedProvider] = useState<any>(null)
  
  // Process data when it changes
  useEffect(() => {
    if (connectionsData) {    
      // Convert snake_case to camelCase for connections
      const formattedConnections = (connectionsData || []).map((conn: any) => ({
        id: conn.id,
        createdAt: conn.created_at, // Map from snake_case to camelCase
        updatedAt: conn.updated_at,
        userId: conn.user_id,
        providerId: conn.provider_id,
        expiresAt: conn.expires_at,
        providerUserId: conn.provider_user_id,
        provider: conn.provider,
        profile: conn.profile
      }));
      
      setConnections(formattedConnections)
    }
  }, [connectionsData])
  
  useEffect(() => {
    if (providersData) {
      console.log('🍅 TOMATO: Raw Providers API response:', JSON.stringify(providersData, null, 2))
      
      // Map TypesOAuthProvider to component's OAuthProvider interface
      const mappedProviders = (providersData || []).map((provider: any) => ({
        id: provider.id,
        name: provider.name,
        description: provider.description,
        type: provider.type,
        version: '1.0', // Default version since it's not in the API response
        enabled: provider.enabled
      }));
      
      setProviders(mappedProviders)
    }
  }, [providersData])
  
  // Handle errors
  useEffect(() => {
    if (connectionsError) {
      error('Failed to load OAuth connections')
      console.error('Error loading OAuth connections:', connectionsError)
      setConnections([])
    }
  }, [connectionsError, error])
  
  useEffect(() => {
    if (providersError) {
      error('Failed to load OAuth providers')
      console.error('Error loading OAuth providers:', providersError)
      setProviders([])
    }
  }, [providersError, error])
  
  const loading = connectionsLoading || providersLoading
  
  // Helper function to guess the provider type from a provider ID
  const guessProviderType = (providerId: string): string => {
    const lowerProviderId = providerId.toLowerCase();
    
    // Check if the ID contains any known provider types
    for (const type of Object.keys(PROVIDER_TYPES)) {
      if (lowerProviderId.includes(type.toLowerCase())) {
        console.log(`🍅 TOMATO: Guessed provider type ${type} from ID ${providerId}`);
        return type;
      }
    }
    
    // Default to GitHub if we can't determine (since we're focusing on GitHub now)
    return 'github';
  }
  
  // Helper function to get name for built-in providers
  const getBuiltInProviderName = (providerId: string): string => {
    const type = guessProviderType(providerId);
    return PROVIDER_TYPES[type as keyof typeof PROVIDER_TYPES] || 'Unknown Provider';
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
      await deleteConnectionMutation.mutateAsync(connectionToDelete)
      success('Connection removed')
      // Reload the connections data
      // Note: This is handled by the useListOAuthConnections hook
      handleCloseConfirmDialog()
    } catch (err) {
      error('Failed to remove connection')
      console.error(err)
    }
  }
  
  const handleRefreshConnection = async (id: string) => {
    try {
      await refreshConnectionMutation.mutateAsync(id)
      success('Connection refreshed')
      // Reload the connections data
      // Note: This is handled by the useListOAuthConnections hook
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

      // Handle Anthropic OAuth specially - it uses PKCE and has a separate endpoint
      if (providerDetails?.type === 'anthropic') {
        console.log('🍅 TOMATO: Using Anthropic-specific OAuth flow');
        const response = await api.get('/api/v1/auth/anthropic/authorize');
        console.log('🍅 TOMATO: Anthropic OAuth response:', response);

        const authUrl = response?.auth_url;
        if (!authUrl) {
          error('Anthropic OAuth not configured. Please contact your administrator.');
          return;
        }

        // Open Anthropic OAuth in a popup
        const width = 800;
        const height = 700;
        const left = (window.innerWidth - width) / 2;
        const top = (window.innerHeight - height) / 2;

        const popup = window.open(
          authUrl,
          'anthropic-oauth-popup',
          `width=${width},height=${height},left=${left},top=${top},toolbar=0,location=0,menubar=0,directories=0,scrollbars=1`
        );

        if (!popup) {
          error('Popup blocked! Please allow popups to connect to Anthropic.');
          return;
        }

        // Listen for Anthropic OAuth success message
        window.addEventListener('message', function handleAnthropicOAuthMessage(event) {
          if (event.data && event.data.type === 'anthropic-oauth-success') {
            console.log('🍅 TOMATO: Received Anthropic OAuth success message');
            window.removeEventListener('message', handleAnthropicOAuthMessage);
            success('Successfully connected to Anthropic!');
          }
        });

        return;
      }

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
      
      // Open in a popup window instead of redirecting
      const width = 800;
      const height = 700;
      const left = (window.innerWidth - width) / 2;
      const top = (window.innerHeight - height) / 2;
      
      // Open the popup
      const popup = window.open(
        authUrl,
        'oauth-popup',
        `width=${width},height=${height},left=${left},top=${top},toolbar=0,location=0,menubar=0,directories=0,scrollbars=1`
      );
      
      if (!popup) {
        error('Popup blocked! Please allow popups for this site to authenticate with the provider.');
        return;
      }
      
      // Add a listener to reload connections when the popup completes
      window.addEventListener('message', function handleOAuthMessage(event) {
        // Check if this is our OAuth success message
        if (event.data && event.data.type === 'oauth-success') {
          console.log('🍅 TOMATO: Received OAuth success message:', event.data);
          // Clean up the event listener
          window.removeEventListener('message', handleOAuthMessage);
          // Reload the connections data
          // Note: This is handled by the useListOAuthConnections hook
          // Show success message
          success('Successfully connected to provider!');
        }
      });
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
    // Safety check - if providerId is undefined or null, return early
    if (!providerId) {
      return "Unknown Provider";
    }
    
    // First try to find the provider in our loaded providers list
    const foundProvider = providers.find(p => p.id === providerId);
    if (foundProvider && foundProvider.name) {
      return foundProvider.name;
    }
    
    // If we can't find it or it has no name, try to determine from the connection
    const associatedConnection = connections.find(c => c.providerId === providerId);
    if (associatedConnection && associatedConnection.provider?.name) {
      return associatedConnection.provider.name;
    }
    
    // If we have a connection with a provider type, use the known provider type names
    if (associatedConnection && associatedConnection.provider?.type) {
      const typeName = PROVIDER_TYPES[associatedConnection.provider.type as keyof typeof PROVIDER_TYPES];
      if (typeName) return typeName;
    }
    
    // Check if the ID contains any known provider names - without toLowerCase
    for (const [type, name] of Object.entries(PROVIDER_TYPES)) {
      if (providerId.includes(type)) {
        return name;
      }
    }
    
    // Default
    return "Unknown Provider";
  }
  
  const isExpired = (expiresAt: string) => {
    // If no expiration date or default date (year before 2000), treat as non-expiring
    if (!expiresAt || new Date(expiresAt).getFullYear() < 2000) return false;
    
    // Check if the date is in the past
    return new Date(expiresAt) < new Date();
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
  
  // Add this function to test GitHub API access
  const testGitHubConnection = async (connection: OAuthConnection) => {
    try {
      console.log('🍅 TOMATO: Testing GitHub connection:', connection.id);
      
      // Call our backend endpoint that will test the GitHub API access
      const response = await api.get(`/api/v1/oauth/connections/${connection.id}/test`);
      console.log('🍅 TOMATO: GitHub API test response:', response);
      
      if (response && response.success) {
        success(`Successfully tested GitHub connection! Found ${response.repos_count || 'your'} repositories.`);
      } else {
        error('Failed to test GitHub connection. Check console for details.');
        console.error('🍅 TOMATO: GitHub API test failed:', response);
      }
    } catch (err) {
      error('Failed to test GitHub connection');
      console.error('🍅 TOMATO: GitHub API test error:', err);
    }
  }
  
  const renderConnectionCard = (connection: OAuthConnection) => {        
    // Look up the provider directly from the providers list
    const providerFromList = providers.find(p => p.id === connection.providerId);    
    
    // Get the provider type from the list or fall back to a default
    const providerType = providerFromList?.type || 'custom';
    
    // Show exactly what we get from the backend
    const icon = getProviderIcon(providerType);
    const color = getProviderColor(providerType);
    const profileName = getProfile(connection);
    const expired = isExpired(connection.expiresAt);
    
    // Use the name from the providers list, or fall back to other methods
    const providerName = providerFromList?.name || 
                         connection.provider?.name || 
                         getProviderName(connection.providerId || "");
    
    // Check if this is a GitHub connection
    const isGitHub = providerType === 'github';
    
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
            <Avatar sx={{ 
              width: 40,
              height: 40,
              bgcolor: color,
              color: 'white'
            }}>
              {icon}
            </Avatar>
          }
          title={providerName}
          titleTypographyProps={{ variant: 'h6' }}
          subheader={profileName}
        />
        <CardContent sx={{ flexGrow: 1 }}>
          {connection.createdAt ? (
            <Typography variant="body2" fontSize="12px" color="text.secondary" gutterBottom>
              Connected on {formatDate(connection.createdAt)}
            </Typography>
          ) : (
            <Typography variant="body2" fontSize="12px" color="text.secondary" gutterBottom>
              Connection date unknown
            </Typography>
          )}
          
          {/* Only show provider ID if debugging is needed */}
          {/* <Typography variant="body2" color="text.secondary">
            Provider ID: {connection.providerId}
          </Typography> */}
          
          {connection.expiresAt && new Date(connection.expiresAt).getFullYear() > 1970 && (
            <Typography variant="body2" fontSize="12px" color={expired ? "error" : "text.secondary"}>
              {expired ? 'Expired on' : 'Expires on'} {formatDate(connection.expiresAt)}
            </Typography>
          )}
        </CardContent>
        <CardActions sx={{ justifyContent: 'space-between', alignItems: 'center' }}>
          {/* Online status indicator on the left */}
          <Box sx={{ 
            display: 'flex', 
            alignItems: 'center', 
            gap: 0.5,
            ml: 1,
          }}>
            <Box sx={{
              width: 10,
              height: 10,
              borderRadius: '50%',
              backgroundColor: expired ? '#f44336' : '#4caf50',
              flexShrink: 0,
              mr: 0.5,
              mt: 0.2
            }} />
            <Typography variant="body2" fontSize="11px">
              {expired ? 'Expired' : 'Connected'}
            </Typography>
          </Box>
          
          {/* Action buttons on the right */}
          <Box sx={{ display: 'flex', gap: 0.5 }}>
            {/* Add test button for GitHub connections */}
            {isGitHub && (
              <Tooltip title="Test GitHub API access">
                <IconButton 
                  onClick={() => testGitHubConnection(connection)}
                  size="small"
                  color="info"
                >
                  <Info size={16} />
                </IconButton>
              </Tooltip>
            )}
            
            {connection.expiresAt && (
              <Tooltip title="Refresh token">
                <IconButton 
                  onClick={() => handleRefreshConnection(connection.id)} 
                  disabled={!connection.expiresAt}
                  size="small"
                  color="primary"
                >
                  <RefreshCcw size={16} />
                </IconButton>
              </Tooltip>
            )}
            <Tooltip title="Remove connection">
              <IconButton 
                onClick={() => confirmDeleteConnection(connection.id)} 
                size="small"
                color="primary"
              >
                <Trash size={16} />
              </IconButton>
            </Tooltip>
          </Box>
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
            startIcon={<AddCircleOutlineIcon sx={{ color: 'secondary.main' }} />}
            color="secondary"
            variant={(notConfigured || isDisabled) ? "outlined" : "outlined"}
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
                  bgcolor: selectedProvider.enabled ? getProviderColor(selectedProvider?.type || 'custom') : 'transparent',
                  color: selectedProvider.enabled ? 'white' : getProviderColor(selectedProvider?.type || 'custom'),
                  border: selectedProvider.enabled ? 'none' : `1px solid ${getProviderColor(selectedProvider?.type || 'custom')}`
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
        <DialogActions sx={{ justifyContent: 'space-between' }}>
          <Button onClick={handleCloseConnectDialog} color="primary" size="small">Cancel</Button>
          <Button 
            onClick={() => {
              handleCloseConnectDialog();
              startOAuthFlow(selectedProvider.id);
            }} 
            color="secondary" 
            variant="outlined"
            size="small"
          >
            Connect
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default OAuthConnections 