import React, { FC, useEffect, useState } from 'react'
import {
  Box,
  Button,
  CircularProgress,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from '@mui/material'
import { ChevronDown, Folder, FolderPlus, Hammer, ListTodo, MessageCircle } from 'lucide-react'

import {
  TypesCodeAgentOverrides,
  TypesSandboxResourceOverrides,
  TypesSandboxRuntime,
} from '../api/api'
import AdvancedModelPicker from '../components/create/AdvancedModelPicker'
import RobustPromptInput from '../components/common/RobustPromptInput'
import SpecTaskExecutionControls from '../components/tasks/SpecTaskExecutionControls'
import ManagedCreateProjectDialog from '../components/project/ManagedCreateProjectDialog'
import Page from '../components/system/Page'
import { useAccount } from '../contexts/account'
import { useStreaming } from '../contexts/streaming'
import { getBrowserLocale } from '../hooks/useBrowserLocale'
import useLightTheme from '../hooks/useLightTheme'
import useApps from '../hooks/useApps'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { codeAgentExecutionConfigFromApp } from '../utils/codeAgentExecutionConfig'
import { useListProjectSpecTaskAgents, useListProjects } from '../services'
import { useListProviders } from '../services/providersService'
import { invalidateSessionsQuery } from '../services/sessionService'
import {
  SPEC_TASK_ATTACHMENT_ACCEPTED_MIME,
  SPEC_TASK_ATTACHMENT_MAX_BYTES,
  SPEC_TASK_ATTACHMENT_MAX_PER_TASK,
  useUploadSpecTaskAttachments,
} from '../services/specTaskAttachmentsService'
import {
  useCreateSpecTaskFromPrompt,
  useStartSpecTaskPlanning,
} from '../services/specTaskService'
import { SESSION_TYPE_TEXT } from '../types'
import { useQueryClient } from '@tanstack/react-query'
import {
  buildNewChatTaskRequest,
  chooseProjectChatAgentId,
  modelSupportsReasoningEffort,
  NEW_CHAT_REASONING_EFFORT_OPTIONS,
  newChatHeading,
  NewChatReasoningEffort,
  NewChatTaskMode,
  projectChatAgentStorageKey,
  readNewChatReasoningEffort,
} from './newChatLogic'
import {
  preferredSpecTaskSandboxRuntime,
  saveSpecTaskSandboxRuntimePreference,
} from '../utils/specTaskSandboxRuntime'

const T3_FONT_FAMILY = '-apple-system, BlinkMacSystemFont, "Segoe UI", system-ui, sans-serif'
const TASK_ATTACHMENT_ACCEPT = Object.entries(SPEC_TASK_ATTACHMENT_ACCEPTED_MIME)
  .flatMap(([mime, extensions]) => [mime, ...extensions])
  .join(',')
const TASK_ATTACHMENT_MIME_TYPES = new Set(Object.keys(SPEC_TASK_ATTACHMENT_ACCEPTED_MIME))
const TASK_ATTACHMENT_EXTENSIONS = Object.values(SPEC_TASK_ATTACHMENT_ACCEPTED_MIME).flat()

const selectorButtonSx = {
  minWidth: 0,
  height: 28,
  px: 0.75,
  borderRadius: 1,
  color: 'text.secondary',
  fontSize: '0.75rem',
  fontWeight: 500,
  lineHeight: 1,
  textTransform: 'none',
  '& .MuiButton-startIcon': {
    ml: 0,
    mr: 0.625,
  },
  '& .MuiButton-endIcon': {
    ml: 0.375,
    mr: 0,
  },
  '&:hover': {
    color: 'text.primary',
    backgroundColor: 'action.hover',
  },
}

function taskAttachmentValidation(file: File): string | null {
  if (TASK_ATTACHMENT_MIME_TYPES.has(file.type)) return null
  const lowerName = file.name.toLowerCase()
  if (TASK_ATTACHMENT_EXTENSIONS.some((extension) => lowerName.endsWith(extension))) return null
  return 'file type is not supported for task attachments'
}

function errorMessage(error: any, fallback: string): string {
  const responseData = error?.response?.data
  if (typeof responseData === 'string') return responseData
  return responseData?.message || responseData?.error || error?.message || fallback
}

const Home: FC = () => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const router = useRouter()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const apps = useApps()
  const { NewInference } = useStreaming()
  const orgId = account.organizationTools.organization?.id || ''
  const requestedProjectId = router.params.project_id || ''
  const userId = account.user?.id || ''

  const { data: projects = [], isLoading: projectsLoading } = useListProjects(orgId, {
    enabled: !!userId && !!orgId,
  })
  const { data: providers = [] } = useListProviders({
    loadModels: true,
    orgId,
    enabled: !!userId && !!orgId,
  })
  const selectedProject = projects.find((project) => project.id === requestedProjectId)
  const selectedProjectId = selectedProject?.id || ''
  const { data: codingAgents = [] } = useListProjectSpecTaskAgents(
    selectedProjectId,
    !!userId && !!selectedProjectId,
  )

  const [selectedProvider, setSelectedProvider] = useState(() => localStorage.getItem('helix_provider') || '')
  const [selectedModel, setSelectedModel] = useState(() => localStorage.getItem('helix_model') || '')
  const [reasoningEffort, setReasoningEffort] = useState<NewChatReasoningEffort>(() => (
    readNewChatReasoningEffort(localStorage.getItem('helix_reasoning_effort'))
  ))
  const [selectedAgentId, setSelectedAgentId] = useState('')
  const [taskCodeAgentOverrides, setTaskCodeAgentOverrides] = useState<TypesCodeAgentOverrides>({})
  const [taskSandboxResources, setTaskSandboxResources] = useState<TypesSandboxResourceOverrides>({
    vcpus: 4,
    memory_mb: 8192,
  })
  const [taskSandboxRuntime, setTaskSandboxRuntime] = useState<TypesSandboxRuntime>(() =>
    preferredSpecTaskSandboxRuntime(requestedProjectId),
  )
  const [taskMode, setTaskMode] = useState<NewChatTaskMode>('build')
  const [modeMenuAnchor, setModeMenuAnchor] = useState<HTMLElement | null>(null)
  const [effortMenuAnchor, setEffortMenuAnchor] = useState<HTMLElement | null>(null)
  const [projectMenuAnchor, setProjectMenuAnchor] = useState<HTMLElement | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [createProjectOpen, setCreateProjectOpen] = useState(false)

  const createTask = useCreateSpecTaskFromPrompt()
  const uploadTaskAttachments = useUploadSpecTaskAttachments()
  const startTask = useStartSpecTaskPlanning()

  useEffect(() => {
    if (!selectedProvider || !selectedModel) return
    localStorage.setItem('helix_provider', selectedProvider)
    localStorage.setItem('helix_model', selectedModel)
  }, [selectedProvider, selectedModel])

  useEffect(() => {
    localStorage.setItem('helix_reasoning_effort', reasoningEffort)
  }, [reasoningEffort])

  const codingAgentIds = codingAgents.flatMap((agent) => agent.id ? [agent.id] : []).join(',')
  const agentStorageKey = projectChatAgentStorageKey(orgId)

  useEffect(() => {
    const availableIds = codingAgentIds ? codingAgentIds.split(',') : []
    setSelectedAgentId(chooseProjectChatAgentId(
      availableIds,
      orgId ? localStorage.getItem(agentStorageKey) : null,
    ))
  }, [agentStorageKey, codingAgentIds, orgId])

  useEffect(() => {
    if (!orgId || !selectedAgentId) return
    const availableIds = codingAgentIds ? codingAgentIds.split(',') : []
    if (!availableIds.includes(selectedAgentId)) return
    localStorage.setItem(agentStorageKey, selectedAgentId)
  }, [agentStorageKey, codingAgentIds, orgId, selectedAgentId])

  useEffect(() => {
    setTaskCodeAgentOverrides({})
    setTaskSandboxResources({ vcpus: 4, memory_mb: 8192 })
    setTaskSandboxRuntime(preferredSpecTaskSandboxRuntime(
      selectedProjectId,
      selectedProject?.default_sandbox_runtime,
    ))
  }, [selectedProjectId, selectedProject?.default_sandbox_runtime])

  const handleTaskSandboxRuntimeChange = (runtime: TypesSandboxRuntime) => {
    setTaskSandboxRuntime(runtime)
    saveSpecTaskSandboxRuntimePreference(selectedProjectId, runtime)
  }

  const taskAgents = codingAgents.flatMap((summary) => {
    const app = apps.apps.find((candidate) => candidate.id === summary.id)
    return app ? [app] : []
  })
  const isProjectContext = !!selectedProjectId
  const supportsReasoningEffort = modelSupportsReasoningEffort(
    providers,
    selectedProvider,
    selectedModel,
  )

  const openProject = (projectId?: string) => {
    setProjectMenuAnchor(null)
    if (projectId) account.orgNavigate('chat', {}, { project_id: projectId })
    else account.orgNavigate('chat')
  }

  const handleNormalChat = async (message: string, _interrupt?: boolean, attachments: File[] = []) => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }

    setSubmitting(true)
    try {
      const session = await NewInference({
        regenerate: false,
        type: SESSION_TYPE_TEXT,
        message,
        messages: [],
        provider: selectedProvider,
        modelName: selectedModel,
        reasoningEffort: supportsReasoningEffort ? reasoningEffort : undefined,
        attachedImages: attachments,
        orgId,
      })
      if (!session?.id) return false
      invalidateSessionsQuery(queryClient)
      account.orgNavigate('session', { session_id: session.id })
      return true
    } catch (error) {
      snackbar.error(errorMessage(error, 'Failed to start chat'))
      return false
    } finally {
      setSubmitting(false)
    }
  }

  const handleProjectTask = async (message: string, _interrupt?: boolean, attachments: File[] = []) => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    if (!selectedProjectId) return false
    if (!selectedAgentId) {
      snackbar.error('Select a coding agent before starting this task')
      return false
    }

    setSubmitting(true)
    let taskId = ''
    try {
      const task = await createTask.mutateAsync(buildNewChatTaskRequest({
        mode: taskMode,
        projectId: selectedProjectId,
        prompt: message,
        codeAgentConfig: codeAgentExecutionConfigFromApp(
          taskAgents.find((agent) => agent.id === selectedAgentId),
          taskCodeAgentOverrides,
        ),
        sandboxResourceOverrides: taskSandboxResources,
        sandboxRuntime: taskSandboxRuntime,
      }))
      taskId = task?.id || ''
      if (!taskId) throw new Error('Task creation returned no task ID')

      if (attachments.length > 0) {
        try {
          await uploadTaskAttachments.mutateAsync({ taskId, files: attachments })
        } catch (error) {
          snackbar.error(errorMessage(error, 'Task created, but its attachments could not be uploaded'))
          account.orgNavigate('chat-task', { id: selectedProjectId, taskId })
          return true
        }
      }

      try {
        const { keyboardLayout, timezone } = getBrowserLocale()
        await startTask.mutateAsync({ taskId, keyboard: keyboardLayout, timezone })
      } catch (error) {
        snackbar.error(errorMessage(error, 'Task created, but it could not be started'))
        account.orgNavigate('chat-task', { id: selectedProjectId, taskId })
        return true
      }

      account.orgNavigate('chat-task', { id: selectedProjectId, taskId })
      return true
    } catch (error) {
      snackbar.error(errorMessage(error, 'Failed to create task'))
      if (taskId) account.orgNavigate('chat-task', { id: selectedProjectId, taskId })
      return !!taskId
    } finally {
      setSubmitting(false)
    }
  }

  const modeSelector = (
    <>
      <Tooltip title={taskMode === 'build' ? 'Start implementation immediately' : 'Start with planning'}>
        <Button
          disabled={submitting}
          startIcon={taskMode === 'build' ? <Hammer size={15} /> : <ListTodo size={15} />}
          onClick={(event) => setModeMenuAnchor(event.currentTarget)}
          sx={selectorButtonSx}
        >
          {taskMode === 'build' ? 'Build' : 'Plan'}
        </Button>
      </Tooltip>
      <Menu
        anchorEl={modeMenuAnchor}
        open={!!modeMenuAnchor}
        onClose={() => setModeMenuAnchor(null)}
      >
        <MenuItem
          selected={taskMode === 'plan'}
          onClick={() => {
            setTaskMode('plan')
            setModeMenuAnchor(null)
          }}
        >
          <ListItemIcon><ListTodo size={16} /></ListItemIcon>
          <ListItemText primary="Plan" secondary="Create specifications first" />
        </MenuItem>
        <MenuItem
          selected={taskMode === 'build'}
          onClick={() => {
            setTaskMode('build')
            setModeMenuAnchor(null)
          }}
        >
          <ListItemIcon><Hammer size={16} /></ListItemIcon>
          <ListItemText primary="Build" secondary="Go directly to implementation" />
        </MenuItem>
      </Menu>
    </>
  )

  const projectActions = (
    <Box sx={{ display: 'flex', alignItems: 'center', minWidth: 0, overflow: 'hidden' }}>
      <SpecTaskExecutionControls
        agents={taskAgents}
        selectedAgentId={selectedAgentId}
        codeAgentOverrides={taskCodeAgentOverrides}
        sandboxResourceOverrides={taskSandboxResources}
        sandboxRuntime={taskSandboxRuntime}
        onAgentModelChange={(agentId, overrides) => {
          setSelectedAgentId(agentId)
          setTaskCodeAgentOverrides(overrides)
        }}
        onSandboxResourceOverridesChange={setTaskSandboxResources}
        onSandboxRuntimeChange={handleTaskSandboxRuntimeChange}
        disabled={submitting}
        compact
      />
      <Box
        aria-hidden="true"
        sx={{
          width: '1px',
          height: 16,
          mx: 0.5,
          flexShrink: 0,
          bgcolor: 'divider',
          opacity: 0.65,
        }}
      />
      {modeSelector}
    </Box>
  )

  const modelSelector = (
    <AdvancedModelPicker
      selectedProvider={selectedProvider}
      selectedModelId={selectedModel}
      onSelectModel={(provider, model) => {
        setSelectedProvider(provider)
        setSelectedModel(model)
      }}
      currentType={SESSION_TYPE_TEXT}
      displayMode="short"
      buttonVariant="text"
    />
  )

  const effortSelector = supportsReasoningEffort ? (
    <>
      <Tooltip title={`Reasoning effort: ${reasoningEffort}`}>
        <Button
          disabled={submitting}
          endIcon={<ChevronDown size={13} />}
          onClick={(event) => setEffortMenuAnchor(event.currentTarget)}
          sx={selectorButtonSx}
        >
          {NEW_CHAT_REASONING_EFFORT_OPTIONS.find((option) => option.value === reasoningEffort)?.label}
        </Button>
      </Tooltip>
      <Menu
        anchorEl={effortMenuAnchor}
        open={!!effortMenuAnchor}
        onClose={() => setEffortMenuAnchor(null)}
      >
        {NEW_CHAT_REASONING_EFFORT_OPTIONS.map((option) => (
          <MenuItem
            key={option.value}
            selected={option.value === reasoningEffort}
            onClick={() => {
              setReasoningEffort(option.value)
              setEffortMenuAnchor(null)
            }}
          >
            <ListItemText primary={option.label} />
          </MenuItem>
        ))}
      </Menu>
    </>
  ) : null

  const modelActions = (
    <Box sx={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
      {modelSelector}
      {effortSelector && (
        <>
          <Box
            aria-hidden="true"
            sx={{ width: '1px', height: 16, mx: 0.5, flexShrink: 0, bgcolor: 'divider', opacity: 0.65 }}
          />
          {effortSelector}
        </>
      )}
    </Box>
  )

  if (projectsLoading) {
    return (
      <Page
        breadcrumbs={[{ title: 'None' }]}
        breadcrumbTitle="New thread"
        breadcrumbShowHome={false}
        disableContentScroll
        px={2}
      >
        <Box sx={{ height: '100%', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <CircularProgress size={24} />
        </Box>
      </Page>
    )
  }

  if (projects.length === 0) {
    return (
      <>
        <Page
          breadcrumbs={[{ title: 'Projects' }]}
          breadcrumbTitle="Get started"
          breadcrumbShowHome={false}
          disableContentScroll
          px={2}
        >
          <Box
            sx={{
              height: '100%',
              display: 'flex',
              alignItems: 'center',
              justifyContent: 'center',
              bgcolor: lightTheme.isLight ? '#f7f7f8' : '#080808',
              px: 3,
            }}
          >
            <Box sx={{ textAlign: 'center', maxWidth: 440 }}>
              <Box
                sx={{
                  width: 56,
                  height: 56,
                  mx: 'auto',
                  mb: 2,
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  borderRadius: 2,
                  bgcolor: 'action.selected',
                  color: 'text.secondary',
                }}
              >
                <FolderPlus size={28} />
              </Box>
              <Typography component="h1" variant="h5" sx={{ fontWeight: 600, mb: 1 }}>
                Get started by creating a new project
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Create a new or connect an existing repository
              </Typography>
              <Button
                variant="contained"
                color="secondary"
                startIcon={<FolderPlus size={18} />}
                onClick={() => setCreateProjectOpen(true)}
              >
                Create new project
              </Button>
            </Box>
          </Box>
        </Page>
        <ManagedCreateProjectDialog
          open={createProjectOpen}
          onClose={() => setCreateProjectOpen(false)}
          onSuccess={(projectId) => {
            setCreateProjectOpen(false)
            account.orgNavigate('chat', {}, { project_id: projectId })
          }}
        />
      </>
    )
  }

  return (
    <Page
      breadcrumbs={[{ title: selectedProject?.name || 'None' }]}
      breadcrumbTitle={isProjectContext ? 'New Task' : 'New thread'}
      breadcrumbShowHome={false}
      disableContentScroll
      px={2}
    >
      <Box
        sx={{
          height: '100%',
          minHeight: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          px: { xs: 2, sm: 3 },
          pb: { xs: 4, md: 12 },
          backgroundColor: lightTheme.isLight ? '#f7f7f8' : '#080808',
          fontFamily: T3_FONT_FAMILY,
          '& .MuiTypography-root, & .MuiButton-root': { fontFamily: 'inherit' },
        }}
      >
        <Box sx={{ width: '100%', maxWidth: 768 }}>
          <Typography
            component="h1"
            sx={{
              mb: 3.5,
              color: 'text.primary',
              fontSize: { xs: '1.65rem', sm: '1.9rem' },
              fontWeight: 560,
              lineHeight: 1.2,
              letterSpacing: '-0.025em',
              textAlign: 'center',
            }}
          >
            {newChatHeading(selectedProject?.name)}
          </Typography>

          {requestedProjectId && projectsLoading ? (
            <Box sx={{ height: 150, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <CircularProgress size={22} />
            </Box>
          ) : (
            <RobustPromptInput
              key={selectedProjectId || 'none'}
              sessionId={`new-thread:${selectedProjectId || 'none'}`}
              sendMode="direct"
              autoFocus
              disabled={submitting || (isProjectContext && !selectedAgentId)}
              placeholder={isProjectContext ? 'Describe what you want to build' : 'Ask anything'}
              inlineImageAttachments={!isProjectContext}
              deferredFileAttachments={isProjectContext}
              attachmentAccept={isProjectContext ? TASK_ATTACHMENT_ACCEPT : undefined}
              attachmentMaxBytes={isProjectContext ? SPEC_TASK_ATTACHMENT_MAX_BYTES : undefined}
              attachmentMaxCount={isProjectContext ? SPEC_TASK_ATTACHMENT_MAX_PER_TASK : undefined}
              validateAttachment={isProjectContext ? taskAttachmentValidation : undefined}
              leadingActions={isProjectContext ? projectActions : modelActions}
              onSend={isProjectContext ? handleProjectTask : handleNormalChat}
            />
          )}

          <Box sx={{ mt: 1, px: 2 }}>
            <Button
              startIcon={isProjectContext ? <Folder size={14} /> : <MessageCircle size={14} />}
              endIcon={<ChevronDown size={12} />}
              onClick={(event) => setProjectMenuAnchor(event.currentTarget)}
              sx={{ ...selectorButtonSx, fontSize: '0.7rem' }}
            >
              {selectedProject?.name || 'None'}
            </Button>
            <Menu
              anchorEl={projectMenuAnchor}
              open={!!projectMenuAnchor}
              onClose={() => setProjectMenuAnchor(null)}
            >
              <MenuItem selected={!isProjectContext} onClick={() => openProject()}>
                <ListItemIcon><MessageCircle size={16} /></ListItemIcon>
                <ListItemText primary="None" secondary="Start a normal chat" />
              </MenuItem>
              {projects.map((project) => (
                <MenuItem
                  key={project.id}
                  selected={project.id === selectedProjectId}
                  onClick={() => openProject(project.id)}
                >
                  <ListItemIcon><Folder size={16} /></ListItemIcon>
                  <ListItemText primary={project.name || 'Untitled project'} />
                </MenuItem>
              ))}
            </Menu>
          </Box>
        </Box>
      </Box>
    </Page>
  )
}

export default Home
