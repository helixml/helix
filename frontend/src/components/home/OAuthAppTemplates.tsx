import React, { FC, useState, useEffect } from 'react'
import {
  Box,
  Typography,
  Grid,
  Avatar,
  Button,
  Tooltip,
} from '@mui/material'
import AddIcon from '@mui/icons-material/Add'
import Row from '../../components/widgets/Row'
import useApi from '../../hooks/useApi'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import { PROVIDER_ICONS, PROVIDER_COLORS, PROVIDER_TYPES } from '../icons/ProviderIcons'

interface OAuthProvider {
  id: string
  name: string
  description: string
  type: string
  version: string
  enabled: boolean
}

interface OAuthAppTemplate {
  id: string
  name: string
  description: string
  provider: OAuthProvider
  templateId: string
  appType: string
}

const OAuthAppTemplates: FC = () => {
  const api = useApi()
  const account = useAccount()
  const { error, success } = useSnackbar()
  
  const [providers, setProviders] = useState<OAuthProvider[]>([])
  const [loading, setLoading] = useState(true)
  const [appTemplates, setAppTemplates] = useState<OAuthAppTemplate[]>([])
  
  // Load OAuth providers on component mount
  useEffect(() => {
    loadProviders()
  }, [])
  
  const loadProviders = async () => {
    try {
      setLoading(true)
      const providersResponse = await api.get('/api/v1/oauth/providers')
      
      // Only include enabled providers with credentials
      const enabledProviders = providersResponse?.filter((provider: any) =>
        provider.enabled && 
        provider.client_id && 
        provider.client_secret
      )
      
      setProviders(enabledProviders || [])
      
      // Generate app templates for each provider
      if (enabledProviders && enabledProviders.length > 0) {
        generateAppTemplates(enabledProviders)
      }
    } catch (err) {
      console.error('Error loading OAuth providers:', err)
      setProviders([])
    } finally {
      setLoading(false)
    }
  }
  
  // Generate app templates based on available providers
  const generateAppTemplates = (providers: OAuthProvider[]) => {
    const templates: OAuthAppTemplate[] = []
    
    providers.forEach(provider => {
      // Add a template app for each provider type
      if (provider.type === 'github') {
        templates.push({
          id: `github-repo-${provider.id}`,
          name: 'GitHub Repository Analyzer',
          description: 'Analyze GitHub repositories, issues, and PRs',
          provider,
          templateId: 'github-repo-analyzer',
          appType: 'assistant'
        })
      } else if (provider.type === 'jira') {
        templates.push({
          id: `jira-project-${provider.id}`,
          name: 'Jira Project Manager',
          description: 'Manage and analyze Jira projects and issues',
          provider,
          templateId: 'jira-project-manager',
          appType: 'assistant'
        })
      } else if (provider.type === 'slack') {
        templates.push({
          id: `slack-channel-${provider.id}`,
          name: 'Slack Channel Assistant',
          description: 'Answer questions and perform tasks in Slack channels',
          provider,
          templateId: 'slack-assistant',
          appType: 'assistant'
        })
      } else if (provider.type === 'google') {
        templates.push({
          id: `google-drive-${provider.id}`,
          name: 'Google Drive Navigator',
          description: 'Search and summarize documents in Google Drive',
          provider,
          templateId: 'google-drive-navigator',
          appType: 'assistant'
        })
      } else {
        // Generic template for other provider types
        templates.push({
          id: `${provider.type}-assistant-${provider.id}`,
          name: `${provider.name} Assistant`,
          description: `AI assistant that connects to your ${provider.name} account`,
          provider,
          templateId: `${provider.type}-assistant`,
          appType: 'assistant'
        })
      }
    })
    
    setAppTemplates(templates)
  }
  
  // Create a new app from an OAuth template
  const createAppFromTemplate = async (template: OAuthAppTemplate) => {
    try {
      // Show the user that we're processing their request
      success(`Preparing to create app with ${template.provider.name} integration...`);
      
      // Check if the required provider ID exists and is valid
      if (!template.provider.id) {
        error('Provider ID is missing');
        console.error('Provider ID is missing for template:', template);
        return;
      }
      
      // Log the template and provider information for debugging
      console.log('Creating app from template:', {
        templateId: template.templateId,
        providerId: template.provider.id,
        providerName: template.provider.name,
        providerType: template.provider.type
      });
      
      // Navigate to the app creation page with template information
      account.orgNavigate('apps', { 
        create: 'true',
        template: template.templateId,
        provider_name: template.provider.name,
        oauth: 'true'
      });
    } catch (err) {
      // Provide more detailed error message
      const errorMessage = err instanceof Error ? 
        `Failed to create app: ${err.message}` : 
        'Failed to create app from template';
      
      error(errorMessage);
      console.error('Error in createAppFromTemplate:', err);
    }
  }
  
  // Get provider icon based on type
  const getProviderIcon = (type: string): React.ReactNode => {
    return PROVIDER_ICONS[type as keyof typeof PROVIDER_ICONS] || null
  }
  
  // Get provider color based on type
  const getProviderColor = (type: string) => {
    return PROVIDER_COLORS[type as keyof typeof PROVIDER_COLORS] || '#aaaaaa'
  }
  
  // Don't render anything if there are no providers or templates
  if (appTemplates.length === 0) {
    return null
  }
  
  return (
    <>
      <Row
        sx={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'left',
          justifyContent: 'left',
          mb: 1,
          mt: 3,
        }}
      >
        Integration Templates
      </Row>
      <Row
        sx={{
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'left',
          justifyContent: 'left',
          mb: 1,
        }}
      >
        <Grid container spacing={1} justifyContent="left">
          {appTemplates.map(template => {
            const providerIcon = getProviderIcon(template.provider.type)
            const providerColor = getProviderColor(template.provider.type)
            
            return (
              <Grid item xs={12} sm={6} md={4} lg={4} xl={4} sx={{ textAlign: 'left', maxWidth: '100%' }} key={template.id}>
                <Box
                  sx={{
                    borderRadius: '12px',
                    border: '1px solid rgba(255, 255, 255, 0.2)',
                    p: 1.5,
                    pb: 0.5,
                    cursor: 'pointer',
                    '&:hover': {
                      backgroundColor: 'rgba(255, 255, 255, 0.05)',
                    },
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'flex-start',
                    gap: 1,
                    width: '100%',
                    minWidth: 0,
                  }}
                  onClick={() => createAppFromTemplate(template)}
                >
                  <Avatar
                    sx={{
                      width: 28,
                      height: 28,
                      backgroundColor: providerColor,
                      color: '#fff',
                      fontWeight: 'bold',
                    }}
                  >
                    {providerIcon || template.provider.name.charAt(0).toUpperCase()}
                  </Avatar>
                  <Box sx={{ textAlign: 'left', width: '100%', minWidth: 0 }}>
                    <Typography sx={{ 
                      color: '#fff',
                      fontSize: '0.95rem',
                      lineHeight: 1.2,
                      fontWeight: 'bold',
                      overflow: 'hidden',
                      textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap',
                      width: '100%',
                    }}>
                      {template.name}
                    </Typography>
                    <Typography variant="caption" sx={{ 
                      color: 'rgba(255, 255, 255, 0.5)',
                      fontSize: '0.8rem',
                      lineHeight: 1.2,
                    }}>
                      {template.description}
                    </Typography>
                  </Box>
                </Box>
              </Grid>
            )
          })}
        </Grid>
      </Row>
    </>
  )
}

export default OAuthAppTemplates 