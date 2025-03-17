import React, { useState, useEffect, useCallback } from 'react'
import {
  Box,
  Button,
  Dialog,
  DialogActions,
  DialogContent,
  DialogTitle,
  FormControl,
  FormControlLabel,
  Grid,
  IconButton,
  InputLabel,
  MenuItem,
  Paper,
  Select,
  SelectChangeEvent,
  Switch,
  TextField,
  Typography,
  Chip,
  Card,
  CardContent,
  CardMedia,
  CardActions,
  CardHeader,
  Avatar,
  Tooltip,
  Divider,
  SvgIcon,
} from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import EditIcon from '@mui/icons-material/Edit'
import DeleteIcon from '@mui/icons-material/Delete'
import GitHubIcon from '@mui/icons-material/GitHub'
import GoogleIcon from '@mui/icons-material/Google'
import AppleIcon from '@mui/icons-material/Apple'
import CloudIcon from '@mui/icons-material/Cloud'
import SettingsIcon from '@mui/icons-material/Settings'
import CodeIcon from '@mui/icons-material/Code'
import RefreshIcon from '@mui/icons-material/Refresh'
import { SvgIconComponent } from '@mui/icons-material'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { formatDate } from '../../utils/format'
import atlassianLogo from '../../../assets/img/atlassian-logo.png'

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

// Atlassian Logo Component

interface OAuthProvider {
  id: string
  name: string
  description: string
  type: string
  client_id: string
  client_secret: string
  auth_url: string
  token_url: string
  user_info_url: string
  callback_url: string
  scopes: string[]
  enabled: boolean
  created_at: string
  isTemplate?: boolean
  isAddCard?: boolean
  isConfigured: boolean
  fromApi: boolean
}

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

const PROVIDER_ICONS: Record<string, React.ReactNode> = {
  github: <GitHubIcon sx={{ fontSize: 30 }} />,
  google: <GoogleIcon sx={{ fontSize: 30 }} />,
  microsoft: <MicrosoftLogo sx={{ fontSize: 30 }} />,
  atlassian: <img src={atlassianLogo} style={{ width: 30, height: 30 }} alt="Atlassian" />,
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

const PROVIDER_COLORS: Record<string, string> = {
  github: '#333333',
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
    name: 'Atlassian', // Uses OAuth 2.0
    description: 'Link to Jira, Confluence and other Atlassian products'
  }
]

// Add provider URL defaults for built-in providers
const PROVIDER_DEFAULTS: Record<string, {
  auth_url: string;
  token_url: string;
  user_info_url: string;
  scopes: string[];
}> = {
  github: {
    auth_url: 'https://github.com/login/oauth/authorize',
    token_url: 'https://github.com/login/oauth/access_token',
    user_info_url: 'https://api.github.com/user',
    scopes: ['read:user', 'user:email']
  },
  google: {
    auth_url: 'https://accounts.google.com/o/oauth2/v2/auth',
    token_url: 'https://oauth2.googleapis.com/token',
    user_info_url: 'https://www.googleapis.com/oauth2/v3/userinfo',
    scopes: ['email', 'profile']
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
}

const OAuthProvidersTable: React.FC = () => {
  const { error, success } = useSnackbar()
  const api = useApi()
  
  const [providers, setProviders] = useState<OAuthProvider[]>([])
  const [loading, setLoading] = useState(true)
  const [openDialog, setOpenDialog] = useState(false)
  const [isEditing, setIsEditing] = useState(false)
  const [currentProvider, setCurrentProvider] = useState<OAuthProvider | null>(null)
  const [renderCount, setRenderCount] = useState(0)
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
  
  // Function to fetch providers
  const fetchProvidersManually = async () => {
    try {
      setLoading(true);
      
      // Get providers from API
      const providers = await api.get('/api/v1/oauth/providers');
      
      if (!Array.isArray(providers)) {
        console.error("Error: API did not return an array of providers");
        setProviders([]);
        setLoading(false);
        return;
      }
      
      // Process each provider
      const processedProviders = providers.map((provider: any) => {
        const clientId = provider.client_id || '';
        const clientSecret = provider.client_secret || '';
        
        const hasClientId = typeof clientId === 'string' && clientId.trim() !== '';
        const hasClientSecret = typeof clientSecret === 'string' && clientSecret.trim() !== '';
        const hasCredentials = hasClientId && hasClientSecret;
        
        return {
          id: provider.id,
          name: provider.name,
          description: provider.description || '',
          type: provider.type,
          client_id: clientId,
          client_secret: clientSecret,
          auth_url: provider.auth_url || '',
          token_url: provider.token_url || '',
          user_info_url: provider.user_info_url || '',
          callback_url: provider.callback_url || '',
          scopes: provider.scopes || [],
          enabled: provider.enabled === true,
          created_at: provider.created_at || new Date().toISOString(),
          isTemplate: false, // Never a template if it came from the API
          isConfigured: hasCredentials, // But we still track if it has credentials
          fromApi: true // Flag to indicate this came from the API
        } as OAuthProvider;
      });
      
      // Update state
      setProviders(processedProviders);
      setLoading(false);
      
      // Force rerender to ensure UI updates
      setTimeout(() => {
        setRenderCount(prev => prev + 1);
      }, 100);
    } catch (err) {
      console.error("Error fetching providers:", err);
      setProviders([]);
      setLoading(false);
    }
  };

  // Load providers on component mount
  useEffect(() => {
    fetchProvidersManually();
  }, []);
  
  const handleOpenDialog = (provider?: OAuthProvider) => {
    resetFieldErrors();
    
    if (provider && !provider.isTemplate) {
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
        type: templateType,
        client_id: '',
        client_secret: '',
        auth_url: defaults.auth_url,
        token_url: defaults.token_url,
        user_info_url: defaults.user_info_url,
        callback_url: window.location.origin + '/oauth/flow/callback',
        scopes: defaults.scopes,
        enabled: true,
        created_at: new Date().toISOString(),
        isConfigured: false,
        fromApi: false,
        isTemplate: false
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
      fetchProvidersManually()
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
  
  const handleSelectChange = (e: SelectChangeEvent) => {
    if (!currentProvider) return
    
    setCurrentProvider({
      ...currentProvider,
      [e.target.name as string]: e.target.value,
    })
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
      await fetchProvidersManually() // Added await to ensure providers are loaded before continuing
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
    if (!Array.isArray(providers)) {
      console.warn('🚨 CRITICAL: Providers is not an array');
      return createTemplateProviders();
    }
    
    // Check if we have any providers with credentials in state
    const configuredProviders = providers.filter(p => {
      const hasClientId = p.client_id && p.client_id.trim() !== '';
      const hasClientSecret = p.client_secret && p.client_secret.trim() !== '';
      return hasClientId && hasClientSecret;
    });
    
    // Get unique provider types that already exist
    const existingTypes = new Set(providers.map(p => p.type));
    
    // Create a copy of the providers
    const result = [...providers];
    
    // Add missing built-in providers as templates
    BUILT_IN_PROVIDERS.forEach(builtIn => {
      const providerType = builtIn.type as string;
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
          callback_url: window.location.origin + '/oauth/flow/callback',
          scopes: [],
          enabled: false,
          created_at: new Date().toISOString(),
          isTemplate: true,
          isConfigured: false,
          fromApi: false
        } as OAuthProvider);
      }
    });
    
    // Add the "Add Custom Provider" card at the end
    result.push({
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
      isAddCard: true,
      isTemplate: false,
      isConfigured: false,
      fromApi: false
    } as OAuthProvider);
    
    // Sort providers - Always put configured providers first
    const sortedResult = result.sort((a, b) => {
      // Add card is always last
      if (a.isAddCard) return 1;
      if (b.isAddCard) return -1;
      
      // Check isConfigured flag first
      if (a.isConfigured && !b.isConfigured) return -1;
      if (!a.isConfigured && b.isConfigured) return 1;
      
      // Then sort by name
      return a.name.localeCompare(b.name);
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
        callback_url: window.location.origin + '/oauth/flow/callback',
        scopes: [],
        enabled: false,
        created_at: new Date().toISOString(),
        isTemplate: true,
        isConfigured: false,
        fromApi: false
      } as OAuthProvider;
    });
    
    // Add the "Add Custom Provider" card
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
      isAddCard: true,
      isTemplate: false,
      isConfigured: false,
      fromApi: false
    } as OAuthProvider];
    
    return result;
  }
  
  const renderProviderCard = (provider: OAuthProvider) => {
    if (provider.isAddCard) {
      return renderAddCard();
    }
    
    const icon = getProviderIcon(provider.type);
    const color = getProviderColor(provider.type);
    
    // Check if this is a template - explicit isTemplate flag or not from API and missing credentials
    const isTemplate = provider.isTemplate === true || 
      (!provider.fromApi && !provider.isConfigured);
    
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
              callback_url: window.location.origin + '/oauth/flow/callback',
              scopes: defaults.scopes,
              enabled: true,
              created_at: new Date().toISOString(),
              isConfigured: false,
              fromApi: false,
              isTemplate: false
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
                Created: {formatDate(provider.created_at)}
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
                  handleDeleteProvider(provider.id);
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
      
      {loading && providers.length === 0 ? (
        <>
          <Typography>Loading providers...</Typography>
          <Typography variant="caption" color="text.secondary">
            Debug: renderCount={renderCount}, providersLength={providers.length}
          </Typography>
        </>
      ) : (
        <>
          <Typography variant="h5" sx={{ mb: 3 }}>Provider Catalog</Typography>
          {loading && <Typography variant="caption" color="text.secondary">Refreshing...</Typography>}
          
          <Button 
            variant="outlined" 
            size="small" 
            sx={{ mb: 2 }}
            onClick={() => fetchProvidersManually()}
            startIcon={<RefreshIcon />}
          >
            Refresh
          </Button>
          
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
                  value={currentProvider.scopes.join(', ')}
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