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
import { styled } from '@mui/material/styles'
import * as yaml from 'yaml'
import * as pako from 'pako'
import LoginIcon from '@mui/icons-material/Login'
import LockPersonIcon from '@mui/icons-material/LockPerson'
import SecurityIcon from '@mui/icons-material/Security'
import WarningIcon from '@mui/icons-material/Warning'
import SmartToyIcon from '@mui/icons-material/SmartToy'
import CodeIcon from '@mui/icons-material/Code'
import FileDownloadIcon from '@mui/icons-material/FileDownload'

import Page from '../components/system/Page'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useApps from '../hooks/useApps'
import { ICreateAgentParams } from '../contexts/apps'
import useApi from '../hooks/useApi'

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

interface ParsedConfig {
  name?: string
  description?: string
  avatar?: string
  image?: string
  metadata?: {
    name?: string
  }
  spec?: {
    description?: string
    avatar?: string
    image?: string
    assistants?: Array<{
      name?: string
      description?: string
      system_prompt?: string
      model?: string
      provider?: string
      avatar?: string
      image?: string
    }>
  }
  assistants?: Array<{
    name?: string
    description?: string
    system_prompt?: string
    model?: string
    provider?: string
    avatar?: string
    image?: string
  }>
  [key: string]: any
}

const ImportAgent: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const apps = useApps()
  const [configData, setConfigData] = useState<ParsedConfig | null>(null)
  const [yamlString, setYamlString] = useState<string>('')
  const [loading, setLoading] = useState(true)
  const [importing, setImporting] = useState(false)
  const [error, setError] = useState<string>('')

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
        
        setConfigData(parsed)
        setYamlString(decompressed)
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
    if (configData?.metadata?.name) return configData.metadata.name
    if (configData?.name) return configData.name
    if (configData?.spec?.assistants?.[0]?.name) return configData.spec.assistants[0].name
    if (configData?.assistants?.[0]?.name) return configData.assistants[0].name
    return 'Unnamed Agent'
  }

  const getAgentDescription = () => {
    // Prioritize top-level app description over system prompt
    if (configData?.spec?.description) return configData.spec.description
    if (configData?.description) return configData.description
    if (configData?.spec?.assistants?.[0]?.description) return configData.spec.assistants[0].description
    if (configData?.assistants?.[0]?.description) return configData.assistants[0].description
    // Fall back to system prompt only if no description is available
    if (configData?.spec?.assistants?.[0]?.system_prompt) return configData.spec.assistants[0].system_prompt
    if (configData?.assistants?.[0]?.system_prompt) return configData.assistants[0].system_prompt
    return 'No description available'
  }

  const getAgentAvatar = () => {
    if (configData?.spec?.avatar) return configData.spec.avatar
    if (configData?.avatar) return configData.avatar
    if (configData?.spec?.assistants?.[0]?.avatar) return configData.spec.assistants[0].avatar
    if (configData?.assistants?.[0]?.avatar) return configData.assistants[0].avatar
    return null
  }

  const getAgentImage = () => {
    if (configData?.spec?.image) return configData.spec.image
    if (configData?.image) return configData.image
    if (configData?.spec?.assistants?.[0]?.image) return configData.spec.assistants[0].image
    if (configData?.assistants?.[0]?.image) return configData.assistants[0].image
    return null
  }

  const getModelConfiguration = () => {
    // Let backend handle all model defaults - don't auto-populate from general provider/model
    return {
      reasoningModelProvider: '',
      reasoningModel: '',
      reasoningModelEffort: '',
      generationModelProvider: '',
      generationModel: '',
      smallReasoningModelProvider: '',
      smallReasoningModel: '',
      smallReasoningModelEffort: '',
      smallGenerationModelProvider: '',
      smallGenerationModel: '',
    }
  }

  const getSystemPrompt = () => {
    if (!configData) return ''
    
    // Try to get system prompt from assistant configurations first
    // Support both old-style (top-level assistants) and new-style (spec.assistants)
    const systemPrompt = configData.assistants?.[0]?.system_prompt ||
                        configData.spec?.assistants?.[0]?.system_prompt
    
    if (systemPrompt) return systemPrompt
    
    // Fall back to description if no system prompt is found
    return getAgentDescription()
  }

  const handleImport = async () => {
    if (!configData || !account.user) return

    setImporting(true)
    try {
      const api = useApi()
      
      // Send structured request with YAML config
      const appData = {
        organization_id: account.organizationTools.organization?.id || '',
        global: false,
        yaml_config: configData, // Pass the parsed YAML as-is
      }
      
      // Post to the API with structured format
      const result = await api.post('/api/v1/apps', appData, {
        params: {
          create: true,
        }
      })

      if (!result) {
        throw new Error('Failed to create agent')
      }

      // Navigate to the agent editor
      account.orgNavigate('app', { app_id: result.id })
      snackbar.success('Agent imported successfully')
    } catch (error) {
      console.error('Error importing agent:', error)
      snackbar.error('Failed to import agent')
    } finally {
      setImporting(false)
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
                  <SmartToyIcon sx={{ color: themeConfig.tealRoot, mr: 1, fontSize: '1.5rem' }} />
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
                        <SmartToyIcon 
                          sx={{ 
                            fontSize: '1.8rem', 
                            color: 'white',
                            filter: 'drop-shadow(0 2px 4px rgba(0,0,0,0.3))'
                          }} 
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
    </Page>
  )
}

export default ImportAgent 