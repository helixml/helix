import React, { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Paper from '@mui/material/Paper'
import Typography from '@mui/material/Typography'
import Alert from '@mui/material/Alert'
import AlertTitle from '@mui/material/AlertTitle'
import Divider from '@mui/material/Divider'
import CircularProgress from '@mui/material/CircularProgress'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import { styled } from '@mui/material/styles'
import * as yaml from 'yaml'
import * as pako from 'pako'
import LoginIcon from '@mui/icons-material/Login'
import LockPersonIcon from '@mui/icons-material/LockPerson'
import SecurityIcon from '@mui/icons-material/Security'
import WarningIcon from '@mui/icons-material/Warning'
import { Bot } from 'lucide-react'
import CodeIcon from '@mui/icons-material/Code'
import FileDownloadIcon from '@mui/icons-material/FileDownload'

import Page from '../components/system/Page'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useApps from '../hooks/useApps'
import { ICreateAgentParams } from '../contexts/apps'
import useApi from '../hooks/useApi'
import { extractErrorMessage } from '../hooks/useErrorCallback'
import { IModelSubstitution, IAppCreateResponse } from '../types'

const CodeBlock = styled('pre')(({ theme }) => ({
  backgroundColor: '#0f0f0f',
  color: '#e8e8e8',
  padding: theme.spacing(3),
  margin: 0,
  borderRadius: 0,
  overflow: 'auto',
  maxHeight: '400px',
  fontSize: '0.875rem',
  fontFamily: 'Monaco, Menlo, "Ubuntu Mono", monospace',
  userSelect: 'text !important' as any,
  WebkitUserSelect: 'text !important' as any,
  MozUserSelect: 'text !important' as any,
  msUserSelect: 'text !important' as any,
  cursor: 'text',
  lineHeight: 1.5,
  whiteSpace: 'pre-wrap',
  wordBreak: 'break-word',
  position: 'relative',
  zIndex: 10,
  pointerEvents: 'auto',
  '&::selection': {
    backgroundColor: '#404040',
    color: '#ffffff',
  },
  '&::-moz-selection': {
    backgroundColor: '#404040',
    color: '#ffffff',
  },
}))

const LoginCard = styled(Card)<{ themeConfig: any }>(({ theme, themeConfig }) => ({
  background: `linear-gradient(135deg, ${themeConfig.tealRoot}15 0%, ${themeConfig.magentaRoot}15 100%)`,
  border: `1px solid ${themeConfig.darkBorder}`,
  borderRadius: '16px',
  boxShadow: `
    0 8px 32px rgba(0,0,0,0.3),
    0 0 0 1px ${themeConfig.tealRoot}30,
    inset 0 1px 0 rgba(255,255,255,0.1)
  `,
  userSelect: 'text',
  WebkitUserSelect: 'text',
  MozUserSelect: 'text',
}))

const ImportCard = styled(Card)<{ themeConfig: any }>(({ theme, themeConfig }) => ({
  background: `linear-gradient(135deg, ${themeConfig.tealRoot}15 0%, ${themeConfig.magentaRoot}15 100%)`,
  border: `1px solid ${themeConfig.darkBorder}`,
  borderRadius: '16px',
  boxShadow: `
    0 8px 32px rgba(0,0,0,0.3),
    0 0 0 1px ${themeConfig.tealRoot}20,
    inset 0 1px 0 rgba(255,255,255,0.05)
  `,
  userSelect: 'text',
  WebkitUserSelect: 'text',
  MozUserSelect: 'text',
}))

const AgentPreviewCard = styled(Card)<{ themeConfig: any; hasBackgroundImage?: boolean }>(({ theme, themeConfig, hasBackgroundImage }) => ({
  background: hasBackgroundImage ? 'transparent' : `linear-gradient(135deg, 
    rgba(0, 213, 255, 0.08) 0%, 
    rgba(239, 46, 198, 0.05) 100%)`,
  border: `1px solid rgba(0, 213, 255, 0.25)`,
  borderRadius: '12px',
  padding: '2.5rem',
  cursor: 'pointer',
  transition: 'all 0.4s cubic-bezier(0.4, 0, 0.2, 1)',
  position: 'relative',
  overflow: 'visible',
  backdropFilter: 'blur(20px)',
  userSelect: 'text',
  WebkitUserSelect: 'text',
  MozUserSelect: 'text',
  
  // Animated top border for non-background cards
  ...(!hasBackgroundImage && {
    '&::before': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: 0,
      right: 0,
      height: '3px',
      background: `linear-gradient(90deg, 
        transparent, 
        rgba(0, 213, 255, 0.8), 
        rgba(239, 46, 198, 0.8),
        transparent)`,
      opacity: 0,
      transition: 'opacity 0.3s ease',
    },
  }),

  // Background image overlay
  ...(hasBackgroundImage && {
    '&::after': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      background: `linear-gradient(135deg, 
        rgba(30, 30, 40, 0.75) 0%, 
        rgba(20, 25, 35, 0.80) 100%)`,
      borderRadius: '12px',
      zIndex: 1,
      pointerEvents: 'none',
    },
    '&::before': {
      content: '""',
      position: 'absolute',
      top: 0,
      left: 0,
      right: 0,
      bottom: 0,
      background: 'rgba(0, 0, 0, 0.45)',
      borderRadius: '12px',
      zIndex: 0,
      pointerEvents: 'none',
    },
  }),

  // Hover effects
  '&:hover': {
    borderColor: 'rgba(0, 213, 255, 0.4)',
    boxShadow: '0 8px 25px rgba(0, 213, 255, 0.15)',
    transform: 'translateY(-4px)',
    background: hasBackgroundImage ? 'transparent' : `linear-gradient(135deg, 
      rgba(0, 213, 255, 0.1) 0%, 
      rgba(239, 46, 198, 0.06) 100%)`,
  },

  // Show animated border on hover for non-background cards
  ...(!hasBackgroundImage && {
    '&:hover::before': {
      opacity: 1,
    },
  }),
}))

const ConfigCard = styled(Card)<{ themeConfig: any }>(({ theme, themeConfig }) => ({
  background: themeConfig.neutral800,
  border: `1px solid ${themeConfig.neutral600}`,
  borderRadius: '12px',
  overflow: 'hidden',
  boxShadow: '0 8px 24px rgba(0, 0, 0, 0.4), inset 0 1px 0 rgba(255, 255, 255, 0.1)',
  position: 'relative',
}))

const SecurityWarningCard = styled(Alert)<{ themeConfig: any }>(({ theme, themeConfig }) => ({
  background: `linear-gradient(135deg, ${themeConfig.yellowRoot}15 0%, ${themeConfig.redRoot}15 100%)`,
  border: `1px solid ${themeConfig.yellowRoot}40`,
  borderRadius: '12px',
  '& .MuiAlert-icon': {
    color: themeConfig.yellowRoot,
  },
}))

const LoginIconContainer = styled(Box)<{ themeConfig: any }>(({ themeConfig }) => ({
  width: '80px',
  height: '80px',
  borderRadius: '50%',
  background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  margin: '0 auto 24px',
  position: 'relative',
  '&::before': {
    content: '""',
    position: 'absolute',
    top: '-4px',
    left: '-4px',
    right: '-4px',
    bottom: '-4px',
    borderRadius: '50%',
    background: `linear-gradient(135deg, ${themeConfig.tealRoot}80 0%, ${themeConfig.magentaRoot}80 100%)`,
    zIndex: -1,
    filter: 'blur(8px)',
    opacity: 0.6,
  },
}))

const AgentIconContainer = styled(Box)<{ themeConfig: any; hasAvatar?: boolean }>(({ themeConfig, hasAvatar }) => ({
  flexShrink: 0,
  width: '64px',
  height: '64px',
  borderRadius: '16px',
  background: `linear-gradient(135deg, 
    rgba(0, 213, 255, 0.15) 0%, 
    rgba(239, 46, 198, 0.15) 100%)`,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  marginBottom: '16px',
  position: 'relative',
  zIndex: 2,
  transition: 'all 0.3s ease',
  '&::before': {
    content: '""',
    position: 'absolute',
    inset: '-2px',
    borderRadius: '18px',
    background: 'rgba(255, 255, 255, 0.15)',
    zIndex: -1,
    opacity: 0,
    transition: 'opacity 0.3s ease',
  },
  '& img': {
    width: '40px',
    height: '40px',
    objectFit: 'contain',
  },
}))

const LoginButton = styled(Button)<{ themeConfig: any }>(({ themeConfig }) => ({
  background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
  border: 'none',
  borderRadius: '12px',
  padding: '12px 32px',
  fontSize: '1.1rem',
  fontWeight: 600,
  textTransform: 'none',
  boxShadow: `0 8px 32px ${themeConfig.tealRoot}30`,
  transition: 'all 0.3s ease',
  '&:hover': {
    background: `linear-gradient(135deg, ${themeConfig.tealDark} 0%, ${themeConfig.magentaDark} 100%)`,
    transform: 'translateY(-2px)',
    boxShadow: `0 12px 40px ${themeConfig.tealRoot}40`,
  },
  '&:active': {
    transform: 'translateY(0px)',
  },
}))

const ImportButton = styled(Button)<{ themeConfig: any }>(({ themeConfig }) => ({
  background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
  border: 'none',
  borderRadius: '12px',
  padding: '12px 24px',
  fontSize: '1rem',
  fontWeight: 600,
  textTransform: 'none',
  boxShadow: `0 6px 24px ${themeConfig.tealRoot}30`,
  transition: 'all 0.3s ease',
  '&:hover': {
    background: `linear-gradient(135deg, ${themeConfig.tealDark} 0%, ${themeConfig.magentaDark} 100%)`,
    transform: 'translateY(-1px)',
    boxShadow: `0 8px 28px ${themeConfig.tealRoot}40`,
  },
  '&:active': {
    transform: 'translateY(0px)',
  },
  '&:disabled': {
    background: themeConfig.neutral600,
    transform: 'none',
    boxShadow: 'none',
  },
}))

const CancelButton = styled(Button)<{ themeConfig: any }>(({ themeConfig }) => ({
  background: 'transparent',
  border: `2px solid ${themeConfig.neutral500}`,
  borderRadius: '12px',
  padding: '10px 24px',
  fontSize: '1rem',
  fontWeight: 600,
  textTransform: 'none',
  color: themeConfig.darkText,
  transition: 'all 0.3s ease',
  '&:hover': {
    background: themeConfig.neutral700,
    borderColor: themeConfig.neutral400,
    transform: 'translateY(-1px)',
  },
  '&:active': {
    transform: 'translateY(0px)',
  },
}))

// Minimal interface for frontend preview - only fields needed for display
interface ParsedConfig {
  // Top-level fields
  name?: string
  description?: string
  avatar?: string
  image?: string
  
  // CRD metadata
  metadata?: {
    name?: string
  }
  
  // CRD spec format
  spec?: {
    name?: string
    description?: string
    avatar?: string
    image?: string
    assistants?: Array<{
      name?: string
      description?: string
      system_prompt?: string // Fallback for description
      avatar?: string
      image?: string
    }>
  }
  
  // Direct assistants format
  assistants?: Array<{
    name?: string
    description?: string
    system_prompt?: string // Fallback for description
    avatar?: string
    image?: string
  }>
  
  // Keep this for passing full parsed config to backend
  [key: string]: any
}

const ImportAgent: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const api = useApi()
  const [configData, setConfigData] = useState<ParsedConfig | null>(null)
  const [yamlString, setYamlString] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState<string>('')
  const [modelSubstitutions, setModelSubstitutions] = useState<IModelSubstitution[]>([])
  const [showSubstitutionDialog, setShowSubstitutionDialog] = useState(false)
  const [pendingNavigation, setPendingNavigation] = useState<{
    appId: string
    hasOAuthSkills: boolean
    hasSeedZipUrl: boolean
  } | null>(null)

  useEffect(() => {
    const parseConfigFromUrl = () => {
      try {
        const urlParams = new URLSearchParams(window.location.search)
        const configParam = urlParams.get('config')
        
        if (!configParam) {
          setError('No agent configuration provided in URL')
          setLoading(false)
          return
        }

        // Decode base64
        const compressedData = atob(decodeURIComponent(configParam))
        
        // Convert string to Uint8Array for pako
        const uint8Array = new Uint8Array(compressedData.length)
        for (let i = 0; i < compressedData.length; i++) {
          uint8Array[i] = compressedData.charCodeAt(i)
        }
        
        // Decompress
        const decompressed = pako.ungzip(uint8Array, { to: 'string' })
        
        // Parse YAML
        const parsed = yaml.parse(decompressed) as ParsedConfig
        
        // Extract just the yaml_config content for display, or use the entire parsed data if it's legacy format
        let displayYaml = decompressed
        if (parsed.yaml_config) {
          // For new structured format, show only the yaml_config content
          displayYaml = yaml.stringify(parsed.yaml_config, { indent: 2, lineWidth: 0 })
        }
        
        setConfigData(parsed)
        setYamlString(displayYaml)
        setLoading(false)
      } catch (err) {
        console.error('Error parsing config:', err)
        setError('Failed to parse agent configuration. The data may be corrupted or in an invalid format.')
        setLoading(false)
      }
    }

    parseConfigFromUrl()
  }, [])

  const getAgentName = () => {
    // Handle new structured format
    const yamlConfig = configData?.yaml_config || configData
    
    if (yamlConfig?.metadata?.name) return yamlConfig.metadata.name
    if (yamlConfig?.name) return yamlConfig.name
    if (yamlConfig?.spec?.assistants?.[0]?.name) return yamlConfig.spec.assistants[0].name
    if (yamlConfig?.assistants?.[0]?.name) return yamlConfig.assistants[0].name
    return 'Unnamed Agent'
  }

  const getAgentDescription = () => {
    // Handle new structured format
    const yamlConfig = configData?.yaml_config || configData
    
    // Prioritize top-level app description over system prompt
    if (yamlConfig?.spec?.description) return yamlConfig.spec.description
    if (yamlConfig?.description) return yamlConfig.description
    if (yamlConfig?.spec?.assistants?.[0]?.description) return yamlConfig.spec.assistants[0].description
    if (yamlConfig?.assistants?.[0]?.description) return yamlConfig.assistants[0].description
    // Fall back to system prompt only if no description is available
    if (yamlConfig?.spec?.assistants?.[0]?.system_prompt) return yamlConfig.spec.assistants[0].system_prompt
    if (yamlConfig?.assistants?.[0]?.system_prompt) return yamlConfig.assistants[0].system_prompt
    return 'No description available'
  }

  const getAgentAvatar = () => {
    // Handle new structured format
    const yamlConfig = configData?.yaml_config || configData
    
    if (yamlConfig?.spec?.avatar) return yamlConfig.spec.avatar
    if (yamlConfig?.avatar) return yamlConfig.avatar
    if (yamlConfig?.spec?.assistants?.[0]?.avatar) return yamlConfig.spec.assistants[0].avatar
    if (yamlConfig?.assistants?.[0]?.avatar) return yamlConfig.assistants[0].avatar
    return null
  }

  const getAgentImage = () => {
    // Handle new structured format
    const yamlConfig = configData?.yaml_config || configData
    
    if (yamlConfig?.spec?.image) return yamlConfig.spec.image
    if (yamlConfig?.image) return yamlConfig.image
    if (yamlConfig?.spec?.assistants?.[0]?.image) return yamlConfig.spec.assistants[0].image
    if (yamlConfig?.assistants?.[0]?.image) return yamlConfig.assistants[0].image
    return null
  }

  const handleImport = async () => {
    if (!configData || !account.user) return

    setImporting(true)
    
    // Always expect the structured format from Launchpad with model_classes
    if (!configData.model_classes || !configData.yaml_config) {
      snackbar.error('Invalid agent configuration format. Please deploy from Launchpad.')
      setImporting(false)
      return
    }

    // Use the structured format from Launchpad
    const appData = {
      organization_id: account.organizationTools.organization?.id || configData.organization_id || '',
      global: configData.global || false,
      yaml_config: configData.yaml_config,
      model_classes: configData.model_classes || []
    }
    
    // Post to the API with structured format
    const result = await api.post<any, IAppCreateResponse>('/api/v1/apps', appData, {
      params: {
        create: true,
      }
    }, {
      snackbar: false, // Disable automatic error snackbar so we can handle it ourselves
      errorCapture: (errorMessage) => {
        snackbar.error(`Failed to import agent: ${errorMessage}`)
        setImporting(false)
      }
    })

    if (result) {
      // Check if the imported config had seed_zip_url (knowledge data) or OAuth skills
      const actualConfig = configData.yaml_config || configData
      const hasSeedZipUrl = (actualConfig as any)?.assistants?.[0]?.knowledge?.some((k: any) => k.source?.filestore?.seed_zip_url) ||
                           (actualConfig as any)?.spec?.assistants?.[0]?.knowledge?.some((k: any) => k.source?.filestore?.seed_zip_url)
      
      const hasOAuthSkills = (actualConfig as any)?.assistants?.[0]?.apis?.some((api: any) => api.oauth_provider) ||
                            (actualConfig as any)?.spec?.assistants?.[0]?.apis?.some((api: any) => api.oauth_provider)
      
      // Check for model substitutions
      if (result.model_substitutions && result.model_substitutions.length > 0) {
        setModelSubstitutions(result.model_substitutions)
        setShowSubstitutionDialog(true)
        
        // Store navigation details for the dialog Continue button
        setPendingNavigation({
          appId: result.id,
          hasOAuthSkills,
          hasSeedZipUrl
        })
      } else {
        // Navigate immediately when no substitutions
        navigateToApp(result.id, hasOAuthSkills, hasSeedZipUrl)
      }
    }
    
    setImporting(false)
  }

  const navigateToApp = (appId: string, hasOAuthSkills: boolean, hasSeedZipUrl: boolean) => {
    // Navigate to the agent editor, jumping to appropriate tab based on what was imported
    if (hasOAuthSkills) {
      // Navigate directly to the skills tab since OAuth skills were imported
      account.orgNavigate('app', { app_id: appId }, { tab: 'skills' })
      snackbar.success('Agent imported successfully! Please review OAuth skills and enable any required providers.')
    } else if (hasSeedZipUrl) {
      // Navigate directly to the knowledge tab since data was imported
      account.orgNavigate('app', { app_id: appId }, { tab: 'knowledge' })
      snackbar.success('Agent imported successfully! Knowledge data is being processed.')
    } else {
      account.orgNavigate('app', { app_id: appId })
      snackbar.success('Agent imported successfully')
    }
  }

  const handleSubstitutionDialogClose = () => {
    setShowSubstitutionDialog(false)
    
    // Navigate when dialog closes
    if (pendingNavigation) {
      navigateToApp(pendingNavigation.appId, pendingNavigation.hasOAuthSkills, pendingNavigation.hasSeedZipUrl)
      setPendingNavigation(null)
    }
  }

  if (loading) {
    return (
      <Page>
        <Container maxWidth="md" sx={{ mt: 4, display: 'flex', justifyContent: 'center' }}>
          <CircularProgress />
        </Container>
      </Page>
    )
  }

  if (error) {
    return (
      <Page>
        <Container maxWidth="md" sx={{ mt: 4 }}>
          <Alert severity="error">
            <AlertTitle>Import Error</AlertTitle>
            {error}
          </Alert>
        </Container>
      </Page>
    )
  }

  if (!account.user) {
    return (
      <Page>
        <Container maxWidth="sm" sx={{ mt: 8, mb: 4 }}>
          <LoginCard themeConfig={themeConfig}>
            <CardContent sx={{ p: 4, textAlign: 'center' }}>
              <LoginIconContainer themeConfig={themeConfig}>
                <LockPersonIcon 
                  sx={{ 
                    fontSize: '2.5rem', 
                    color: 'white',
                    filter: 'drop-shadow(0 2px 4px rgba(0,0,0,0.3))'
                  }} 
                />
              </LoginIconContainer>
              
              <Typography 
                variant="h4" 
                sx={{ 
                  mb: 2, 
                  fontWeight: 700,
                  background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
                  WebkitBackgroundClip: 'text',
                  WebkitTextFillColor: 'transparent',
                  backgroundClip: 'text',
                }}
              >
                Authentication Required
              </Typography>
              
              <Typography 
                variant="body1" 
                sx={{ 
                  mb: 4, 
                  color: themeConfig.darkTextFaded,
                  fontSize: '1.1rem',
                  lineHeight: 1.6,
                  maxWidth: '400px',
                  mx: 'auto',
                }}
              >
                To import agents and access your personalized experience, please sign in to your account.
              </Typography>

              <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'center', mb: 4 }}>
                <SecurityIcon sx={{ color: themeConfig.darkTextFaded, mr: 1, fontSize: '1.2rem' }} />
                <Typography variant="body2" sx={{ color: themeConfig.darkTextFaded, fontSize: '0.9rem' }}>
                  Secure authentication powered by industry standards
                </Typography>
              </Box>
              
              <LoginButton 
                themeConfig={themeConfig}
                variant="contained"
                onClick={account.onLogin}
                startIcon={<LoginIcon />}
                size="large"
              >
                Sign In to Continue
              </LoginButton>
              
              <Typography 
                variant="body2" 
                sx={{ 
                  mt: 3, 
                  color: themeConfig.darkTextFaded,
                  fontSize: '0.85rem',
                }}
              >
                Don't have an account? You'll be able to create one during the sign-in process.
              </Typography>
            </CardContent>
          </LoginCard>
        </Container>
      </Page>
    )
  }

  return (
    <Page
      showDrawerButton={false}
      orgBreadcrumbs={true}
      breadcrumbs={[
        {
          title: 'Import Agent',
        }
      ]}
    >
      <Container maxWidth="lg" sx={{ mt: 4, mb: 6 }}>
                 <ImportCard themeConfig={themeConfig}>
           <CardContent sx={{ p: 4 }}>
             <Box sx={{ textAlign: 'center', mb: 4 }}>
              <Typography 
                variant="h3" 
                sx={{ 
                  mb: 2, 
                  fontWeight: 700,
                  background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
                  WebkitBackgroundClip: 'text',
                  WebkitTextFillColor: 'transparent',
                  backgroundClip: 'text',
                }}
              >
                Import Agent
              </Typography>
              <Typography 
                variant="body1" 
                sx={{ 
                  color: themeConfig.darkTextFaded,
                  fontSize: '1.1rem',
                  maxWidth: '600px',
                  mx: 'auto',
                }}
              >
                Review and import your agent configuration safely
              </Typography>
            </Box>

            <SecurityWarningCard 
              severity="warning" 
              themeConfig={themeConfig}
              sx={{ mb: 4 }}
              icon={<WarningIcon />}
            >
              <AlertTitle sx={{ fontWeight: 600 }}>Security Warning</AlertTitle>
              You are about to import an agent configuration from an external source. 
              Please ensure you trust the source of this agent before proceeding. 
              Review the configuration below carefully.
            </SecurityWarningCard>

            <Box sx={{ display: 'grid', gap: 4, gridTemplateColumns: { xs: '1fr', md: '1fr 1fr' }, mb: 4 }}>
              <Box>
                <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
                  <Bot size={24} color={themeConfig.tealRoot} style={{ marginRight: 8 }} />
                  <Typography variant="h5" sx={{ fontWeight: 600, color: themeConfig.darkText }}>
                    Agent Preview
                  </Typography>
                </Box>
                                                 <AgentPreviewCard 
                  themeConfig={themeConfig} 
                  hasBackgroundImage={!!getAgentImage()}
                  sx={getAgentImage() ? { 
                    backgroundImage: `url(${getAgentImage()})`,
                    backgroundSize: 'cover',
                    backgroundPosition: 'center',
                    backgroundRepeat: 'no-repeat',
                    '&:hover .agent-icon::before': {
                      opacity: 1,
                    },
                    '&:hover .agent-icon': {
                      transform: 'scale(1.05)',
                      background: `linear-gradient(135deg, 
                        rgba(0, 213, 255, 0.2) 0%, 
                        rgba(239, 46, 198, 0.2) 100%)`,
                    },
                  } : {
                    '&:hover .agent-icon::before': {
                      opacity: 1,
                    },
                    '&:hover .agent-icon': {
                      transform: 'scale(1.05)',
                      background: `linear-gradient(135deg, 
                        rgba(0, 213, 255, 0.2) 0%, 
                        rgba(239, 46, 198, 0.2) 100%)`,
                    },
                  }}
                >
                  <CardContent sx={{ p: 0, position: 'relative', zIndex: 2 }}>
                    <AgentIconContainer 
                      themeConfig={themeConfig} 
                      hasAvatar={!!getAgentAvatar()}
                      className="agent-icon"
                    >
                      {getAgentAvatar() ? (
                        <img 
                          src={getAgentAvatar()!} 
                          alt={getAgentName()}
                        />
                      ) : (
                        <Bot
                          size={28}
                          color="white"
                          style={{ filter: 'drop-shadow(0 2px 4px rgba(0,0,0,0.3))' }}
                        />
                      )}
                    </AgentIconContainer>
                    <Typography 
                      variant="h6" 
                      sx={{ 
                        fontWeight: 700, 
                        mb: 2,
                        color: getAgentImage() ? 'white' : themeConfig.darkText,
                        lineHeight: 1.3,
                        textShadow: getAgentImage() ? '0 2px 8px rgba(0,0,0,0.9), 0 1px 3px rgba(0,0,0,0.8)' : 'none',
                      }}
                    >
                      {getAgentName()}
                    </Typography>
                    <Typography 
                      variant="body2" 
                      sx={{ 
                        color: getAgentImage() ? 'rgba(255,255,255,0.95)' : themeConfig.darkTextFaded,
                        lineHeight: 1.6,
                        fontSize: '0.95rem',
                        textShadow: getAgentImage() ? '0 2px 6px rgba(0,0,0,0.9), 0 1px 3px rgba(0,0,0,0.7)' : 'none',
                      }}
                    >
                      {getAgentDescription()}
                    </Typography>
                  </CardContent>
                </AgentPreviewCard>
              </Box>

              <Box>
                <Box sx={{ display: 'flex', alignItems: 'center', mb: 2 }}>
                  <CodeIcon sx={{ color: themeConfig.magentaRoot, mr: 1, fontSize: '1.5rem' }} />
                  <Typography variant="h5" sx={{ fontWeight: 600, color: themeConfig.darkText }}>
                    Configuration
                  </Typography>
                </Box>
                <ConfigCard themeConfig={themeConfig}>
                  <CodeBlock>{yamlString}</CodeBlock>
                </ConfigCard>
              </Box>
            </Box>

            <Box sx={{ display: 'flex', gap: 3, justifyContent: 'center', pt: 2 }}>
              <CancelButton 
                themeConfig={themeConfig}
                variant="outlined" 
                onClick={() => window.close()}
                disabled={importing}
                size="large"
              >
                Cancel
              </CancelButton>
              <ImportButton 
                themeConfig={themeConfig}
                variant="contained" 
                onClick={handleImport}
                disabled={importing}
                startIcon={importing ? <CircularProgress size={20} /> : <FileDownloadIcon />}
                size="large"
              >
                {importing ? 'Importing...' : 'Import Agent'}
              </ImportButton>
            </Box>
          </CardContent>
        </ImportCard>
      </Container>

      {/* Model Substitution Dialog */}
      <Dialog 
        open={showSubstitutionDialog} 
        onClose={undefined}
        disableEscapeKeyDown={true}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle sx={{ 
          background: `linear-gradient(135deg, ${themeConfig.tealRoot}15 0%, ${themeConfig.magentaRoot}15 100%)`,
          borderBottom: `1px solid ${themeConfig.darkBorder}`,
        }}>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <WarningIcon sx={{ color: themeConfig.yellowRoot, mr: 2 }} />
            <Typography variant="h6" sx={{ fontWeight: 600 }}>
              Model Substitutions Applied
            </Typography>
          </Box>
        </DialogTitle>
        <DialogContent sx={{ p: 3 }}>
          <Typography variant="body1" sx={{ mb: 3, color: themeConfig.darkTextFaded }}>
            Some models in your agent configuration were not available and have been automatically 
            substituted with compatible alternatives. The following changes were made:
          </Typography>
          
          <List sx={{ bgcolor: themeConfig.darkPanel, borderRadius: 2 }}>
            {modelSubstitutions.map((substitution, index) => (
              <ListItem key={index} divider={index < modelSubstitutions.length - 1}>
                <ListItemText
                  primary={
                    <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1 }}>
                      {substitution.assistant_name}
                    </Typography>
                  }
                  secondary={
                    <Box>
                      <Typography variant="body2" sx={{ color: themeConfig.darkTextFaded, mb: 1 }}>
                        <strong>Original:</strong> {substitution.original_provider} / {substitution.original_model}
                      </Typography>
                      <Typography variant="body2" sx={{ color: themeConfig.tealRoot, mb: 1 }}>
                        <strong>Substituted with:</strong> {substitution.new_provider} / {substitution.new_model}
                      </Typography>
                      <Typography variant="body2" sx={{ color: themeConfig.darkTextFaded, fontSize: '0.85rem' }}>
                        <em>Reason:</em> {substitution.reason}
                      </Typography>
                    </Box>
                  }
                />
              </ListItem>
            ))}
          </List>

          <Alert severity="info" sx={{ mt: 3 }}>
            <Typography variant="body2">
              You can change these model selections later in the agent editor if needed. 
              The substituted models are compatible alternatives from the same performance class, but you should run the tests to ensure they work as expected.
            </Typography>
          </Alert>
        </DialogContent>
        <DialogActions sx={{ p: 3, borderTop: `1px solid ${themeConfig.darkBorder}` }}>
          <Button 
            onClick={handleSubstitutionDialogClose}
            variant="contained"
            sx={{
              background: `linear-gradient(135deg, ${themeConfig.tealRoot} 0%, ${themeConfig.magentaRoot} 100%)`,
              '&:hover': {
                background: `linear-gradient(135deg, ${themeConfig.tealRoot}dd 0%, ${themeConfig.magentaRoot}dd 100%)`,
              }
            }}
          >
            Continue
          </Button>
        </DialogActions>
      </Dialog>
    </Page>
  )
}

export default ImportAgent 