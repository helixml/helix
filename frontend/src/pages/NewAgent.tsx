import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Paper from '@mui/material/Paper'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import LinkIcon from '@mui/icons-material/Link'
import TextFieldsIcon from '@mui/icons-material/TextFields'
import SupportIcon from '@mui/icons-material/Support'
import ShoppingCartIcon from '@mui/icons-material/ShoppingCart'
import BuildIcon from '@mui/icons-material/Build'
import SettingsIcon from '@mui/icons-material/Settings'
import Card from '@mui/material/Card'
import Avatar from '@mui/material/Avatar'
import Tooltip from '@mui/material/Tooltip'
import InfoIcon from '@mui/icons-material/Info'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'

import Page from '../components/system/Page'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useApps from '../hooks/useApps'
import { useListProviders } from '../services/providersService'
import { useGetOrgByName } from '../services/orgService';
import { ICreateAgentParams } from '../contexts/apps'
import { PROVIDERS } from '../components/providers/types'
import useRouter from '../hooks/useRouter';

const PERSONAS = {
  IT_SUPPORT: {
    name: 'IT Support Agent',
    icon: <SupportIcon />,
    prompt: `You are an IT Support Specialist with deep knowledge of our company's systems and documentation. Your primary role is to assist employees with technical issues and questions. Always be professional, patient, and thorough in your responses. When answering questions:
1. First, check if the information exists in our documentation
2. Provide step-by-step solutions when possible
3. If you're unsure, acknowledge limitations and suggest contacting the IT team
4. Use clear, non-technical language when explaining solutions
5. Always prioritize security and data protection in your advice`,
    recommendedKnowledgeType: 'url',
    knowledgeMessage: 'Please provide documentation URL for the IT Support agent'
  },
  SALES_ASSISTANT: {
    name: 'Sales Assistant',
    icon: <ShoppingCartIcon />,
    prompt: `You are a knowledgeable Sales Assistant for our company. Your role is to help potential customers understand our products, pricing, and value proposition. When interacting:
1. Be professional yet conversational
2. Focus on understanding customer needs before suggesting solutions
3. Provide accurate pricing information and explain our value proposition
4. Handle objections professionally and provide relevant examples
5. Know when to escalate to a human sales representative
6. Always maintain transparency about product capabilities and limitations`,
    recommendedKnowledgeType: 'text',
    knowledgeMessage: 'Please provide pricing and product information in the text field'
  },
  CUSTOM: {
    name: 'Custom Agent',
    icon: <BuildIcon />,
    prompt: `You are a specialized agent designed to [specific role]. Your key responsibilities include:
1. [Primary responsibility]
2. [Important guidelines or constraints]
3. [How to handle edge cases or limitations]`,
    recommendedKnowledgeType: null,
    knowledgeMessage: ''
  }
} as const


interface ProviderModelPreset {
  reasoningModel: string       // Used for reasoning about tool calling
  reasoningModelEffort: string
  generationModel: string      // Strategy/plan
  smallReasoningModel: string  // Skill result interpretation
  smallReasoningModelEffort: string
  smallGenerationModel: string // Used for thoughts about tools/strategy
}

// Available models for each provider that users can select as their default model
const PROVIDER_AVAILABLE_MODELS: Record<string, { id: string; name: string; description: string }[]> = {
  'openai': [
    { id: 'gpt-4o', name: 'GPT-4o', description: 'Most capable OpenAI model for complex tasks' },
    { id: 'gpt-4o-mini', name: 'GPT-4o Mini', description: 'Fast and cost-effective for simpler tasks' },
    { id: 'gpt-4-turbo', name: 'GPT-4 Turbo', description: 'Previous generation GPT-4 with vision' },
    { id: 'gpt-3.5-turbo', name: 'GPT-3.5 Turbo', description: 'Fast and economical' },
    { id: 'o1', name: 'o1', description: 'Advanced reasoning model' },
    { id: 'o1-mini', name: 'o1 Mini', description: 'Faster reasoning model' },
    { id: 'o3-mini', name: 'o3 Mini', description: 'Latest reasoning model' },
  ],
  'google': [
    { id: 'gemini-2.0-flash-001', name: 'Gemini 2.0 Flash', description: 'Fast multimodal model' },
    { id: 'gemini-1.5-pro', name: 'Gemini 1.5 Pro', description: 'Most capable Gemini model' },
    { id: 'gemini-1.5-flash', name: 'Gemini 1.5 Flash', description: 'Fast and efficient' },
  ],
  'anthropic': [
    { id: 'claude-3-5-sonnet-20241022', name: 'Claude 3.5 Sonnet', description: 'Best balance of capability and speed' },
    { id: 'claude-3-5-haiku-20241022', name: 'Claude 3.5 Haiku', description: 'Fastest Claude model' },
    { id: 'claude-3-opus-20240229', name: 'Claude 3 Opus', description: 'Most capable for complex tasks' },
  ],
}

const PROVIDER_MODEL_PRESETS: Record<string, ProviderModelPreset> = {
  'openai': {
    reasoningModel: 'o3-mini',
    reasoningModelEffort: 'medium',
    generationModel: 'gpt-4o',
    smallReasoningModel: 'o3-mini',
    smallReasoningModelEffort: 'low',
    smallGenerationModel: 'gpt-4o-mini',
  },
  // TODO: fix google models
  'google': {
    reasoningModel: 'gemini-2.0-flash-001',
    reasoningModelEffort: 'none',
    generationModel: 'gemini-2.0-flash-001',
    smallReasoningModel: 'gemini-2.0-flash-001',
    smallReasoningModelEffort: 'none',
    smallGenerationModel: 'gemini-2.0-flash-001',
  },
  // TODO: Match anthropic models by prefix
  'anthropic': {
    reasoningModel: 'claude-3-5-sonnet-20241022',
    reasoningModelEffort: 'none',
    generationModel: 'claude-3-5-sonnet-20241022',
    smallReasoningModel: 'claude-3-5-sonnet-20241022',
    smallReasoningModelEffort: 'none',
    smallGenerationModel: 'claude-3-5-haiku-20241022',
  }
}

function getModelPreset(provider: string): ProviderModelPreset {
  // if provider has prefix user/ - remove it
  const providerName = provider.replace('user/', '')
  return PROVIDER_MODEL_PRESETS[providerName.toLowerCase()]
}

const NewAgent: FC = () => {
  const router = useRouter()
  const account = useAccount()  
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const apps = useApps()

  const orgName = router.params.org_id

  // Get org if orgName is set  
  const { data: org, isLoading: isLoadingOrg } = useGetOrgByName(orgName, orgName !== undefined)

  const { data: providerEndpoints = [], isLoading: isLoadingProviders } = useListProviders({
    loadModels: false,
    orgId: org?.id,
    enabled: !isLoadingOrg,
  })

  const [name, setName] = useState('')
  const [systemPrompt, setSystemPrompt] = useState('')
  const [knowledgeType, setKnowledgeType] = useState<'url' | 'text' | 'file'>('url')
  const [knowledgeUrl, setKnowledgeUrl] = useState('')
  const [knowledgeText, setKnowledgeText] = useState('')
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [selectedPersona, setSelectedPersona] = useState<keyof typeof PERSONAS | null>(null)
  const [selectedProvider, setSelectedProvider] = useState<string | null>(null)
  
  // Add state variables for model fields
  const [reasoningModelProvider, setReasoningModelProvider] = useState('')
  const [reasoningModel, setReasoningModel] = useState('')
  const [reasoningModelEffort, setReasoningModelEffort] = useState('')
  const [generationModelProvider, setGenerationModelProvider] = useState('')
  const [generationModel, setGenerationModel] = useState('')
  const [smallReasoningModelProvider, setSmallReasoningModelProvider] = useState('')
  const [smallReasoningModel, setSmallReasoningModel] = useState('')
  const [smallReasoningModelEffort, setSmallReasoningModelEffort] = useState('')
  const [smallGenerationModelProvider, setSmallGenerationModelProvider] = useState('')
  const [smallGenerationModel, setSmallGenerationModel] = useState('')

  // State for the selected default model (used for basic chat mode)
  const [selectedDefaultModel, setSelectedDefaultModel] = useState('')

  // Helper to get available models for the current provider
  const getAvailableModels = (provider: string | null) => {
    if (!provider || provider === 'custom') return []
    const providerKey = provider.replace('user/', '').toLowerCase()
    return PROVIDER_AVAILABLE_MODELS[providerKey] || []
  }

  // Filter for main providers (OpenAI, Google, Anthropic)
  const mainProviders = providerEndpoints.filter(endpoint => 
    endpoint.name?.toLowerCase().includes('openai') ||
    endpoint.name?.toLowerCase().includes('google') ||
    endpoint.name?.toLowerCase().includes('anthropic')
  )

  const handlePersonaSelect = (persona: keyof typeof PERSONAS) => {
    setSelectedPersona(persona)
    setSystemPrompt(PERSONAS[persona].prompt)
    if (PERSONAS[persona].recommendedKnowledgeType) {
      setKnowledgeType(PERSONAS[persona].recommendedKnowledgeType as 'url' | 'text' | 'file')
    }
  }

  const handleProviderSelect = (providerName: string) => {
    console.log('Provider selected:', providerName)
    setSelectedProvider(providerName)

    // Auto-populate model fields based on provider selection
    const preset = getModelPreset(providerName)
    if (preset) {
      // Set the model fields in the form state
      setReasoningModelProvider(providerName)
      setReasoningModel(preset.reasoningModel)
      setReasoningModelEffort(preset.reasoningModelEffort)
      setGenerationModelProvider(providerName)
      setGenerationModel(preset.generationModel)
      setSmallReasoningModelProvider(providerName)
      setSmallReasoningModel(preset.smallReasoningModel)
      setSmallReasoningModelEffort(preset.smallReasoningModelEffort)
      setSmallGenerationModelProvider(providerName)
      setSmallGenerationModel(preset.smallGenerationModel)

      // Set default model to the generation model
      setSelectedDefaultModel(preset.generationModel)
    } else {
      // Reset default model when switching to custom or unknown provider
      setSelectedDefaultModel('')
    }
  }

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!name || !systemPrompt) {
      snackbar.error('Please fill in all required fields')
      return
    }

    // Require provider and model selection
    if (!selectedProvider) {
      snackbar.error('Please select an AI provider')
      return
    }

    // Require at least a generation model
    if (!generationModel) {
      snackbar.error('Please configure at least a Generation Model')
      return
    }

    setIsSubmitting(true)
    try {
      // Create knowledge source if provided
      const knowledge = []
      if (knowledgeType === 'url' && knowledgeUrl) {
        knowledge.push({
          id: '', // This will be set by the server
          name: 'Web Knowledge',
          description: 'Knowledge from web URL',
          source: {
            web: {
              urls: [knowledgeUrl],
              crawler: {
                enabled: true,
                max_depth: 10,
                max_pages: 20,
                readability: true
              }
            }
          },
          refresh_schedule: '',
          version: '',
          state: 'pending',
          rag_settings: {
            results_count: 0,
            chunk_size: 0,
            chunk_overflow: 0,
            enable_vision: false,
          }
        })
      } else if (knowledgeType === 'text' && knowledgeText) {
        knowledge.push({          
          id: '', // This will be set by the server
          name: 'Text Knowledge',
          description: 'Knowledge from text input',
          source: {
            text: knowledgeText
          },
          refresh_schedule: '',
          version: '',
          state: 'pending',
          rag_settings: {
            results_count: 0,
            chunk_size: 0,
            chunk_overflow: 0,
            enable_vision: false,
          }
        })
      }

      // Create the agent with the new parameter structure
      const agentParams: ICreateAgentParams = {
        name,
        systemPrompt,
        knowledge: knowledge.length > 0 ? knowledge : undefined,
        model: selectedDefaultModel, // Default model for basic chat mode
        reasoningModelProvider,
        reasoningModel,
        reasoningModelEffort,
        generationModelProvider,
        generationModel,
        smallReasoningModelProvider,
        smallReasoningModel,
        smallReasoningModelEffort,
        smallGenerationModelProvider,
        smallGenerationModel,
      }

      const newApp = await apps.createAgent(agentParams)

      if (!newApp) {
        throw new Error('Failed to create agent')
      }

      // Navigate to the agent pages (org aware)
      account.orgNavigate('app', { app_id: newApp.id })
      snackbar.success('Agent created successfully')
    } catch (error) {
      console.error('Error creating agent:', error)
      snackbar.error('Failed to create agent')
    } finally {
      setIsSubmitting(false)
    }
  }

  if (!account.user) return null

  return (
    <Page
      showDrawerButton={false}
      orgBreadcrumbs={true}
      breadcrumbs={[
        {
          title: 'Agents',
          routeName: 'apps'
        },
        {
          title: 'New',
        }
      ]}
    >
      <Container maxWidth="md" sx={{ mt: 4 }}>
        <Paper 
          elevation={0}
          sx={{ 
            p: 4,
            backgroundColor: themeConfig.darkPanel,
            borderRadius: 2,
            boxShadow: '0 4px 24px 0 rgba(0,0,0,0.12)'
          }}
        >
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 4 }}>
            <Typography variant="h4" sx={{ fontWeight: 'bold' }}>
              Create New Agent
            </Typography>
            <LaunchpadCTAButton size="medium" />
          </Box>

          <form onSubmit={handleSubmit}>
            <Grid container spacing={3}>
              {/* Agent Name */}
              <Grid item xs={12}>
                <TextField
                  required
                  fullWidth
                  label="Agent Name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  helperText="Give your agent a descriptive name"
                  sx={{ mb: 1 }}
                />
              </Grid>

              {/* System Prompt */}
              <Grid item xs={12}>
                <Typography variant="h6" sx={{ mb: 2 }}>
                  Choose a Persona
                </Typography>
                <Box sx={{ mb: 3 }}>
                  {Object.entries(PERSONAS).map(([key, persona]) => (
                    <Button
                      key={key}
                      variant={selectedPersona === key ? 'contained' : 'outlined'}
                      color={selectedPersona === key ? 'secondary' : 'primary'}
                      onClick={() => handlePersonaSelect(key as keyof typeof PERSONAS)}
                      startIcon={persona.icon}
                      sx={{ mr: 2 }}
                    >
                      {persona.name}
                    </Button>
                  ))}
                </Box>

                <TextField
                  required
                  fullWidth
                  multiline
                  rows={6}
                  label="System Prompt"
                  value={systemPrompt}
                  onChange={(e) => setSystemPrompt(e.target.value)}
                  helperText="Define how your agent should behave and what it should do"
                  sx={{ mb: 2, mt: 2 }}
                />                
              </Grid>

              {/* Provider Selection Section */}
              <Grid item xs={12}>
                <Typography variant="h6" sx={{ mb: 2 }}>
                  AI Provider <Typography component="span" color="error">*</Typography>
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  Select an AI provider for your agent. This determines which models will be used.
                </Typography>
                {!selectedProvider && (
                  <Typography variant="body2" color="warning.main" sx={{ mb: 2 }}>
                    Please select a provider to continue
                  </Typography>
                )}

                <Box sx={{ display: 'flex', gap: 2, flexWrap: 'wrap' }}>
                  {mainProviders.map((provider) => {
                    const isSelected = selectedProvider === provider.name
                    
                    // Find the matching provider from PROVIDERS list either by id or alias
                    const providerInfo = PROVIDERS.find(p => {                      
                      return p.id === provider.name || (p.alias?.includes(provider.name || '') || false)
                    })
                    
                    return (
                      <Tooltip
                        key={provider.id || provider.name}
                        title={provider.name}
                        arrow
                        placement="top"
                      >
                        <Card
                          onClick={() => handleProviderSelect(provider.name || '')}
                          sx={{
                            width: 80,
                            height: 80,
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: 'center',
                            justifyContent: 'center',
                            cursor: 'pointer',
                            boxShadow: isSelected ? 4 : 2,
                            borderStyle: 'solid',
                            borderWidth: isSelected ? 2 : 1,
                            borderColor: isSelected ? 'secondary.main' : 'divider',
                            transition: 'all 0.2s',
                            '&:hover': {
                              boxShadow: 4,
                              transform: 'translateY(-2px)',
                              borderColor: 'primary.main',
                            },
                            opacity: isLoadingProviders ? 0.5 : 1,
                          }}
                        >
                          <Avatar 
                            sx={{ 
                              bgcolor: 'white', 
                              width: 40, 
                              height: 40,
                              mb: 1
                            }}
                          >
                            {providerInfo?.logo ? (
                              typeof providerInfo.logo === 'string' ? (
                                <img src={providerInfo.logo} alt={provider.name} style={{ width: 32, height: 32 }} />
                              ) : (
                                <providerInfo.logo style={{ width: 32, height: 32 }} />
                              )
                            ) : (
                              <Typography 
                                variant="caption" 
                                sx={{ 
                                  fontWeight: 'bold',
                                  color: 'text.primary'
                                }}
                              >
                                {provider.name?.charAt(0).toUpperCase()}
                              </Typography>
                            )}
                          </Avatar>
                        </Card>
                      </Tooltip>
                    )
                  })}
                  
                  {/* Custom Provider Tile */}
                  <Tooltip
                    title="Custom Models"
                    arrow
                    placement="top"
                  >
                    <Card
                      onClick={() => handleProviderSelect('custom')}
                      sx={{
                        width: 80,
                        height: 80,
                        display: 'flex',
                        flexDirection: 'column',
                        alignItems: 'center',
                        justifyContent: 'center',
                        cursor: 'pointer',
                        boxShadow: selectedProvider === 'custom' ? 4 : 2,
                        borderStyle: 'solid',
                        borderWidth: selectedProvider === 'custom' ? 2 : 1,
                        borderColor: selectedProvider === 'custom' ? 'secondary.main' : 'divider',
                        transition: 'all 0.2s',
                        '&:hover': {
                          boxShadow: 4,
                          transform: 'translateY(-2px)',
                          borderColor: 'primary.main',
                        },
                      }}
                    >
                      <Avatar 
                        sx={{ 
                          bgcolor: 'transparent', 
                          width: 40, 
                          height: 40,
                          mb: 1
                        }}
                      >
                        <SettingsIcon sx={{ width: 32, height: 32, color: 'white' }} />
                      </Avatar>
                    </Card>
                  </Tooltip>
                  
                  {mainProviders.length === 0 && !isLoadingProviders && (
                    <Typography variant="body2" color="text.secondary">
                      No main providers (OpenAI, Google, Anthropic) are currently available.
                    </Typography>
                  )}
                  
                  {isLoadingProviders && (
                    <Typography variant="body2" color="text.secondary">
                      Loading providers...
                    </Typography>
                  )}
                </Box>
                
                {/* Show selected models when provider is chosen */}
                {selectedProvider && selectedProvider !== 'custom' && (
                  <Box sx={{ mt: 3, p: 2, bgcolor: 'transparent', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                    {/* Model Selector Dropdown */}
                    <FormControl fullWidth sx={{ mb: 3 }}>
                      <InputLabel id="default-model-label">Default Model</InputLabel>
                      <Select
                        labelId="default-model-label"
                        value={selectedDefaultModel}
                        label="Default Model"
                        onChange={(e) => setSelectedDefaultModel(e.target.value)}
                      >
                        {getAvailableModels(selectedProvider).map((model) => (
                          <MenuItem key={model.id} value={model.id}>
                            <Box>
                              <Typography variant="body2">{model.name}</Typography>
                              <Typography variant="caption" color="text.secondary">
                                {model.description}
                              </Typography>
                            </Box>
                          </MenuItem>
                        ))}
                      </Select>
                      <Typography variant="caption" color="text.secondary" sx={{ mt: 1 }}>
                        This model will be used for basic chat interactions
                      </Typography>
                    </FormControl>

                    <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 2 }}>
                      Agent Mode Models (Advanced)
                    </Typography>
                    <Grid container spacing={2}>
                      <Grid item xs={12} sm={6}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography variant="body2" color="text.secondary">
                            <strong>Reasoning Model:</strong> {reasoningModel} {reasoningModelEffort !== 'none' && `(${reasoningModelEffort} effort)`}
                          </Typography>
                          <Tooltip 
                            title="Planning how to use a particular skill and preparing parameters. Requires strong, smart model"
                            arrow
                            placement="top"
                          >
                            <InfoIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                          </Tooltip>
                        </Box>
                      </Grid>
                      <Grid item xs={12} sm={6}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography variant="body2" color="text.secondary">
                            <strong>Generation Model:</strong> {generationModel}
                          </Typography>
                          <Tooltip 
                            title="Overall planning, this model runs the high level agent loop. Requires strong model"
                            arrow
                            placement="top"
                          >
                            <InfoIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                          </Tooltip>
                        </Box>
                      </Grid>
                      <Grid item xs={12} sm={6}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography variant="body2" color="text.secondary">
                            <strong>Small Reasoning Model:</strong> {smallReasoningModel} {smallReasoningModelEffort !== 'none' && `(${smallReasoningModelEffort} effort)`}
                          </Typography>
                          <Tooltip 
                            title="Used for skill response interpretation or re-running the skill multiple times, this can be a smaller model"
                            arrow
                            placement="top"
                          >
                            <InfoIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                          </Tooltip>
                        </Box>
                      </Grid>
                      <Grid item xs={12} sm={6}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Typography variant="body2" color="text.secondary">
                            <strong>Small Generation Model:</strong> {smallGenerationModel}
                          </Typography>
                          <Tooltip 
                            title="Describes tool usage, strategy for the user. Use small models for this task"
                            arrow
                            placement="top"
                          >
                            <InfoIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
                          </Tooltip>
                        </Box>
                      </Grid>
                    </Grid>
                  </Box>
                )}
                
                {/* Custom provider - show model input fields */}
                {selectedProvider === 'custom' && (
                  <Box sx={{ mt: 3, p: 2, bgcolor: 'transparent', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                      Enter your custom model configuration. At minimum, provide a Generation Model.
                    </Typography>
                    <Grid container spacing={2}>
                      <Grid item xs={12} sm={6}>
                        <TextField
                          fullWidth
                          size="small"
                          label="Generation Model Provider"
                          value={generationModelProvider}
                          onChange={(e) => setGenerationModelProvider(e.target.value)}
                          placeholder="e.g., openai"
                        />
                      </Grid>
                      <Grid item xs={12} sm={6}>
                        <TextField
                          required
                          fullWidth
                          size="small"
                          label="Generation Model *"
                          value={generationModel}
                          onChange={(e) => setGenerationModel(e.target.value)}
                          placeholder="e.g., gpt-4o"
                          helperText="Required"
                        />
                      </Grid>
                      <Grid item xs={12} sm={6}>
                        <TextField
                          fullWidth
                          size="small"
                          label="Reasoning Model Provider"
                          value={reasoningModelProvider}
                          onChange={(e) => setReasoningModelProvider(e.target.value)}
                          placeholder="e.g., openai"
                        />
                      </Grid>
                      <Grid item xs={12} sm={6}>
                        <TextField
                          fullWidth
                          size="small"
                          label="Reasoning Model"
                          value={reasoningModel}
                          onChange={(e) => setReasoningModel(e.target.value)}
                          placeholder="e.g., o3-mini"
                        />
                      </Grid>
                    </Grid>
                  </Box>
                )}
                
                {account.serverConfig.providers_management_enabled && (
                  <Typography
                    variant="body2"
                    color="text.secondary"
                    sx={{ mt: 2 }}
                  >
                    Enable more providers{' '}
                    <Typography
                      component="span"
                      variant="body2"
                      onClick={() => account.orgNavigate('providers')}
                      sx={{
                        color: 'primary.main',
                        cursor: 'pointer',
                        textDecoration: 'underline',
                        '&:hover': {
                          textDecoration: 'underline',
                          opacity: 0.8
                        }
                      }}
                    >
                      here
                    </Typography>
                  </Typography>
                )}
              </Grid>

              {/* Knowledge Section */}
              <Grid item xs={12}>
                <Typography variant="h6" sx={{ mb: 2 }}>
                  Add Knowledge (Optional)
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                  You can add knowledge to your agent by providing a URL, text, or uploading files.
                </Typography>

                <Box sx={{ mb: 3 }}>
                  <Button
                    variant={knowledgeType === 'url' ? 'contained' : 'outlined'}
                    color={knowledgeType === 'url' ? 'secondary' : 'primary'}
                    onClick={() => setKnowledgeType('url')}
                    startIcon={<LinkIcon />}
                    sx={{ 
                      mr: 2,
                      ...(selectedPersona === 'IT_SUPPORT' && {
                        animation: 'shimmer 2s infinite',
                        '@keyframes shimmer': {
                          '0%': { boxShadow: '0 0 5px rgba(255,255,255,0.5)' },
                          '50%': { boxShadow: '0 0 20px rgba(255,255,255,0.8)' },
                          '100%': { boxShadow: '0 0 5px rgba(255,255,255,0.5)' }
                        }
                      })
                    }}
                  >
                    URL
                  </Button>
                  <Button
                    variant={knowledgeType === 'text' ? 'contained' : 'outlined'}
                    color={knowledgeType === 'text' ? 'secondary' : 'primary'}
                    onClick={() => setKnowledgeType('text')}
                    startIcon={<TextFieldsIcon />}
                    sx={{ 
                      mr: 2,
                      ...(selectedPersona === 'SALES_ASSISTANT' && {
                        animation: 'shimmer 2s infinite',
                        '@keyframes shimmer': {
                          '0%': { boxShadow: '0 0 5px rgba(255,255,255,0.5)' },
                          '50%': { boxShadow: '0 0 20px rgba(255,255,255,0.8)' },
                          '100%': { boxShadow: '0 0 5px rgba(255,255,255,0.5)' }
                        }
                      })
                    }}
                  >
                    Text
                  </Button>
                  <Button
                    variant={knowledgeType === 'file' ? 'contained' : 'outlined'}
                    color={knowledgeType === 'file' ? 'secondary' : 'primary'}
                    onClick={() => setKnowledgeType('file')}
                    startIcon={<CloudUploadIcon />}
                  >
                    File
                  </Button>
                </Box>

                {selectedPersona && PERSONAS[selectedPersona].knowledgeMessage && (
                  <Typography 
                    variant="body2" 
                    color="secondary" 
                    sx={{ 
                      mb: 2,
                      animation: 'fadeIn 0.5s',
                      '@keyframes fadeIn': {
                        '0%': { opacity: 0 },
                        '100%': { opacity: 1 }
                      }
                    }}
                  >
                    {PERSONAS[selectedPersona].knowledgeMessage}
                  </Typography>
                )}

                {knowledgeType === 'url' && (
                  <TextField
                    fullWidth
                    label="Knowledge URL"
                    value={knowledgeUrl}
                    onChange={(e) => setKnowledgeUrl(e.target.value)}
                    helperText="Enter a URL containing knowledge for your agent"
                  />
                )}

                {knowledgeType === 'text' && (
                  <TextField
                    fullWidth
                    multiline
                    rows={4}
                    label="Knowledge Text"
                    value={knowledgeText}
                    onChange={(e) => setKnowledgeText(e.target.value)}
                    helperText="Enter text containing knowledge for your agent"
                  />
                )}

                {knowledgeType === 'file' && (
                  <Box sx={{ textAlign: 'center', py: 3 }}>
                    <Typography variant="body2" color="text.secondary">
                      File upload will be available after agent creation
                    </Typography>
                  </Box>
                )}
              </Grid>

              {/* Submit Button */}
              <Grid item xs={12}>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                 Additional configuration like avatar, description, and skills can be set up after creation.
                </Typography>
                <Box sx={{ display: 'flex', justifyContent: 'flex-end', mt: 2 }}>
                  <Tooltip
                    title={!selectedProvider ? "Please select an AI provider first" : !generationModel ? "Please configure a model" : ""}
                    arrow
                    placement="top"
                  >
                    <span>
                      <Button
                        type="submit"
                        variant="outlined"
                        color="secondary"
                        size="large"
                        disabled={isSubmitting || !name.trim() || !selectedProvider || !generationModel}
                      >
                        Create Agent
                      </Button>
                    </span>
                  </Tooltip>
                </Box>
              </Grid>
            </Grid>
          </form>
        </Paper>
      </Container>
    </Page>
  )
}

export default NewAgent