import React, { FC, useEffect, useState } from 'react'
import {
  Box,
  Button,
  Checkbox,
  CircularProgress,
  FormControlLabel,
  ListItemIcon,
  ListItemText,
  Menu,
  MenuItem,
  Tooltip,
  Typography,
} from '@mui/material'
import { ChevronDown, Folder, FolderPlus, Hammer, ListTodo } from 'lucide-react'

import {
  TypesCodeAgentExecutionConfig,
  TypesSandboxResourceOverrides,
  TypesSandboxRuntime,
} from '../api/api'
import RobustPromptInput from '../components/common/RobustPromptInput'
import ChatWelcome, { WELCOME_FONT_FAMILY } from '../components/session/ChatWelcome'
import CodeAgentExecutionControls from '../components/agent/CodeAgentExecutionControls'
import { useSeedProjectCodeAgentConfig } from '../hooks/useSeedProjectCodeAgentConfig'
import { CodeAgentConfigChangeSource } from '../utils/codeAgentExecutionConfig'
import ManagedCreateProjectDialog from '../components/project/ManagedCreateProjectDialog'
import Page from '../components/system/Page'
import { useAccount } from '../contexts/account'
import { getBrowserLocale } from '../hooks/useBrowserLocale'
import useIsPhone from '../hooks/useIsPhone'
import useLightTheme from '../hooks/useLightTheme'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useGetProjectRepositories, useListProjects } from '../services'
import { projectHasPullRequests } from '../services/specTaskPRProposalService'
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
import {
  buildNewChatTaskRequest,
  newChatHeading,
  newChatTaskModeStorageKey,
  NewChatTaskMode,
  readNewChatTaskMode,
} from './newChatLogic'
import {
  preferredSpecTaskSandboxResources,
  preferredSpecTaskSandboxRuntime,
  saveSpecTaskSandboxResourcesPreference,
  saveSpecTaskSandboxRuntimePreference,
} from '../utils/specTaskSandboxRuntime'

const T3_FONT_FAMILY = WELCOME_FONT_FAMILY
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
  const isPhone = useIsPhone()
  const lightTheme = useLightTheme()
  const router = useRouter()
  const snackbar = useSnackbar()
  const orgId = account.organizationTools.organization?.id || ''
  const requestedProjectId = router.params.id || ''
  const userId = account.user?.id || ''

  const { data: projects = [], isLoading: projectsLoading } = useListProjects(orgId, {
    enabled: !!userId && !!orgId,
  })
  const selectedProject = projects.find((project) => project.id === requestedProjectId)
  const selectedProjectId = selectedProject?.id || ''

  useEffect(() => {
    if (!projectsLoading && projects.length > 0 && !selectedProject) account.orgNavigate('projects')
  }, [account, projectsLoading, projects.length, selectedProject])

  const [taskCodeAgentConfig, setTaskCodeAgentConfig] = useState<TypesCodeAgentExecutionConfig>()
  // Synced below from the per-project preference or project default. It remains
  // undefined when neither exists so the server can resolve its live default.
  const [taskSandboxResources, setTaskSandboxResources] =
    useState<TypesSandboxResourceOverrides | undefined>()
  const [taskSandboxRuntime, setTaskSandboxRuntime] = useState<TypesSandboxRuntime>(() =>
    preferredSpecTaskSandboxRuntime(requestedProjectId),
  )
  const [taskMode, setTaskMode] = useState<NewChatTaskMode>('build')
  // PR auto-approval only applies to projects with an external repository,
  // and starts from the project's default every time the project changes.
  const { data: projectRepositories = [] } = useGetProjectRepositories(
    selectedProjectId,
    !!selectedProjectId,
  )
  const showAutoApprovePRs = projectHasPullRequests(projectRepositories)
  const projectAutoApprovesPRs = !!selectedProject?.auto_approve_pull_requests
  const [autoApprovePRs, setAutoApprovePRs] = useState(projectAutoApprovesPRs)
  useEffect(() => {
    setAutoApprovePRs(projectAutoApprovesPRs)
  }, [selectedProjectId, projectAutoApprovesPRs])
  const [modeMenuAnchor, setModeMenuAnchor] = useState<HTMLElement | null>(null)
  const [projectMenuAnchor, setProjectMenuAnchor] = useState<HTMLElement | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const [createProjectOpen, setCreateProjectOpen] = useState(false)

  const createTask = useCreateSpecTaskFromPrompt()
  const uploadTaskAttachments = useUploadSpecTaskAttachments()
  const startTask = useStartSpecTaskPlanning()

  const projectCodeAgentConfigKey = JSON.stringify(selectedProject?.code_agent_config ?? null)
  const projectPlanningCodeAgentConfigKey = JSON.stringify(
    selectedProject?.planning_code_agent_config ?? selectedProject?.code_agent_config ?? null,
  )

  useEffect(() => {
    if (!userId || !orgId || !selectedProjectId) return
    const rememberedMode = readNewChatTaskMode(localStorage.getItem(
      newChatTaskModeStorageKey(userId, orgId, selectedProjectId),
    ))
    setTaskMode(rememberedMode)
    setTaskCodeAgentConfig(
      rememberedMode === 'plan'
        ? selectedProject?.planning_code_agent_config || selectedProject?.code_agent_config
        : selectedProject?.code_agent_config,
    )
  }, [
    userId,
    orgId,
    selectedProjectId,
    projectCodeAgentConfigKey,
    projectPlanningCodeAgentConfigKey,
  ])

  // Compute is remembered per project. A project with no explicit user choice
  // starts from its saved defaults; an absent size remains undefined so the
  // server can resolve its live global default when the task starts.
  useEffect(() => {
    setTaskSandboxResources(preferredSpecTaskSandboxResources(
      selectedProjectId,
      selectedProject?.default_sandbox_resource_overrides,
    ))
    setTaskSandboxRuntime(preferredSpecTaskSandboxRuntime(
      selectedProjectId,
      selectedProject?.default_sandbox_runtime,
    ))
  }, [
    selectedProjectId,
    selectedProject?.default_sandbox_resource_overrides?.vcpus,
    selectedProject?.default_sandbox_resource_overrides?.memory_mb,
    selectedProject?.default_sandbox_runtime,
  ])

  const handleTaskSandboxResourcesChange = (resources: TypesSandboxResourceOverrides) => {
    setTaskSandboxResources(resources)
    saveSpecTaskSandboxResourcesPreference(selectedProjectId, resources)
  }

  const handleTaskSandboxRuntimeChange = (runtime: TypesSandboxRuntime) => {
    setTaskSandboxRuntime(runtime)
    saveSpecTaskSandboxRuntimePreference(selectedProjectId, runtime)
  }

  const seedProjectCodeAgentConfig = useSeedProjectCodeAgentConfig(selectedProject)

  const handleTaskCodeAgentConfigChange = (
    next: TypesCodeAgentExecutionConfig,
    source: CodeAgentConfigChangeSource,
  ) => {
    setTaskCodeAgentConfig(next)
    if (taskMode === 'build') seedProjectCodeAgentConfig(next, source)
  }

  const handleTaskModeChange = (mode: NewChatTaskMode) => {
    setTaskMode(mode)
    setTaskCodeAgentConfig(
      mode === 'plan'
        ? selectedProject?.planning_code_agent_config || selectedProject?.code_agent_config
        : selectedProject?.code_agent_config,
    )
    if (userId && orgId && selectedProjectId) {
      localStorage.setItem(
        newChatTaskModeStorageKey(userId, orgId, selectedProjectId),
        mode,
      )
    }
    setModeMenuAnchor(null)
  }

  const openProject = (projectId: string) => {
    setProjectMenuAnchor(null)
    account.orgNavigate('project-new', { id: projectId })
  }

  const handleProjectTask = async (message: string, _interrupt?: boolean, attachments: File[] = []) => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    if (!selectedProjectId) return false
    if (!taskCodeAgentConfig?.model) {
      snackbar.error('Select a coding runtime and model before starting this task')
      return false
    }

    setSubmitting(true)
    let taskId = ''
    try {
      const task = await createTask.mutateAsync(buildNewChatTaskRequest({
        mode: taskMode,
        projectId: selectedProjectId,
        prompt: message,
        codeAgentConfig: taskCodeAgentConfig,
        sandboxResourceOverrides: taskSandboxResources,
        sandboxRuntime: taskSandboxRuntime,
        autoApprovePullRequests: showAutoApprovePRs ? autoApprovePRs : undefined,
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
          onClick={() => handleTaskModeChange('plan')}
        >
          <ListItemIcon><ListTodo size={16} /></ListItemIcon>
          <ListItemText primary="Plan" secondary="Create specifications first" />
        </MenuItem>
        <MenuItem
          selected={taskMode === 'build'}
          onClick={() => handleTaskModeChange('build')}
        >
          <ListItemIcon><Hammer size={16} /></ListItemIcon>
          <ListItemText primary="Build" secondary="Go directly to implementation" />
        </MenuItem>
      </Menu>
    </>
  )

  const projectActions = (
    <Box sx={{ display: 'flex', alignItems: 'center', minWidth: 0, overflow: 'hidden' }}>
      <CodeAgentExecutionControls
        value={taskCodeAgentConfig}
        sandboxResourceOverrides={taskSandboxResources}
        sandboxRuntime={taskSandboxRuntime}
        onChange={handleTaskCodeAgentConfigChange}
        onSandboxResourceOverridesChange={handleTaskSandboxResourcesChange}
        onSandboxRuntimeChange={handleTaskSandboxRuntimeChange}
        disabled={submitting}
        autoSelectDefault
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
      {showAutoApprovePRs && (
        <Tooltip describeChild title="Approve this task's pull requests, and mark it done when the agent says it is finished, without asking. You can change it later in the task's details.">
          <FormControlLabel
            disabled={submitting}
            control={
              <Checkbox
                size="small"
                checked={autoApprovePRs}
                onChange={(event) => setAutoApprovePRs(event.target.checked)}
                sx={{ p: 0.5 }}
              />
            }
            label={<Typography variant="caption" color="text.secondary" noWrap>Auto-approve PRs</Typography>}
            sx={{ ml: 0.5, mr: 0, flexShrink: 0 }}
          />
        </Tooltip>
      )}
    </Box>
  )

  if (projectsLoading) {
    return (
      <Page
        breadcrumbs={[{ title: 'Projects' }]}
        breadcrumbTitle="New task"
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
            account.orgNavigate('project-new', { id: projectId })
          }}
        />
      </>
    )
  }

  if (!selectedProject) return null

  return (
    <Page
      breadcrumbs={[{ title: selectedProject?.name || 'Untitled project' }]}
      breadcrumbTitle="New task"
      breadcrumbShowHome={false}
      disableContentScroll
      px={2}
    >
      <ChatWelcome heading={newChatHeading(selectedProject?.name)} footer={(
        <>
              <Button
                startIcon={<Folder size={14} />}
                endIcon={<ChevronDown size={12} />}
                onClick={(event) => setProjectMenuAnchor(event.currentTarget)}
                sx={{ ...selectorButtonSx, fontSize: isPhone ? '0.8rem' : '0.7rem' }}
              >
                {selectedProject?.name || 'Untitled project'}
              </Button>
              <Menu
                anchorEl={projectMenuAnchor}
                open={!!projectMenuAnchor}
                onClose={() => setProjectMenuAnchor(null)}
              >
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
        </>
      )}>
          {requestedProjectId && projectsLoading ? (
            <Box sx={{ height: 150, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <CircularProgress size={22} />
            </Box>
          ) : (
            <RobustPromptInput
              key={selectedProjectId}
              sessionId={`new-thread:${selectedProjectId}`}
              sendMode="direct"
              autoFocus
              fill={isPhone}
              disabled={submitting || !taskCodeAgentConfig?.model}
              placeholder="Describe what you want to build"
              deferredFileAttachments
              attachmentAccept={TASK_ATTACHMENT_ACCEPT}
              attachmentMaxBytes={SPEC_TASK_ATTACHMENT_MAX_BYTES}
              attachmentMaxCount={SPEC_TASK_ATTACHMENT_MAX_PER_TASK}
              validateAttachment={taskAttachmentValidation}
              leadingActions={projectActions}
              onSend={handleProjectTask}
            />
          )}
      </ChatWelcome>
    </Page>
  )
}

export default Home
