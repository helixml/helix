import React, { useState, useEffect, useCallback } from 'react'
import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,  
  FormControlLabel,
  Grid,
  IconButton,
  Switch,
  TextField,
  Typography,
  Chip,
  Card,
  CardContent,  
  CardActions,
  CardHeader,
  Avatar,
  Tooltip,
  Divider,  
} from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import SettingsIcon from '@mui/icons-material/Settings'
import RefreshIcon from '@mui/icons-material/Refresh'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { formatDate } from '../../utils/format'
import atlassianLogo from '../../../assets/img/atlassian-logo.png'

import { TypesOAuthProviderType, TypesOAuthProvider } from '../../api/api'

import { 
  useListOAuthConnections, 
  useListOAuthProviders, 
  useDeleteOAuthConnection, 
  useRefreshOAuthConnection 
} from '../../services/oauthProvidersService'

// Import the shared icon components
import {
  PROVIDER_ICONS,
  PROVIDER_COLORS,
  PROVIDER_TYPES,
  BUILT_IN_PROVIDERS,
} from '../icons/ProviderIcons'

// Add provider URL defaults for built-in providers
export const PROVIDER_DEFAULTS: Record<string, {
  auth_url: string;
  token_url: string;
  user_info_url: string;
  scopes: string[];
}> = {
  github: {
    auth_url: 'https://github.com/login/oauth/authorize',
    token_url: 'https://github.com/login/oauth/access_token',
    user_info_url: 'https://api.github.com/user',
    scopes: ['read:user', 'user:email', 'repo']
  },
  google: {
    auth_url: 'https://accounts.google.com/o/oauth2/v2/auth',
    token_url: 'https://oauth2.googleapis.com/token',
    user_info_url: 'https://www.googleapis.com/oauth2/v3/userinfo',
    scopes: ['https://www.googleapis.com/auth/calendar', 'https://www.googleapis.com/auth/userinfo.profile', 'https://www.googleapis.com/auth/userinfo.email']
  },
  microsoft: {
    auth_url: 'https://login.microsoftonline.com/common/oauth2/v2.0/authorize',
    token_url: 'https://login.microsoftonline.com/common/oauth2/v2.0/token',
    user_info_url: 'https://graph.microsoft.com/v1.0/me',
    scopes: ['openid', 'profile', 'email', 'offline_access']
  },
  slack: {
    auth_url: 'https://slack.com/oauth/v2/authorize',
    token_url: 'https://slack.com/api/oauth.v2.access',
    user_info_url: 'https://slack.com/api/users.identity',
    scopes: ['identity.basic', 'identity.email', 'identity.avatar']
  },
  linkedin: {
    auth_url: 'https://www.linkedin.com/oauth/v2/authorization',
    token_url: 'https://www.linkedin.com/oauth/v2/accessToken',
    user_info_url: 'https://api.linkedin.com/v2/me',
    scopes: ['r_liteprofile', 'r_emailaddress']
  },
  atlassian: {
    auth_url: 'https://auth.atlassian.com/authorize',
    token_url: 'https://auth.atlassian.com/oauth/token',
    user_info_url: 'https://api.atlassian.com/me',
    scopes: ['read:me']
  }
}; 

const OAuthProvidersTable: React.FC = () => {
  const { error, success } = useSnackbar()
  const api = useApi()

  const { 
    data: providersData, 
    isLoading: providersLoading, 
    error: providersError,
    refetch: refetchProviders,
  } = useListOAuthProviders()
  
  // const [providers, setProviders] = useState<OAuthProvider[]>([])
  // const [loading, setLoading] = useState(true)
  const [openDialog, setOpenDialog] = useState(false)
  const [isEditing, setIsEditing] = useState(false)
  const [currentProvider, setCurrentProvider] = useState<TypesOAuthProvider | null>(null)
  // const [renderCount, setRenderCount] = useState(0)
  const [fieldErrors, setFieldErrors] = useState<Record<string, boolean>>({})
  
  // Function to validate a single field
  const validateField = (name: string, value: string) => {
    if (name === 'name' || name === 'client_id' || name === 'client_secret') {
      return !!value.trim();
    }
    return true;
  };
  
  // Reset field errors when opening dialog
  const resetFieldErrors = () => {
    setFieldErrors({});
  };  
  
  const handleOpenDialog = (provider?: TypesOAuthProvider) => {
    resetFieldErrors();
    
    if (provider && !provider.id?.includes('template')) {
      // Editing an existing provider
      setCurrentProvider({...provider});
      setIsEditing(true);
    } else {
      // Creating a new provider
      const templateType = provider?.type || 'custom';
      
      // Get default URLs if this is a known provider type
      const defaults = templateType !== 'custom' && PROVIDER_DEFAULTS[templateType as keyof typeof PROVIDER_DEFAULTS]
        ? PROVIDER_DEFAULTS[templateType as keyof typeof PROVIDER_DEFAULTS]
        : {
            auth_url: '',
            token_url: '',
            user_info_url: '',
            scopes: [] as string[]
          };
      
      setCurrentProvider({
        id: '',
        name: provider?.name || '',
        description: provider?.description || '',
        type: templateType as TypesOAuthProviderType,
        client_id: '',
        client_secret: '',
        auth_url: defaults.auth_url,
        token_url: defaults.token_url,
        user_info_url: defaults.user_info_url,
        callback_url: window.location.origin + '/api/v1/oauth/flow/callback',
        scopes: defaults.scopes,
        enabled: true,
        created_at: new Date().toISOString(),
      });
      setIsEditing(false);
    }
    setOpenDialog(true);
  }
  
  const handleCloseDialog = () => {
    setOpenDialog(false)
    setCurrentProvider(null)
  }
  
  const handleDeleteProvider = async (id: string) => {
    if (!window.confirm('Are you sure you want to delete this provider?')) {
      return
    }
    
    try {
      await api.delete(`/api/v1/oauth/providers/${id}`)
      success('Provider deleted')
      refetchProviders()
    } catch (err) {
      error('Failed to delete provider')
      console.error(err)
    }
  }
  
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!currentProvider) return
    
    const { name, value } = e.target
    setCurrentProvider(prev => prev ? { ...prev, [name]: value } : null)
    
    // Validate field as user types
    if (name === 'name' || name === 'client_id' || name === 'client_secret') {
      setFieldErrors(prev => ({
        ...prev,
        [name]: !validateField(name, value)
      }));
    }
  }
  
  const handleSwitchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!currentProvider) return
    
    const { name, checked } = e.target
    setCurrentProvider(prev => prev ? { ...prev, [name]: checked } : null)
  }
  
  const handleScopeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (!currentProvider) return
    
    const scopes = e.target.value.split(',').map(s => s.trim()).filter(Boolean)
    setCurrentProvider(prev => prev ? { ...prev, scopes } : null)
  }
  
  const handleSaveProvider = async () => {
    if (!currentProvider) return
    
    try {
      // Validate all required fields at once
      const errors: Record<string, boolean> = {};
      
      if (!currentProvider.name || currentProvider.name.trim() === '') {
        errors.name = true;
      }
      
      if (!currentProvider.client_id || currentProvider.client_id.trim() === '') {
        errors.client_id = true;
      }
      
      if (!currentProvider.client_secret || currentProvider.client_secret.trim() === '') {
        errors.client_secret = true;
      }
      
      // If there are any errors, show them and stop
      if (Object.keys(errors).length > 0) {
        setFieldErrors(errors);
        error('Please fill in all required fields');
        return;
      }
      
      // Ensure type is preserved and not empty
      const providerToSave = {
        ...currentProvider,
        type: currentProvider.type || 'custom', // Default to custom if no type
      };
      
      if (isEditing) {
        const response = await api.put(`/api/v1/oauth/providers/${providerToSave.id}`, providerToSave)
        success('Provider updated')
      } else {
        const response = await api.post('/api/v1/oauth/providers', providerToSave)
        success('Provider created')
      }
      
      handleCloseDialog()
      await refetchProviders() // Added await to ensure providers are loaded before continuing
    } catch (err) {
      error('Failed to save provider')
      console.error('Error saving provider:', err)
    }
  }

  const getProviderIcon = (type: string) => {
    return PROVIDER_ICONS[type] || PROVIDER_ICONS.custom;
  }

  const getProviderColor = (type: string) => {
    return PROVIDER_COLORS[type] || PROVIDER_COLORS.custom;
  }
  
  // Get all providers including built-in ones that are not yet configured
  const getAllProviders = () => {
    // Safety check for providers being undefined
    if (!Array.isArray(providersData)) {
      console.warn('🚨 CRITICAL: Providers is not an array');
      return createTemplateProviders();
    }
    
    // Get unique provider types that already exist
    const existingTypes = new Set(providersData.map(p => p.type));
    
    // Create a copy of the providers
    const result = [...providersData];
    
    // Add missing built-in providers as templates
    BUILT_IN_PROVIDERS.forEach(builtIn => {
      const providerType = builtIn.type as TypesOAuthProviderType;
      if (!existingTypes.has(providerType)) {
        result.push({
          id: `template-${providerType}`,
          name: builtIn.name || PROVIDER_TYPES[providerType as keyof typeof PROVIDER_TYPES] || providerType,
          description: builtIn.description || '',
          type: providerType,
          client_id: '',
          client_secret: '',
          auth_url: '',
          token_url: '',
          user_info_url: '',
          callback_url: window.location.origin + '/api/v1/oauth/flow/callback',
          scopes: [],
          enabled: false,
          created_at: new Date().toISOString(),
          isTemplate: true,
          isConfigured: false,
          fromApi: false
        } as TypesOAuthProvider);
      }
    });
    
    // Add only ONE "Add Custom Provider" card at the end
    result.push({
      id: 'add-card',
      name: 'Add Custom Provider',
      description: 'Configure a new OAuth integration with any provider',
      type: TypesOAuthProviderType.OAuthProviderTypeCustom,
      client_id: '',
      client_secret: '',
      auth_url: '',
      token_url: '',
      user_info_url: '',
      callback_url: '',
      scopes: [],
      enabled: false,
      created_at: '',
    } as TypesOAuthProvider);
    
    // Sort providers - configured providers first, then templates, then add card last
    const sortedResult = result.sort((a, b) => {
      // Add card always goes last
      if (a.id === 'add-card') return 1;
      if (b.id === 'add-card') return -1;
      
      // Templates go after configured providers
      const aIsTemplate = a.id?.includes('template');
      const bIsTemplate = b.id?.includes('template');
      
      if (aIsTemplate && !bIsTemplate) return 1;
      if (!aIsTemplate && bIsTemplate) return -1;
      
      // Within same category, sort by name
      return a.name?.localeCompare(b.name || '') || 0;
    });
    
    return sortedResult;
  };
  
  // Helper function to create template providers if nothing is in state
  const createTemplateProviders = () => {
    const templates = BUILT_IN_PROVIDERS.map(builtIn => {
      const providerType = builtIn.type as string;
      return {
        id: `template-${providerType}`,
        name: builtIn.name || PROVIDER_TYPES[providerType as keyof typeof PROVIDER_TYPES] || providerType,
        description: builtIn.description || '',
        type: providerType,
        client_id: '',
        client_secret: '',
        auth_url: '',
        token_url: '',
        user_info_url: '',
        callback_url: window.location.origin + '/api/v1/oauth/flow/callback',
        scopes: [],
        enabled: false,
        created_at: new Date().toISOString(),
        isTemplate: true,        
      } as TypesOAuthProvider;
    });
    
    // Add only ONE "Add Custom Provider" card
    const result = [...templates, {
      id: 'add-card',
      name: 'Add Custom Provider',
      description: 'Configure a new OAuth integration with any provider',
      type: 'custom',
      client_id: '',
      client_secret: '',
      auth_url: '',
      token_url: '',
      user_info_url: '',
      callback_url: '',
      scopes: [],
      enabled: false,
      created_at: '',
    } as TypesOAuthProvider];
    
    return result;
  }
  
  const renderProviderCard = (provider: TypesOAuthProvider) => {
    // Handle the special "Add Custom Provider" card
    if (provider.id === 'add-card') {
      return renderAddCard();
    }
    
    const icon = getProviderIcon(provider.type as string);
    const color = getProviderColor(provider.type as string);
    
    // Check if this is a template - explicit isTemplate flag or not from API and missing credentials
    const isTemplate = provider.id?.includes('template');
    
    const isAtlassian = provider.type === 'atlassian';
    
    return (
      <Card sx={{ 
          height: '100%', 
          display: 'flex', 
          flexDirection: 'column',
          transition: 'all 0.25s ease-in-out',
          opacity: isTemplate ? 0.75 : 1,
          borderStyle: isTemplate ? 'dashed' : 'solid',
          borderWidth: isTemplate ? 1 : 0,
          borderColor: 'divider',
          backgroundColor: isTemplate ? 'transparent' : 'background.paper',
          cursor: 'pointer',
          position: 'relative',
          '&:hover': {
            transform: 'translateY(-4px)',
            boxShadow: 4,
            borderColor: isTemplate ? 'primary.main' : 'divider'
          }
        }}
        onClick={() => {
          if (isTemplate) {
            // For templates, pre-set the provider type and URLs before opening the dialog
            const defaults = PROVIDER_DEFAULTS[provider.type as keyof typeof PROVIDER_DEFAULTS] || {
              auth_url: '',
              token_url: '',
              user_info_url: '',
              scopes: [] as string[]
            };
            
            setCurrentProvider({
              id: '',
              name: provider.name,
              description: provider.description,
              type: provider.type,
              client_id: '',
              client_secret: '',
              auth_url: defaults.auth_url,
              token_url: defaults.token_url,
              user_info_url: defaults.user_info_url,
              callback_url: window.location.origin + '/api/v1/oauth/flow/callback',
              scopes: defaults.scopes,
              enabled: true,
              created_at: new Date().toISOString(),
            });
            setIsEditing(false);
            setOpenDialog(true);
          } else {
            handleOpenDialog(provider);
          }
        }}
      >
        <CardHeader
          avatar={
            isAtlassian ? 
            // For Atlassian, use the image directly 
            <Avatar 
              src={atlassianLogo}
              sx={{ 
                opacity: isTemplate ? 0.7 : 1,
                bgcolor: 'transparent',
                border: isTemplate ? `1px solid ${color}` : 'none',
                width: 40,
                height: 40
              }}
            /> :
            // For other providers, use the standard approach
            <Avatar 
              sx={{ 
                bgcolor: isTemplate ? 'transparent' : color,
                color: isTemplate ? color : 'white',
                border: isTemplate ? `1px solid ${color}` : 'none'
              }}
            >
              {icon}
            </Avatar>
          }
          title={provider.name}
          titleTypographyProps={{ variant: 'h6' }}
          subheader={PROVIDER_TYPES[provider.type as keyof typeof PROVIDER_TYPES] || provider.type}
          action={
            !isTemplate && (
                <Chip 
                  color={provider.enabled ? 'success' : 'default'} 
                  label={provider.enabled ? 'Enabled' : 'Disabled'} 
                  size="small"
                  sx={{ mt: 1 }}
                />
            )
          }
        />
        <CardContent sx={{ flexGrow: 1 }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            {provider.description}
          </Typography>
          {!isTemplate && (
            <>
              <Typography variant="caption" display="block" color="text.secondary">
                Created: {formatDate(provider.created_at || '')}
              </Typography>
            </>
          )}
          {isTemplate && (
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
              This provider is not yet configured
            </Typography>
          )}
        </CardContent>
        <CardActions>
          <Tooltip title={isTemplate ? "Configure" : "Edit"}>
            <IconButton 
              onClick={(e) => {
                e.stopPropagation(); // Prevent the card click from triggering
                handleOpenDialog(isTemplate ? undefined : provider);
              }} 
              size="small" 
              color={isTemplate ? "primary" : "default"}
            >
              {isTemplate ? <AddIcon /> : <EditIcon />}
            </IconButton>
          </Tooltip>
          {!isTemplate && (
            <Tooltip title="Delete">
              <IconButton 
                onClick={(e) => {
                  e.stopPropagation(); // Prevent the card click from triggering
                  handleDeleteProvider(provider.id || '');
                }} 
                size="small" 
                color="default"
              >
                <DeleteIcon />
              </IconButton>
            </Tooltip>
          )}
        </CardActions>
      </Card>
    );
  }

  const renderAddCard = () => {
    return (
      <Card 
        sx={{ 
          height: '100%', 
          display: 'flex', 
          flexDirection: 'column',
          justifyContent: 'center',
          alignItems: 'center',
          backgroundColor: 'rgba(0,0,0,0.02)',
          cursor: 'pointer',
          transition: 'all 0.2s',
          borderStyle: 'dashed',
          borderWidth: 1,
          borderColor: 'divider',
          '&:hover': {
            backgroundColor: 'rgba(0,0,0,0.05)',
            transform: 'translateY(-4px)',
            boxShadow: 2,
            borderColor: 'primary.main'
          }
        }}
        onClick={() => handleOpenDialog()}
      >
        <CardContent sx={{ textAlign: 'center', py: 4 }}>
          <Avatar 
            sx={{ 
              width: 60, 
              height: 60, 
              mx: 'auto', 
              mb: 2,
              bgcolor: 'transparent',
              color: 'primary.main',
              border: '1px dashed'
            }}
          >
            <AddIcon sx={{ fontSize: 30 }} />
          </Avatar>
          <Typography variant="h6" color="primary">
            Add Custom Provider
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
            Configure a new OAuth integration with any service
          </Typography>
        </CardContent>
      </Card>
    );
  }
  
  return (
    <Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 4 }}>
        <Box>
          <Typography variant="h4" sx={{ mb: 1 }}>OAuth Providers</Typography>
          <Typography variant="body1" color="text.secondary">
            Configure integrations with third-party authentication providers for your users
          </Typography>
        </Box>
      </Box>
      
      {providersLoading ? (
        <>
          <Typography>Loading providers...</Typography>
        </>
      ) : (
        <>          
          {providersLoading && <Typography variant="caption" color="text.secondary">Refreshing...</Typography>}
          
          <Grid container spacing={3}>
            {getAllProviders().map((provider) => {
              return (
                <Grid item xs={12} sm={6} md={4} key={provider.id}>
                  {renderProviderCard(provider)}
                </Grid>
              );
            })}
          </Grid>
        </>
      )}
      
      <Dialog open={openDialog} onClose={handleCloseDialog} maxWidth="md" fullWidth>
        <DialogTitle>
          {isEditing 
            ? `Edit ${currentProvider?.name} Provider`
            : currentProvider?.type && currentProvider.type !== 'custom'
              ? `Configure ${PROVIDER_TYPES[currentProvider.type as keyof typeof PROVIDER_TYPES] || currentProvider.type} Provider`
              : 'Configure OAuth Provider'
          }
        </DialogTitle>
        <DialogContent>
          {currentProvider && (
            <Grid container spacing={2} sx={{ mt: 1 }}>
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Name"
                  name="name"
                  value={currentProvider.name}
                  onChange={handleInputChange}
                  required
                  error={fieldErrors['name']}
                  helperText={fieldErrors['name'] ? 'Name is required' : ''}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Description"
                  name="description"
                  value={currentProvider.description}
                  onChange={handleInputChange}
                />
              </Grid>
              
              <Grid item xs={12}>
                <Divider sx={{ my: 1 }}>
                  <Chip label="Authentication Settings" />
                </Divider>
              </Grid>
              
              <Grid item xs={12}>
                <FormControlLabel
                  control={
                    <Switch
                      checked={currentProvider.enabled}
                      onChange={handleSwitchChange}
                      name="enabled"
                    />
                  }
                  label="Enabled"
                />
              </Grid>
              
              <Grid item xs={6}>
                <TextField
                  fullWidth
                  label="Client ID"
                  name="client_id"
                  value={currentProvider.client_id}
                  onChange={handleInputChange}
                  required
                  error={fieldErrors['client_id']}
                  helperText={fieldErrors['client_id'] ? 'Client ID is required' : ''}
                />
              </Grid>
              
              <Grid item xs={6}>
                <TextField
                  fullWidth
                  label="Client Secret"
                  name="client_secret"
                  value={currentProvider.client_secret}
                  onChange={handleInputChange}
                  type="password"
                  required
                  error={fieldErrors['client_secret']}
                  helperText={fieldErrors['client_secret'] ? 'Client Secret is required' : ''}
                />
              </Grid>
              
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Callback URL"
                  name="callback_url"
                  value={currentProvider.callback_url}
                  onChange={handleInputChange}
                  helperText="This URL should be configured in your OAuth provider's settings"
                />
              </Grid>
              
              {currentProvider.type === 'custom' && (
                <>
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      label="Authorization URL"
                      name="auth_url"
                      value={currentProvider.auth_url}
                      onChange={handleInputChange}
                      helperText="The URL to redirect users for authorization"
                    />
                  </Grid>
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      label="Token URL"
                      name="token_url"
                      value={currentProvider.token_url}
                      onChange={handleInputChange}
                      helperText="The URL to exchange authorization code for access token"
                    />
                  </Grid>
                  <Grid item xs={12}>
                    <TextField
                      fullWidth
                      label="User Info URL"
                      name="user_info_url"
                      value={currentProvider.user_info_url}
                      onChange={handleInputChange}
                      helperText="The URL to fetch user information"
                    />
                  </Grid>
                </>
              )}
              
              <Grid item xs={12}>
                <TextField
                  fullWidth
                  label="Scopes"
                  name="scopes"
                  value={currentProvider?.scopes?.join(', ')}
                  onChange={handleScopeChange}
                  helperText="Comma-separated list of OAuth scopes (e.g. profile, email, read:user)"
                />
              </Grid>
            </Grid>
          )}
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseDialog} color="inherit">Cancel</Button>
          <Button onClick={handleSaveProvider} color="primary" variant="contained" startIcon={isEditing ? <SettingsIcon /> : <AddIcon />}>
            {isEditing ? 'Update Provider' : 'Create Provider'}
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  )
}

export default OAuthProvidersTable 