import React, { useState, useCallback, useEffect, useMemo, useRef } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Fade from '@mui/material/Fade'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Divider from '@mui/material/Divider'

import CheckCircleIcon from '@mui/icons-material/CheckCircle'
import RadioButtonUncheckedIcon from '@mui/icons-material/RadioButtonUnchecked'
import BusinessIcon from '@mui/icons-material/Business'
import FolderIcon from '@mui/icons-material/Folder'
import SmartToyIcon from '@mui/icons-material/SmartToy'
import RocketLaunchIcon from '@mui/icons-material/RocketLaunch'
import PersonIcon from '@mui/icons-material/Person'
import GitHubIcon from '@mui/icons-material/GitHub'
import CreateNewFolderIcon from '@mui/icons-material/CreateNewFolder'
import { Bot, Server } from 'lucide-react'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useApps from '../hooks/useApps'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import { IApp, AGENT_TYPE_ZED_EXTERNAL } from '../types'
import { CodeAgentRuntime, generateAgentName } from '../contexts/apps'
import { AdvancedModelPicker } from '../components/create/AdvancedModelPicker'
import BrowseProvidersDialog from '../components/project/BrowseProvidersDialog'
import { SELECTED_ORG_STORAGE_KEY } from '../utils/localStorage'
import { useCreateOrg } from '../services/orgService'
import { useListProviders } from '../services/providersService'
import { useGetSystemSettings, useUpdateSystemSettings } from '../services/systemSettingsService'
import { PROVIDERS, Provider } from '../components/providers/types'
import AddProviderDialog from '../components/providers/AddProviderDialog'

const RECOMMENDED_MODELS = [
  'claude-opus-4-6',
  'claude-opus-4-5-20251101',
  'claude-sonnet-4-5-20250929',
  'claude-haiku-4-5-20251001',
  'openai/gpt-5.1-codex',
  'openai/gpt-oss-120b',
  'gemini-2.5-pro',
  'gemini-2.5-flash',
  'glm-4.6',
  'Qwen/Qwen3-Coder-480B-A35B-Instruct',
  'Qwen/Qwen3-Coder-30B-A3B-Instruct',
  'Qwen/Qwen3-235B-A22B-fp8-tput',
]
const DEFAULT_ONBOARDING_AGENT_MODEL = 'claude-opus-4-6'
import type { TypesExternalRepositoryType, TypesRepositoryInfo, TypesGitHub, TypesGitLab, TypesAzureDevOps, TypesBitbucket } from '../api/api'
import { TypesProviderEndpointType } from '../api/api'

const ACCENT = '#00e891'
const ACCENT_DIM = 'rgba(0, 232, 145, 0.08)'
const BG = '#0d0d1a'
const CARD_BG = '#0f0f1e'
const CARD_BG_ACTIVE = '#101024'
const CARD_BORDER = 'rgba(255,255,255,0.04)'
const CARD_BORDER_ACTIVE = 'rgba(0, 232, 145, 0.25)'

// Shared text field styling (uses InputProps, not slotProps)
const inputSx = {
  color: '#fff',
  fontSize: '0.82rem',
  '& fieldset': { borderColor: 'rgba(255,255,255,0.1)' },
  '&:hover fieldset': { borderColor: 'rgba(255,255,255,0.2)' },
  '&.Mui-focused fieldset': { borderColor: ACCENT },
}
const labelSx = { color: 'rgba(255,255,255,0.4)', fontSize: '0.82rem' }
const helperSx = { color: 'rgba(255,255,255,0.25)', fontSize: '0.72rem' }

const btnSx = {
  bgcolor: ACCENT,
  color: '#000',
  fontWeight: 600,
  px: 2.5,
  py: 0.8,
  borderRadius: 1.5,
  textTransform: 'none' as const,
  fontSize: '0.8rem',
  '&:hover': { bgcolor: '#00cc7a' },
  '&.Mui-disabled': { bgcolor: 'rgba(0,232,145,0.3)', color: 'rgba(0,0,0,0.5)' },
}

interface StepConfig {
  icon: React.ReactNode
  title: string
  subtitle: string
}

const STEPS: StepConfig[] = [
  {
    icon: <PersonIcon />,
    title: 'Sign in with your account',
    subtitle: 'To get started, please sign in with your account credentials.',
  },
  {
    icon: <Server size={20} />,
    title: 'Connect an AI provider',
    subtitle: 'Add an API key so your agents can use AI models.',
  },
  {
    icon: <BusinessIcon />,
    title: 'Set up your organization',
    subtitle: 'Organizations help you collaborate with your team and manage projects together.',
  },
  {
    icon: <FolderIcon />,
    title: 'Create your first project',
    subtitle: 'Set up your project with a repository and AI agent.',
  },
  {
    icon: <RocketLaunchIcon />,
    title: 'Create your first task',
    subtitle: 'Describe what you want to build and let AI handle the implementation.',
  },
]

export default function Onboarding() {
  const account = useAccount()
  const api = useApi()
  const apps = useApps()
  const snackbar = useSnackbar()
  const router = useRouter()

  // Step tracking
  const [activeStep, setActiveStep] = useState(1)
  const [completedSteps, setCompletedSteps] = useState<Set<number>>(new Set([0]))

  // Step 1: Provider
  const [selectedOnboardingProvider, setSelectedOnboardingProvider] = useState<Provider | null>(null)
  const [addProviderDialogOpen, setAddProviderDialogOpen] = useState(false)
  const [hasAutoSetKodit, setHasAutoSetKodit] = useState(false)
  const initialProvidersChecked = useRef(false)
  const systemSettings = useGetSystemSettings()
  const updateSystemSettings = useUpdateSystemSettings()

  // Step 2: Organization
  const [orgMode, setOrgMode] = useState<'select' | 'create'>('select')
  const [selectedOrgId, setSelectedOrgId] = useState<string>('')
  const [orgDisplayName, setOrgDisplayName] = useState('')
  const [createdOrgId, setCreatedOrgId] = useState<string>('')
  const [createdOrgName, setCreatedOrgName] = useState<string>('')
  const createOrgMutation = useCreateOrg()

  // Step 3: Project + Agent
  const [projectName, setProjectName] = useState('')
  const [projectDescription, setProjectDescription] = useState('')
  const [repoMode, setRepoMode] = useState<'new' | 'external'>('new')
  const [creatingProject, setCreatingProject] = useState(false)
  const [createdProjectId, setCreatedProjectId] = useState<string>('')
  const [linkRepoDialogOpen, setLinkRepoDialogOpen] = useState(false)
  const [linkingRepo, setLinkingRepo] = useState(false)
  const [linkedExternalRepo, setLinkedExternalRepo] = useState<{
    repo: TypesRepositoryInfo
    providerType: string
    oauthConnectionId?: string
    patCredentials?: {
      pat?: string
      username?: string
      orgUrl?: string
      gitlabBaseUrl?: string
      githubBaseUrl?: string
      bitbucketBaseUrl?: string
    }
  } | null>(null)

  // Agent selection (part of step 2)
  const [agentMode, setAgentMode] = useState<'select' | 'create'>('create')
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const [selectedProvider, setSelectedProvider] = useState('')
  const [selectedModel, setSelectedModel] = useState('')
  const [hasUserSelectedModel, setHasUserSelectedModel] = useState(false)
  const [codeAgentRuntime, setCodeAgentRuntime] = useState<CodeAgentRuntime>('zed_agent')
  const [newAgentName, setNewAgentName] = useState('-')
  const [userModifiedAgentName, setUserModifiedAgentName] = useState(false)
  const [creatingAgent, setCreatingAgent] = useState(false)
  const [createdAgentId, setCreatedAgentId] = useState<string>('')

  // Step 4: Task
  const [taskPrompt, setTaskPrompt] = useState('')
  const [creatingTask, setCreatingTask] = useState(false)


  const existingOrgs = account.organizationTools.organizations
  const hasExistingOrgs = existingOrgs.length > 0

  const { data: providers, isLoading: isLoadingProviders } = useListProviders({
    loadModels: true,
    enabled: true,
  })

  // Derived provider state
  const connectedProviderIds = useMemo(() => {
    if (!providers) return new Set<string>()
    const ids = new Set<string>()
    providers.forEach(p => {
      if (p.endpoint_type === TypesProviderEndpointType.ProviderEndpointTypeUser && p.name) {
        ids.add(p.name)
      }
    })
    return ids
  }, [providers])
  const hasUserProviders = connectedProviderIds.size > 0

  // Check if any providers (including system/global) have enabled models
  const hasAnyEnabledModels = useMemo(() => {
    if (!providers) return false
    return providers.some(p =>
      (p.available_models || []).some(m => m.enabled && m.type === 'chat')
    )
  }, [providers])

  // Auto-complete provider step if any providers already exist on initial load
  // (either user-configured or system/global providers)
  useEffect(() => {
    if (initialProvidersChecked.current || isLoadingProviders || !providers) return
    initialProvidersChecked.current = true

    if (hasAnyEnabledModels) {
      setCompletedSteps(prev => new Set([...prev, 1]))
      if (activeStep === 1) {
        setActiveStep(2)
      }
    }
  }, [isLoadingProviders])

  // Auto-set kodit enrichment model after first provider connection during onboarding
  useEffect(() => {
    if (hasAutoSetKodit || !providers?.length) return
    const userProviders = providers.filter(p => p.endpoint_type === TypesProviderEndpointType.ProviderEndpointTypeUser)
    if (userProviders.length === 0) return
    // Don't auto-set if already configured
    if (systemSettings.data?.kodit_enrichment_model) return

    const firstProvider = userProviders[0]
    const providerName = firstProvider.name || ''
    const modelMap: Record<string, { model: string; provider: string }> = {
      'user/openai': { model: 'gpt-4o-mini', provider: 'user/openai' },
      'user/anthropic': { model: 'claude-haiku-4-5-20251001', provider: 'user/anthropic' },
      'user/google': { model: 'gemini-2.5-flash', provider: 'user/google' },
      'user/groq': { model: 'llama-3.1-8b-instant', provider: 'user/groq' },
      'user/togetherai': { model: 'Qwen/Qwen3-30B-A3B', provider: 'user/togetherai' },
      'user/cerebras': { model: 'llama-3.3-70b', provider: 'user/cerebras' },
    }

    const mapping = modelMap[providerName]
    if (!mapping) return

    setHasAutoSetKodit(true)
    updateSystemSettings.mutate({
      kodit_enrichment_model: mapping.model,
      kodit_enrichment_provider: mapping.provider,
      providers_management_enabled: true,
    })
  }, [providers, systemSettings.data, hasAutoSetKodit])

  useEffect(() => {
    if (hasExistingOrgs) {
      setOrgMode('select')
      if (!selectedOrgId && existingOrgs[0]?.id) {
        setSelectedOrgId(existingOrgs[0].id)
      }
    } else {
      setOrgMode('create')
    }
  }, [hasExistingOrgs, existingOrgs])


  const [orgApps, setOrgApps] = useState<IApp[]>([])

  const zedExternalAgents = useMemo(() => {
    if (!orgApps) return []
    return orgApps.filter((app: IApp) => {
      return app.config?.helix?.assistants?.some(
        (assistant) => assistant.agent_type === AGENT_TYPE_ZED_EXTERNAL
      ) || app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL
    })
  }, [orgApps])

  useEffect(() => {
    if (zedExternalAgents.length > 0 && !selectedAgentId) {
      setAgentMode('select')
      setSelectedAgentId(zedExternalAgents[0].id)
    }
  }, [zedExternalAgents, selectedAgentId])

  useEffect(() => {
    if (activeStep !== 3 || !createdOrgId) return

    api.get<IApp[]>('/api/v1/apps', {
      params: { organization_id: createdOrgId },
    }, { snackbar: true }).then(result => {
      setOrgApps(result || [])
    })
  }, [activeStep, createdOrgId])

  useEffect(() => {
    if (agentMode !== 'create' || selectedModel || hasUserSelectedModel || isLoadingProviders || !providers?.length) {
      return
    }

    const providerWithDefault = providers.find((provider) =>
      (provider.available_models || []).some((model) =>
        model.enabled && model.type === 'chat' && model.id === DEFAULT_ONBOARDING_AGENT_MODEL
      )
    )

    if (providerWithDefault) {
      setSelectedProvider(providerWithDefault.name || '')
      setSelectedModel(DEFAULT_ONBOARDING_AGENT_MODEL)
      return
    }

    const firstAvailableModel = providers
      .flatMap((provider) =>
        (provider.available_models || []).map((model) => ({ provider, model }))
      )
      .find(({ model }) => model.enabled && model.type === 'chat')

    if (!firstAvailableModel?.model.id) return

    setSelectedProvider(firstAvailableModel.provider.name || '')
    setSelectedModel(firstAvailableModel.model.id)
  }, [agentMode, selectedModel, hasUserSelectedModel, isLoadingProviders, providers])

  // Auto-generate agent name when model or runtime changes
  useEffect(() => {
    if (!userModifiedAgentName && agentMode === 'create' && selectedModel) {
      setNewAgentName(generateAgentName(selectedModel, codeAgentRuntime))
    }
  }, [selectedModel, codeAgentRuntime, userModifiedAgentName, agentMode])

  const markComplete = useCallback((step: number) => {
    setCompletedSteps(prev => new Set([...prev, step]))
    setActiveStep(step + 1)
  }, [])

  const handleComplete = useCallback(async () => {
    try {
      await api.getApiClient().v1UsersMeOnboardingCreate()
    } catch (err) {
      console.error('Failed to mark onboarding complete:', err)
    }
    if (createdOrgName) {
      localStorage.setItem(SELECTED_ORG_STORAGE_KEY, createdOrgName)
    }
    if (createdProjectId && createdOrgName) {
      router.navigateReplace('org_projects', { org_id: createdOrgName })
    } else {
      router.navigateReplace('projects')
    }
  }, [api, createdProjectId, createdOrgName, router])

  const handleSelectExistingOrg = useCallback(() => {
    if (!selectedOrgId) {
      snackbar.error('Please select an organization')
      return
    }
    const org = existingOrgs.find(o => o.id === selectedOrgId)
    if (!org?.id) {
      snackbar.error('Could not find the selected organization')
      return
    }
    setCreatedOrgId(org.id)
    setCreatedOrgName(org.name || org.id)
    markComplete(2)
  }, [selectedOrgId, existingOrgs, markComplete, snackbar])

  const handleCreateOrg = useCallback(async () => {
    if (!orgDisplayName.trim()) {
      snackbar.error('Please enter an organization name')
      return
    }
    try {
      const newOrg = await createOrgMutation.mutateAsync({
        display_name: orgDisplayName.trim(),
      })
      if (newOrg?.id) {
        setCreatedOrgId(newOrg.id)
        setCreatedOrgName(newOrg.name || newOrg.id)
        await account.organizationTools.loadOrganizations()
        markComplete(2)
      }
    } catch (err) {
      console.error('Failed to create org:', err)
      snackbar.error('Failed to create organization')
    }
  }, [orgDisplayName, createOrgMutation, account.organizationTools, markComplete, snackbar])

  const handleBrowseSelectRepository = useCallback((repo: TypesRepositoryInfo, providerTypeOrCreds: string, oauthConnectionId?: string) => {
    let providerType = providerTypeOrCreds
    let patCredentials: { pat?: string; username?: string; orgUrl?: string; gitlabBaseUrl?: string; githubBaseUrl?: string; bitbucketBaseUrl?: string } | undefined

    try {
      const parsed = JSON.parse(providerTypeOrCreds)
      providerType = parsed.type
      patCredentials = {
        pat: parsed.pat,
        username: parsed.username,
        orgUrl: parsed.orgUrl,
        gitlabBaseUrl: parsed.gitlabBaseUrl,
        githubBaseUrl: parsed.githubBaseUrl,
        bitbucketBaseUrl: parsed.bitbucketBaseUrl,
      }
    } catch {
      // Not JSON - plain provider type (OAuth flow)
    }

    setLinkedExternalRepo({ repo, providerType, oauthConnectionId, patCredentials })
    if (!projectName.trim() && repo.name) {
      setProjectName(repo.name)
    }
    setLinkRepoDialogOpen(false)
  }, [projectName])

  // Step 3: Create project (includes agent creation/selection)
  const handleCreateProject = useCallback(async () => {
    if (!projectName.trim()) {
      snackbar.error('Please enter a project name')
      return
    }
    if (repoMode === 'external' && !linkedExternalRepo) {
      snackbar.error('Please link a repository first')
      return
    }

    // Validate agent selection
    if (agentMode === 'select' && !selectedAgentId) {
      snackbar.error('Please select an agent')
      return
    }
    if (agentMode === 'create' && !selectedModel) {
      snackbar.error('Please select a model for the agent')
      return
    }

    if (!createdOrgId) {
      snackbar.error('No valid organization selected. Please go back and set up your organization first.')
      return
    }

    const orgId = createdOrgId

    setCreatingProject(true)
    try {
      const apiClient = api.getApiClient()

      // Create or use existing agent
      let agentId = ''
      if (agentMode === 'select') {
        agentId = selectedAgentId
      } else {
        setCreatingAgent(true)
        try {
          const agentName = newAgentName.trim() || generateAgentName(selectedModel, codeAgentRuntime)
          const newApp = await apps.createAgent({
            name: agentName,
            description: 'Code development agent',
            agentType: AGENT_TYPE_ZED_EXTERNAL,
            codeAgentRuntime,
            model: selectedModel,
            organizationId: orgId,
            generationModelProvider: selectedProvider,
            generationModel: selectedModel,
            reasoningModelProvider: '',
            reasoningModel: '',
            reasoningModelEffort: 'none',
            smallReasoningModelProvider: '',
            smallReasoningModel: '',
            smallReasoningModelEffort: 'none',
            smallGenerationModelProvider: '',
            smallGenerationModel: '',
          })
          if (!newApp?.id) {
            snackbar.error('Failed to create agent')
            return
          }
          agentId = newApp.id
          setCreatedAgentId(newApp.id)
        } catch (err) {
          console.error('Failed to create agent:', err)
          snackbar.error('Failed to create agent')
          return
        } finally {
          setCreatingAgent(false)
        }
      }

      if (!agentId) {
        snackbar.error('Agent is required')
        return
      }

      let repoId = ''
      if (repoMode === 'new') {
        const repoResponse = await apiClient.v1GitRepositoriesCreate({
          name: projectName.trim(),
          description: projectDescription.trim(),
          owner_id: account.user?.id || '',
          organization_id: orgId,
          repo_type: 'code' as any,
          default_branch: 'main',
        })
        repoId = repoResponse.data?.id || ''
      } else if (repoMode === 'external' && linkedExternalRepo) {
        const { repo, providerType, oauthConnectionId, patCredentials } = linkedExternalRepo

        const externalTypeMap: Record<string, TypesExternalRepositoryType> = {
          'github': 'github' as TypesExternalRepositoryType,
          'gitlab': 'gitlab' as TypesExternalRepositoryType,
          'azure-devops': 'ado' as TypesExternalRepositoryType,
          'bitbucket': 'bitbucket' as TypesExternalRepositoryType,
        }

        let github: TypesGitHub | undefined
        let gitlab: TypesGitLab | undefined
        let azureDevOps: TypesAzureDevOps | undefined
        let bitbucket: TypesBitbucket | undefined

        if (patCredentials?.pat) {
          if (providerType === 'github') {
            github = { personal_access_token: patCredentials.pat, base_url: patCredentials.githubBaseUrl }
          } else if (providerType === 'gitlab') {
            gitlab = { personal_access_token: patCredentials.pat, base_url: patCredentials.gitlabBaseUrl }
          } else if (providerType === 'azure-devops') {
            azureDevOps = { organization_url: patCredentials.orgUrl || '', personal_access_token: patCredentials.pat }
          } else if (providerType === 'bitbucket') {
            bitbucket = { username: patCredentials.username || '', app_password: patCredentials.pat, base_url: patCredentials.bitbucketBaseUrl }
          }
        }

        const repoResponse = await apiClient.v1GitRepositoriesCreate({
          name: repo.name || projectName.trim(),
          description: repo.description || projectDescription.trim(),
          owner_id: account.user?.id || '',
          organization_id: orgId,
          repo_type: 'code' as any,
          default_branch: repo.default_branch || 'main',
          is_external: true,
          external_url: repo.clone_url || repo.html_url || '',
          external_type: externalTypeMap[providerType] || ('github' as TypesExternalRepositoryType),
          kodit_indexing: true,
          github,
          gitlab,
          azure_devops: azureDevOps,
          bitbucket,
          oauth_connection_id: oauthConnectionId,
        })
        repoId = repoResponse.data?.id || ''
      }

      if (!repoId) {
        snackbar.error('Failed to create repository')
        setCreatingProject(false)
        return
      }

      const projectResponse = await apiClient.v1ProjectsCreate({
        name: projectName.trim(),
        description: projectDescription.trim(),
        default_repo_id: repoId,
        organization_id: orgId,
        default_helix_app_id: agentId,
      })

      if (projectResponse.data?.id) {
        setCreatedProjectId(projectResponse.data.id)
        if (agentMode === 'select') {
          setCreatedAgentId(selectedAgentId)
        }
        markComplete(3)
      }
    } catch (err: any) {
      console.error('Failed to create project:', err)
      const msg = err?.response?.data?.message || err?.message || 'Failed to create project'
      snackbar.error(msg)
    } finally {
      setCreatingProject(false)
    }
  }, [projectName, projectDescription, repoMode, linkedExternalRepo, createdOrgId, account, api, apps, markComplete, snackbar, agentMode, selectedAgentId, selectedModel, selectedProvider, codeAgentRuntime, newAgentName])

  // Step 4: Create task
  const handleCreateTask = useCallback(async () => {
    if (!taskPrompt.trim()) {
      snackbar.error('Please describe what you want to build')
      return
    }
    if (!createdProjectId) {
      snackbar.error('No project available')
      return
    }
    setCreatingTask(true)
    try {
      const response = await api.getApiClient().v1SpecTasksFromPromptCreate({
        prompt: taskPrompt.trim(),
        project_id: createdProjectId,
        app_id: createdAgentId || undefined,
      })

      if (response.data) {
        markComplete(4)
        snackbar.success('Task created! Your AI agent will start working on it.')
        setTimeout(() => handleComplete(), 1500)
      }
    } catch (err: any) {
      console.error('Failed to create task:', err)
      const msg = err?.response?.data?.message || err?.message || 'Failed to create task'
      snackbar.error(msg)
    } finally {
      setCreatingTask(false)
    }
  }, [taskPrompt, createdProjectId, createdAgentId, api, markComplete, snackbar, handleComplete])

  const userName = account.user?.name?.split(' ')[0] || account.user?.email?.split('@')[0] || 'there'

  const isStepCompleted = (step: number) => completedSteps.has(step)
  const isStepActive = (step: number) => activeStep === step
  const isStepLocked = (step: number) => step > activeStep

  const renderStepIcon = (step: number) => {
    if (isStepCompleted(step)) {
      return (
        <CheckCircleIcon
          sx={{
            fontSize: 24,
            color: ACCENT,
            filter: `drop-shadow(0 0 6px ${ACCENT}60)`,
          }}
        />
      )
    }
    return (
      <RadioButtonUncheckedIcon
        sx={{
          fontSize: 24,
          color: isStepActive(step) ? ACCENT : 'rgba(255,255,255,0.15)',
        }}
      />
    )
  }

  const renderStepContent = (step: number) => {
    switch (step) {
      case 1:
        return (
          <Fade in={isStepActive(1)} timeout={400}>
            <Box sx={{ mt: 2.5 }}>
              <Box sx={{ display: 'grid', gridTemplateColumns: 'repeat(2, 1fr)', gap: 1.5, mb: 2.5 }}>
                {PROVIDERS.map((prov) => {
                  const isConnected = connectedProviderIds.has(prov.id)
                  const Logo = prov.logo
                  return (
                    <Box
                      key={prov.id}
                      onClick={() => {
                        setSelectedOnboardingProvider(prov)
                        setAddProviderDialogOpen(true)
                      }}
                      sx={{
                        p: 1.5,
                        borderRadius: 1.5,
                        border: `1px solid ${isConnected ? CARD_BORDER_ACTIVE : CARD_BORDER}`,
                        bgcolor: isConnected ? ACCENT_DIM : 'transparent',
                        cursor: 'pointer',
                        transition: 'all 0.2s',
                        '&:hover': { borderColor: 'rgba(255,255,255,0.15)', bgcolor: 'rgba(255,255,255,0.02)' },
                        display: 'flex',
                        alignItems: 'center',
                        gap: 1.5,
                      }}
                    >
                      <Box sx={{ width: 28, height: 28, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}>
                        {typeof Logo === 'string' ? (
                          <img src={Logo} alt="" style={{ width: 24, height: 24 }} />
                        ) : (
                          <Logo style={{ width: 24, height: 24 }} />
                        )}
                      </Box>
                      <Box sx={{ flex: 1, minWidth: 0 }}>
                        <Typography sx={{ color: '#fff', fontWeight: 500, fontSize: '0.78rem' }}>
                          {prov.name}
                        </Typography>
                      </Box>
                      {isConnected && (
                        <CheckCircleIcon sx={{ fontSize: 16, color: ACCENT, flexShrink: 0 }} />
                      )}
                    </Box>
                  )
                })}
              </Box>

              <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'center' }}>
                <Button
                  variant="contained"
                  onClick={() => markComplete(1)}
                  disabled={!hasUserProviders}
                  sx={btnSx}
                >
                  Continue
                </Button>
                <Button
                  variant="text"
                  onClick={() => markComplete(1)}
                  sx={{
                    color: 'rgba(255,255,255,0.3)',
                    textTransform: 'none',
                    fontSize: '0.78rem',
                    '&:hover': { color: 'rgba(255,255,255,0.6)' },
                  }}
                >
                  I'll do this later
                </Button>
              </Box>
            </Box>
          </Fade>
        )

      case 2:
        return (
          <Fade in={isStepActive(2)} timeout={400}>
            <Box sx={{ mt: 2.5 }}>
              {hasExistingOrgs && (
                <Box sx={{ display: 'flex', gap: 1.5, mb: 2.5 }}>
                  <Box
                    onClick={() => setOrgMode('select')}
                    sx={{
                      flex: 1,
                      p: 1.5,
                      borderRadius: 1.5,
                      border: `1px solid ${orgMode === 'select' ? CARD_BORDER_ACTIVE : CARD_BORDER}`,
                      bgcolor: orgMode === 'select' ? ACCENT_DIM : 'transparent',
                      cursor: 'pointer',
                      transition: 'all 0.2s',
                      '&:hover': { borderColor: 'rgba(255,255,255,0.15)' },
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.8, mb: 0.3 }}>
                      <BusinessIcon sx={{ fontSize: 16, color: orgMode === 'select' ? ACCENT : 'rgba(255,255,255,0.4)' }} />
                      <Typography sx={{ color: '#fff', fontWeight: 500, fontSize: '0.78rem' }}>
                        Existing organization
                      </Typography>
                    </Box>
                    <Typography sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.7rem' }}>
                      Use one of your organizations
                    </Typography>
                  </Box>
                  <Box
                    onClick={() => setOrgMode('create')}
                    sx={{
                      flex: 1,
                      p: 1.5,
                      borderRadius: 1.5,
                      border: `1px solid ${orgMode === 'create' ? CARD_BORDER_ACTIVE : CARD_BORDER}`,
                      bgcolor: orgMode === 'create' ? ACCENT_DIM : 'transparent',
                      cursor: 'pointer',
                      transition: 'all 0.2s',
                      '&:hover': { borderColor: 'rgba(255,255,255,0.15)' },
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.8, mb: 0.3 }}>
                      <CreateNewFolderIcon sx={{ fontSize: 16, color: orgMode === 'create' ? ACCENT : 'rgba(255,255,255,0.4)' }} />
                      <Typography sx={{ color: '#fff', fontWeight: 500, fontSize: '0.78rem' }}>
                        New organization
                      </Typography>
                    </Box>
                    <Typography sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.7rem' }}>
                      Create a new organization
                    </Typography>
                  </Box>
                </Box>
              )}

              {orgMode === 'select' && hasExistingOrgs ? (
                <>
                  <FormControl fullWidth size="small" sx={{ mb: 2.5 }}>
                    <InputLabel
                      sx={{
                        color: 'rgba(255,255,255,0.4)',
                        fontSize: '0.82rem',
                        '&.Mui-focused': { color: ACCENT },
                      }}
                    >
                      Organization
                    </InputLabel>
                    <Select
                      value={selectedOrgId}
                      onChange={e => setSelectedOrgId(e.target.value)}
                      label="Organization"
                      sx={{
                        color: '#fff',
                        fontSize: '0.82rem',
                        '& fieldset': { borderColor: 'rgba(255,255,255,0.1)' },
                        '&:hover fieldset': { borderColor: 'rgba(255,255,255,0.2)' },
                        '&.Mui-focused fieldset': { borderColor: ACCENT },
                        '& .MuiSvgIcon-root': { color: 'rgba(255,255,255,0.4)' },
                      }}
                      MenuProps={{
                        PaperProps: {
                          sx: {
                            bgcolor: '#1a1a2e',
                            color: '#fff',
                            maxHeight: 280,
                          },
                        },
                      }}
                    >
                      {existingOrgs.map((org) => (
                        <MenuItem key={org.id} value={org.id} sx={{ fontSize: '0.82rem' }}>
                          {org.display_name || org.name}
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <Button
                    variant="contained"
                    onClick={handleSelectExistingOrg}
                    disabled={!selectedOrgId}
                    sx={btnSx}
                    startIcon={<BusinessIcon sx={{ fontSize: 16 }} />}
                  >
                    Continue with this organization
                  </Button>
                </>
              ) : (
                <>
                  <TextField
                    fullWidth
                    size="small"
                    label="Organization name"
                    placeholder="My Company"
                    value={orgDisplayName}
                    onChange={e => setOrgDisplayName(e.target.value)}
                    variant="outlined"
                    sx={{ mb: 2.5 }}
                    InputProps={{ sx: inputSx }}
                    InputLabelProps={{ sx: labelSx }}
                  />
                  <Button
                    variant="contained"
                    onClick={handleCreateOrg}
                    disabled={createOrgMutation.isPending || !orgDisplayName.trim()}
                    sx={btnSx}
                    startIcon={createOrgMutation.isPending ? <CircularProgress size={14} sx={{ color: '#000' }} /> : <BusinessIcon sx={{ fontSize: 16 }} />}
                  >
                    {createOrgMutation.isPending ? 'Creating...' : 'Create organization'}
                  </Button>
                </>
              )}
            </Box>
          </Fade>
        )

      case 3:
        return (
          <Fade in={isStepActive(3)} timeout={400}>
            <Box sx={{ mt: 2.5 }}>
              <TextField
                fullWidth
                size="small"
                label="Project name"
                placeholder="my-awesome-project"
                value={projectName}
                onChange={e => setProjectName(e.target.value)}
                variant="outlined"
                sx={{ mb: 1.5 }}
                InputProps={{ sx: inputSx }}
                InputLabelProps={{ sx: labelSx }}
              />
              <TextField
                fullWidth
                size="small"
                label="Description (optional)"
                placeholder="A brief description of your project"
                value={projectDescription}
                onChange={e => setProjectDescription(e.target.value)}
                variant="outlined"
                multiline
                rows={2}
                sx={{ mb: 2 }}
                InputProps={{ sx: inputSx }}
                InputLabelProps={{ sx: labelSx }}
              />

              <Typography sx={{ color: 'rgba(255,255,255,0.4)', fontSize: '0.75rem', mb: 1 }}>
                Repository
              </Typography>
              <Box sx={{ display: 'flex', gap: 1.5, mb: 2 }}>
                <Box
                  onClick={() => setRepoMode('new')}
                  sx={{
                    flex: 1,
                    p: 1.5,
                    borderRadius: 1.5,
                    border: `1px solid ${repoMode === 'new' ? CARD_BORDER_ACTIVE : CARD_BORDER}`,
                    bgcolor: repoMode === 'new' ? ACCENT_DIM : 'transparent',
                    cursor: 'pointer',
                    transition: 'all 0.2s',
                    '&:hover': { borderColor: 'rgba(255,255,255,0.15)' },
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.8, mb: 0.3 }}>
                    <CreateNewFolderIcon sx={{ fontSize: 16, color: repoMode === 'new' ? ACCENT : 'rgba(255,255,255,0.4)' }} />
                    <Typography sx={{ color: '#fff', fontWeight: 500, fontSize: '0.78rem' }}>
                      New repository
                    </Typography>
                  </Box>
                  <Typography sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.7rem' }}>
                    Start fresh with an empty repository
                  </Typography>
                </Box>
                <Box
                  onClick={() => {
                    setRepoMode('external')
                    setLinkRepoDialogOpen(true)
                  }}
                  sx={{
                    flex: 1,
                    p: 1.5,
                    borderRadius: 1.5,
                    border: `1px solid ${repoMode === 'external' ? CARD_BORDER_ACTIVE : CARD_BORDER}`,
                    bgcolor: repoMode === 'external' ? ACCENT_DIM : 'transparent',
                    cursor: 'pointer',
                    transition: 'all 0.2s',
                    '&:hover': { borderColor: 'rgba(255,255,255,0.15)' },
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.8, mb: 0.3 }}>
                    <GitHubIcon sx={{ fontSize: 16, color: repoMode === 'external' ? ACCENT : 'rgba(255,255,255,0.4)' }} />
                    <Typography sx={{ color: '#fff', fontWeight: 500, fontSize: '0.78rem' }}>
                      External repository
                    </Typography>
                  </Box>
                  <Typography sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.7rem' }}>
                    Connect a GitHub repository
                  </Typography>
                </Box>
              </Box>

              {repoMode === 'external' && linkedExternalRepo && (
                <Box sx={{ mb: 2 }}>
                  <Box sx={{
                    px: 1.5,
                    py: 1,
                    borderRadius: 1.5,
                    bgcolor: 'rgba(0,232,145,0.04)',
                    border: `1px solid ${CARD_BORDER_ACTIVE}`,
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'space-between',
                  }}>
                    <Box>
                      <Typography sx={{ color: '#fff', fontSize: '0.78rem', fontWeight: 500 }}>
                        {linkedExternalRepo.repo.full_name || linkedExternalRepo.repo.name}
                      </Typography>
                      <Typography sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.7rem' }}>
                        {linkedExternalRepo.repo.clone_url || linkedExternalRepo.repo.html_url}
                      </Typography>
                    </Box>
                    <Button
                      size="small"
                      onClick={() => setLinkRepoDialogOpen(true)}
                      sx={{
                        color: 'rgba(255,255,255,0.4)',
                        textTransform: 'none',
                        fontSize: '0.72rem',
                        minWidth: 'auto',
                        '&:hover': { color: '#fff' },
                      }}
                    >
                      Change
                    </Button>
                  </Box>
                </Box>
              )}

              <Divider sx={{ borderColor: 'rgba(255,255,255,0.06)', my: 1.5 }} />

              <Typography sx={{ color: 'rgba(255,255,255,0.4)', fontSize: '0.75rem', mb: 2 }}>
                AI Agent
              </Typography>

              {zedExternalAgents.length > 0 && (
                <Box sx={{ display: 'flex', gap: 1.5, mb: 2 }}>
                  <Box
                    onClick={() => setAgentMode('select')}
                    sx={{
                      flex: 1,
                      p: 1.5,
                      borderRadius: 1.5,
                      border: `1px solid ${agentMode === 'select' ? CARD_BORDER_ACTIVE : CARD_BORDER}`,
                      bgcolor: agentMode === 'select' ? ACCENT_DIM : 'transparent',
                      cursor: 'pointer',
                      transition: 'all 0.2s',
                      '&:hover': { borderColor: 'rgba(255,255,255,0.15)' },
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.8, mb: 0.3 }}>
                      <SmartToyIcon sx={{ fontSize: 16, color: agentMode === 'select' ? ACCENT : 'rgba(255,255,255,0.4)' }} />
                      <Typography sx={{ color: '#fff', fontWeight: 500, fontSize: '0.78rem' }}>
                        Existing agent
                      </Typography>
                    </Box>
                    <Typography sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.7rem' }}>
                      Use one of your agents
                    </Typography>
                  </Box>
                  <Box
                    onClick={() => setAgentMode('create')}
                    sx={{
                      flex: 1,
                      p: 1.5,
                      borderRadius: 1.5,
                      border: `1px solid ${agentMode === 'create' ? CARD_BORDER_ACTIVE : CARD_BORDER}`,
                      bgcolor: agentMode === 'create' ? ACCENT_DIM : 'transparent',
                      cursor: 'pointer',
                      transition: 'all 0.2s',
                      '&:hover': { borderColor: 'rgba(255,255,255,0.15)' },
                    }}
                  >
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.8, mb: 0.3 }}>
                      <CreateNewFolderIcon sx={{ fontSize: 16, color: agentMode === 'create' ? ACCENT : 'rgba(255,255,255,0.4)' }} />
                      <Typography sx={{ color: '#fff', fontWeight: 500, fontSize: '0.78rem' }}>
                        New agent
                      </Typography>
                    </Box>
                    <Typography sx={{ color: 'rgba(255,255,255,0.3)', fontSize: '0.7rem' }}>
                      Create a new AI agent
                    </Typography>
                  </Box>
                </Box>
              )}

              {agentMode === 'select' && zedExternalAgents.length > 0 ? (
                <FormControl fullWidth size="small" sx={{ mb: 2 }}>
                  <InputLabel
                    sx={{
                      color: 'rgba(255,255,255,0.4)',
                      fontSize: '0.82rem',
                      '&.Mui-focused': { color: ACCENT },
                    }}
                  >
                    Select Agent
                  </InputLabel>
                  <Select
                    value={selectedAgentId}
                    onChange={e => setSelectedAgentId(e.target.value)}
                    label="Select Agent"
                    sx={{
                      color: '#fff',
                      fontSize: '0.82rem',
                      '& fieldset': { borderColor: 'rgba(255,255,255,0.1)' },
                      '&:hover fieldset': { borderColor: 'rgba(255,255,255,0.2)' },
                      '&.Mui-focused fieldset': { borderColor: ACCENT },
                      '& .MuiSvgIcon-root': { color: 'rgba(255,255,255,0.4)' },
                    }}
                    renderValue={(value) => {
                      const app = zedExternalAgents.find((a: IApp) => a.id === value)
                      return app?.config?.helix?.name || 'Select Agent'
                    }}
                    MenuProps={{
                      PaperProps: {
                        sx: {
                          bgcolor: '#1a1a2e',
                          color: '#fff',
                          maxHeight: 280,
                        },
                      },
                    }}
                  >
                    {zedExternalAgents.map((app: IApp) => (
                      <MenuItem key={app.id} value={app.id} sx={{ fontSize: '0.82rem' }}>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                          <Bot size={16} color="rgba(255,255,255,0.4)" />
                          <span>{app.config?.helix?.name || 'Unnamed Agent'}</span>
                        </Box>
                      </MenuItem>
                    ))}
                  </Select>
                </FormControl>
              ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1.5, mb: 2 }}>
                  <FormControl fullWidth size="small">
                    <InputLabel
                      sx={{
                        color: 'rgba(255,255,255,0.4)',
                        fontSize: '0.82rem',
                        '&.Mui-focused': { color: ACCENT },
                      }}
                    >
                      Code Agent Runtime
                    </InputLabel>
                    <Select
                      value={codeAgentRuntime}
                      onChange={e => setCodeAgentRuntime(e.target.value as CodeAgentRuntime)}
                      label="Code Agent Runtime"
                      sx={{
                        color: '#fff',
                        fontSize: '0.82rem',
                        '& fieldset': { borderColor: 'rgba(255,255,255,0.1)' },
                        '&:hover fieldset': { borderColor: 'rgba(255,255,255,0.2)' },
                        '&.Mui-focused fieldset': { borderColor: ACCENT },
                        '& .MuiSvgIcon-root': { color: 'rgba(255,255,255,0.4)' },
                      }}
                      MenuProps={{
                        PaperProps: {
                          sx: {
                            bgcolor: '#1a1a2e',
                            color: '#fff',
                          },
                        },
                      }}
                    >
                      <MenuItem value="zed_agent" sx={{ fontSize: '0.82rem' }}>
                        <Box>
                          <Typography sx={{ fontSize: '0.82rem', color: '#fff' }}>Zed Agent (Built-in)</Typography>
                          <Typography sx={{ fontSize: '0.7rem', color: 'rgba(255,255,255,0.3)' }}>
                            Uses Zed's native agent panel
                          </Typography>
                        </Box>
                      </MenuItem>
                      <MenuItem value="qwen_code" sx={{ fontSize: '0.82rem' }}>
                        <Box>
                          <Typography sx={{ fontSize: '0.82rem', color: '#fff' }}>Qwen Code</Typography>
                          <Typography sx={{ fontSize: '0.7rem', color: 'rgba(255,255,255,0.3)' }}>
                            Uses qwen-code CLI as a custom agent server
                          </Typography>
                        </Box>
                      </MenuItem>
                    </Select>
                  </FormControl>

                  <AdvancedModelPicker
                    recommendedModels={RECOMMENDED_MODELS}
                    autoSelectFirst={false}
                    hint="Choose a capable model for agentic coding."
                    selectedProvider={selectedProvider}
                    selectedModelId={selectedModel}
                    onSelectModel={(provider, model) => {
                      setHasUserSelectedModel(true)
                      setSelectedProvider(provider)
                      setSelectedModel(model)
                    }}
                    currentType="text"
                    displayMode="full"
                    disabled={creatingAgent}
                  />

                  <TextField
                    fullWidth
                    size="small"
                    label="Agent Name"
                    value={newAgentName}
                    onChange={e => {
                      setNewAgentName(e.target.value)
                      setUserModifiedAgentName(true)
                    }}
                    variant="outlined"
                    InputProps={{ sx: inputSx }}
                    InputLabelProps={{ sx: labelSx }}
                    FormHelperTextProps={{ sx: helperSx }}
                    helperText="Auto-generated from model and runtime"
                  />
                </Box>
              )}

              <Button
                variant="contained"
                onClick={handleCreateProject}
                disabled={
                  creatingProject || creatingAgent || !projectName.trim() || !createdOrgId ||
                  (repoMode === 'external' && !linkedExternalRepo) ||
                  (agentMode === 'select' && !selectedAgentId) ||
                  (agentMode === 'create' && !selectedModel)
                }
                sx={btnSx}
                startIcon={(creatingProject || creatingAgent) ? <CircularProgress size={14} sx={{ color: '#000' }} /> : <FolderIcon sx={{ fontSize: 16 }} />}
              >
                {creatingAgent ? 'Creating agent...' : creatingProject ? 'Creating project...' : 'Create project'}
              </Button>
            </Box>
          </Fade>
        )

      case 4:
        return (
          <Fade in={isStepActive(4)} timeout={400}>
            <Box sx={{ mt: 2.5 }}>
              <TextField
                fullWidth
                size="small"
                label="What would you like to build?"
                placeholder="e.g., Create a REST API with user authentication and CRUD operations for a todo app"
                value={taskPrompt}
                onChange={e => setTaskPrompt(e.target.value)}
                variant="outlined"
                multiline
                rows={3}
                sx={{ mb: 2 }}
                InputProps={{ sx: inputSx }}
                InputLabelProps={{ sx: labelSx }}
              />

              <Box sx={{ display: 'flex', gap: 1.5, alignItems: 'center' }}>
                <Button
                  variant="contained"
                  onClick={handleCreateTask}
                  disabled={creatingTask || !taskPrompt.trim()}
                  sx={btnSx}
                  startIcon={creatingTask ? <CircularProgress size={14} sx={{ color: '#000' }} /> : <RocketLaunchIcon sx={{ fontSize: 16 }} />}
                >
                  {creatingTask ? 'Creating...' : 'Create task'}
                </Button>
                <Button
                  variant="text"
                  onClick={handleComplete}
                  sx={{
                    color: 'rgba(255,255,255,0.3)',
                    textTransform: 'none',
                    fontSize: '0.78rem',
                    '&:hover': { color: 'rgba(255,255,255,0.6)' },
                  }}
                >
                  Skip this step
                </Button>
              </Box>
            </Box>
          </Fade>
        )

      default:
        return null
    }
  }

  return (
    <Box
      sx={{
        position: 'fixed',
        inset: 0,
        bgcolor: BG,
        zIndex: 1300,
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'flex-start',
        overflowY: 'auto',
        pt: { xs: 4, md: 6 },
        pb: 6,
      }}
    >
      <Box
        sx={{
          width: '100%',
          maxWidth: 580,
          px: { xs: 2, md: 0 },
        }}
      >
        {/* Header */}
        <Fade in timeout={600}>
          <Box sx={{ mb: 5 }}>
            <Typography
              sx={{
                color: '#fff',
                fontWeight: 700,
                mb: 0.5,
                fontSize: { xs: '1.5rem', md: '1.8rem' },
                letterSpacing: '-0.02em',
              }}
            >
              Hello, {userName}
            </Typography>
            <Typography
              sx={{
                color: 'rgba(255,255,255,0.35)',
                fontSize: '0.88rem',
              }}
            >
              Let's set up for success 😉
            </Typography>
          </Box>
        </Fade>

        {/* Steps */}
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
          {STEPS.map((step, index) => {
            const completed = isStepCompleted(index)
            const active = isStepActive(index)
            const locked = isStepLocked(index)

            return (
              <Fade in timeout={600 + index * 150} key={index}>
                <Box
                  sx={{
                    px: { xs: 2.5, md: 3 },
                    py: { xs: 2, md: 2.5 },
                    borderRadius: 2,
                    border: `1px solid ${active ? CARD_BORDER_ACTIVE : CARD_BORDER}`,
                    bgcolor: active ? CARD_BG_ACTIVE : completed ? CARD_BG : 'transparent',
                    transition: 'all 0.3s ease',
                    opacity: locked ? 0.35 : 1,
                    ...(active && {
                      boxShadow: `0 0 24px rgba(0, 232, 145, 0.04)`,
                    }),
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1.5 }}>
                    {renderStepIcon(index)}
                    <Box sx={{ flex: 1 }}>
                      <Typography
                        sx={{
                          color: completed || active ? '#fff' : 'rgba(255,255,255,0.35)',
                          fontWeight: 600,
                          fontSize: '0.88rem',
                        }}
                      >
                        {step.title}
                      </Typography>
                      <Typography
                        sx={{
                          color: 'rgba(255,255,255,0.28)',
                          fontSize: '0.76rem',
                          mt: 0.2,
                        }}
                      >
                        {step.subtitle}
                      </Typography>
                    </Box>
                  </Box>

                  {active && index > 0 && renderStepContent(index)}
                </Box>
              </Fade>
            )
          })}
        </Box>

        <BrowseProvidersDialog
          open={linkRepoDialogOpen}
          onClose={() => setLinkRepoDialogOpen(false)}
          onSelectRepository={handleBrowseSelectRepository}
          isLinking={linkingRepo}
        />

        {selectedOnboardingProvider && (
          <AddProviderDialog
            open={addProviderDialogOpen}
            onClose={() => setAddProviderDialogOpen(false)}
            onClosed={() => setSelectedOnboardingProvider(null)}
            orgId=""
            provider={selectedOnboardingProvider}
          />
        )}

        {/* All done message */}
        {completedSteps.size === STEPS.length && (
          <Fade in timeout={600}>
            <Box sx={{ mt: 4, textAlign: 'center' }}>
              <Typography sx={{ color: ACCENT, fontWeight: 600, fontSize: '0.95rem', mb: 1.5 }}>
                You're all set!
              </Typography>
              <Button
                variant="contained"
                onClick={handleComplete}
                sx={{
                  ...btnSx,
                  px: 4,
                  py: 1,
                  fontSize: '0.85rem',
                }}
              >
                Go to your workspace
              </Button>
            </Box>
          </Fade>
        )}
      </Box>
    </Box>
  )
}
