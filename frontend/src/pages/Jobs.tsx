import React, { FC, useState, useMemo, useCallback } from 'react'
import {
  Box,
  Container,
  Typography,
  Button,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Tabs,
  Tab,
  TextField,
  CircularProgress,
  Paper,
  Chip,
  IconButton,
  Tooltip,
  Alert,
  Divider,
} from '@mui/material'
import {
  PlayArrow as PlayIcon,
  Stop as StopIcon,
  Save as SaveIcon,
  Refresh as RefreshIcon,
  ContentCopy as CopyIcon,
} from '@mui/icons-material'
import { useQuery } from '@tanstack/react-query'

import Page from '../components/system/Page'
import ExternalAgentDesktopViewer from '../components/external-agent/ExternalAgentDesktopViewer'
import RobustPromptInput from '../components/common/RobustPromptInput'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useSnackbar from '../hooks/useSnackbar'
import { useStreaming } from '../contexts/streaming'
import { SESSION_TYPE_TEXT } from '../types'
import {
  useListProjects,
  useGetProject,
  useGetProjectRepositories,
  TypesProject,
} from '../services'
import {
  useCreateOrUpdateRepositoryFile,
} from '../services/gitRepositoryService'
import {
  useListSessions,
  useStopExternalAgent,
} from '../services/sessionService'
import type {
  TypesSession,
  TypesPaginatedSessionsList,
} from '../api/api'

const JOB_FILES = [
  { name: 'persona.md', label: 'Persona', placeholder: 'Define the agent\'s role, expertise, and behavior...' },
  { name: 'tasks.md', label: 'Tasks', placeholder: 'List the tasks the agent should perform...' },
  { name: 'notes.md', label: 'Notes', placeholder: 'Additional context, reference data, or instructions...' },
]

interface TabPanelProps {
  children?: React.ReactNode
  index: number
  value: number
}

const TabPanel: FC<TabPanelProps> = ({ children, value, index }) => (
  <Box role="tabpanel" hidden={value !== index} sx={{ py: 2 }}>
    {value === index && children}
  </Box>
)

const Jobs: FC = () => {
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()
  const streaming = useStreaming()

  const [selectedProjectId, setSelectedProjectId] = useState<string>('')
  const [activeTab, setActiveTab] = useState(0)
  const [activeRunSessionId, setActiveRunSessionId] = useState<string>('')
  const [fileContents, setFileContents] = useState<Record<string, string>>({})
  const [fileDirty, setFileDirty] = useState<Record<string, boolean>>({})
  const [saving, setSaving] = useState(false)
  const [starting, setStarting] = useState(false)

  const orgId = account.organizationTools.organization?.id || ''

  // Fetch projects
  const { data: projects = [], isLoading: projectsLoading } = useListProjects(orgId)

  // Fetch selected project details
  const { data: project } = useGetProject(selectedProjectId, !!selectedProjectId)

  // Fetch project repositories to get the default repo ID
  const { data: repos } = useGetProjectRepositories(selectedProjectId, !!selectedProjectId)
  const defaultRepoId = project?.default_repo_id || repos?.[0]?.id || ''

  // Fetch job files from helix-specs branch (files may not exist yet — that's OK)
  const apiClient = api.getApiClient()
  const fetchJobFile = useCallback(async (path: string) => {
    const response = await apiClient.getGitRepositoryFile(defaultRepoId, { path, branch: 'helix-specs' })
    return response.data
  }, [apiClient, defaultRepoId])

  const { data: personaFile, isLoading: personaLoading } = useQuery({
    queryKey: ['job-file', defaultRepoId, 'job/persona.md'],
    queryFn: () => fetchJobFile('job/persona.md'),
    enabled: !!defaultRepoId,
    retry: false,
  })
  const { data: tasksFile, isLoading: tasksLoading } = useQuery({
    queryKey: ['job-file', defaultRepoId, 'job/tasks.md'],
    queryFn: () => fetchJobFile('job/tasks.md'),
    enabled: !!defaultRepoId,
    retry: false,
  })
  const { data: notesFile, isLoading: notesLoading } = useQuery({
    queryKey: ['job-file', defaultRepoId, 'job/notes.md'],
    queryFn: () => fetchJobFile('job/notes.md'),
    enabled: !!defaultRepoId,
    retry: false,
  })

  // Populate file contents when loaded
  React.useEffect(() => {
    if (personaFile && !fileDirty['persona.md']) {
      setFileContents(prev => ({ ...prev, 'persona.md': (personaFile as any)?.content || '' }))
    }
  }, [personaFile])
  React.useEffect(() => {
    if (tasksFile && !fileDirty['tasks.md']) {
      setFileContents(prev => ({ ...prev, 'tasks.md': (tasksFile as any)?.content || '' }))
    }
  }, [tasksFile])
  React.useEffect(() => {
    if (notesFile && !fileDirty['notes.md']) {
      setFileContents(prev => ({ ...prev, 'notes.md': (notesFile as any)?.content || '' }))
    }
  }, [notesFile])

  const updateFileMutation = useCreateOrUpdateRepositoryFile()

  // List job sessions for this project
  const { data: sessionsData } = useListSessions(
    orgId, undefined, undefined, selectedProjectId, 0, 50,
    { enabled: !!selectedProjectId }
  )

  const jobSessions = useMemo(() => {
    const list = (sessionsData?.data as TypesPaginatedSessionsList)?.sessions
      || (sessionsData?.data as any)?.data?.sessions
      || []
    return (list as TypesSession[]).filter(s => s.metadata?.session_role === 'job')
  }, [sessionsData])

  const handleFileChange = useCallback((fileName: string, content: string) => {
    setFileContents(prev => ({ ...prev, [fileName]: content }))
    setFileDirty(prev => ({ ...prev, [fileName]: true }))
  }, [])

  const handleSaveFiles = useCallback(async () => {
    if (!defaultRepoId) {
      snackbar.error('No repository found for this project')
      return
    }
    setSaving(true)
    try {
      for (const file of JOB_FILES) {
        if (fileDirty[file.name]) {
          await updateFileMutation.mutateAsync({
            repositoryId: defaultRepoId,
            request: {
              path: `job/${file.name}`,
              content: fileContents[file.name] || '',
              branch: 'helix-specs',
            },
          })
        }
      }
      setFileDirty({})
      snackbar.success('Job files saved')
    } catch (err: any) {
      snackbar.error(`Failed to save: ${err.message}`)
    } finally {
      setSaving(false)
    }
  }, [defaultRepoId, fileContents, fileDirty, updateFileMutation, snackbar])

  const handleStartJob = useCallback(async () => {
    if (!selectedProjectId) return
    setStarting(true)
    try {
      const session = await streaming.NewInference({
        type: SESSION_TYPE_TEXT,
        message: fileContents['tasks.md'] || 'Run the job tasks as specified in the job files.',
        projectId: selectedProjectId,
        agentType: 'zed_external',
      })
      setActiveRunSessionId(session.id || '')
      setActiveTab(1) // Switch to Runs tab
      snackbar.success('Job started')
    } catch (err: any) {
      snackbar.error(`Failed to start job: ${err.message}`)
    } finally {
      setStarting(false)
    }
  }, [selectedProjectId, fileContents, streaming, snackbar])

  const stopMutation = useStopExternalAgent(activeRunSessionId)

  const handleStopJob = useCallback(async () => {
    if (!activeRunSessionId) return
    try {
      await stopMutation.mutateAsync()
      snackbar.success('Job stopped')
    } catch (err: any) {
      snackbar.error(`Failed to stop job: ${err.message}`)
    }
  }, [activeRunSessionId, stopMutation, snackbar])

  const hasDirtyFiles = Object.values(fileDirty).some(Boolean)
  const filesLoading = personaLoading || tasksLoading || notesLoading

  const curlExample = selectedProjectId ? `# Start a job session
curl -X POST ${window.location.origin}/api/v1/sessions/chat \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "project_id": "${selectedProjectId}",
    "agent_type": "zed_external",
    "session_role": "job",
    "messages": [{"role": "user", "content": {"parts": ["Run the job tasks"]}}]
  }'

# Poll for output
curl ${window.location.origin}/api/v1/sessions/SESSION_ID/output \\
  -H "Authorization: Bearer YOUR_API_KEY"` : ''

  return (
    <Page title="Jobs">
      <Container maxWidth="lg" sx={{ py: 3 }}>
        {/* Header */}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 3 }}>
          <Typography variant="h5">Jobs</Typography>
          <Box sx={{ display: 'flex', gap: 1 }}>
            {hasDirtyFiles && (
              <Button
                variant="outlined"
                startIcon={saving ? <CircularProgress size={16} /> : <SaveIcon />}
                onClick={handleSaveFiles}
                disabled={saving}
              >
                Save Files
              </Button>
            )}
            <Button
              variant="contained"
              startIcon={starting ? <CircularProgress size={16} color="inherit" /> : <PlayIcon />}
              onClick={handleStartJob}
              disabled={!selectedProjectId || starting}
            >
              Run Job
            </Button>
          </Box>
        </Box>

        {/* Project Selector */}
        <FormControl fullWidth sx={{ mb: 3 }}>
          <InputLabel>Project</InputLabel>
          <Select
            value={selectedProjectId}
            label="Project"
            onChange={(e) => {
              setSelectedProjectId(e.target.value)
              setFileContents({})
              setFileDirty({})
              setActiveRunSessionId('')
            }}
          >
            {projectsLoading ? (
              <MenuItem disabled><CircularProgress size={16} sx={{ mr: 1 }} /> Loading...</MenuItem>
            ) : projects.length === 0 ? (
              <MenuItem disabled>No projects — create one first</MenuItem>
            ) : (
              projects.map((p: TypesProject) => (
                <MenuItem key={p.id} value={p.id}>{p.name || p.id}</MenuItem>
              ))
            )}
          </Select>
        </FormControl>

        {selectedProjectId && (
          <>
            {/* Tabs */}
            <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
              <Tabs value={activeTab} onChange={(_, v) => setActiveTab(v)}>
                <Tab label="Files" />
                <Tab label="Runs" />
                <Tab label="Schedule" />
                <Tab label="API" />
              </Tabs>
            </Box>

            {/* Files Tab */}
            <TabPanel value={activeTab} index={0}>
              {filesLoading ? (
                <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
                  <CircularProgress />
                </Box>
              ) : !defaultRepoId ? (
                <Alert severity="info">
                  This project has no git repository. Create one in project settings to use job files.
                </Alert>
              ) : (
                <Box sx={{ display: 'flex', flexDirection: 'column', gap: 3 }}>
                  {JOB_FILES.map(file => (
                    <Paper key={file.name} variant="outlined" sx={{ p: 2 }}>
                      <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                        <Typography variant="subtitle2" sx={{ fontFamily: 'monospace' }}>
                          job/{file.name}
                        </Typography>
                        {fileDirty[file.name] && (
                          <Chip label="modified" size="small" color="warning" sx={{ ml: 1 }} />
                        )}
                      </Box>
                      <TextField
                        fullWidth
                        multiline
                        minRows={6}
                        maxRows={20}
                        value={fileContents[file.name] || ''}
                        onChange={(e) => handleFileChange(file.name, e.target.value)}
                        placeholder={file.placeholder}
                        sx={{
                          '& .MuiInputBase-input': { fontFamily: 'monospace', fontSize: '0.875rem' },
                        }}
                      />
                    </Paper>
                  ))}
                </Box>
              )}
            </TabPanel>

            {/* Runs Tab */}
            <TabPanel value={activeTab} index={1}>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {/* Active run desktop viewer */}
                {activeRunSessionId ? (
                  <Paper variant="outlined" sx={{ overflow: 'hidden' }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', px: 2, py: 1, borderBottom: 1, borderColor: 'divider' }}>
                      <Typography variant="subtitle2">
                        Active Run: {activeRunSessionId}
                      </Typography>
                      <Button
                        size="small"
                        color="error"
                        startIcon={<StopIcon />}
                        onClick={handleStopJob}
                      >
                        Stop
                      </Button>
                    </Box>
                    <ExternalAgentDesktopViewer
                      sessionId={activeRunSessionId}
                      mode="stream"
                      showSessionPanel={true}
                      projectId={selectedProjectId}
                    />
                    <Box sx={{ p: 2, borderTop: 1, borderColor: 'divider' }}>
                      <RobustPromptInput
                        sessionId={activeRunSessionId}
                        projectId={selectedProjectId}
                        apiClient={api.getApiClient()}
                        onSend={async (message: string, interrupt?: boolean) => {
                          await streaming.NewInference({
                            type: SESSION_TYPE_TEXT,
                            message,
                            sessionId: activeRunSessionId,
                            interrupt: interrupt ?? true,
                          })
                        }}
                        placeholder="Send message to agent..."
                      />
                    </Box>
                  </Paper>
                ) : (
                  <Alert severity="info">
                    No active run. Click "Run Job" to start one, or select a previous run below.
                  </Alert>
                )}

                {/* Previous runs */}
                <Divider sx={{ my: 1 }} />
                <Typography variant="subtitle2">Previous Runs</Typography>
                {jobSessions.length === 0 ? (
                  <Typography color="text.secondary" variant="body2">No job runs yet.</Typography>
                ) : (
                  jobSessions.map(session => (
                    <Paper
                      key={session.id}
                      variant="outlined"
                      sx={{
                        p: 2,
                        cursor: 'pointer',
                        '&:hover': { bgcolor: 'action.hover' },
                        ...(session.id === activeRunSessionId ? { borderColor: 'primary.main', borderWidth: 2 } : {}),
                      }}
                      onClick={() => setActiveRunSessionId(session.id || '')}
                    >
                      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                        <Box>
                          <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>
                            {session.id}
                          </Typography>
                          <Typography variant="caption" color="text.secondary">
                            {session.created ? new Date(session.created).toLocaleString() : ''}
                          </Typography>
                        </Box>
                        <Chip
                          label={session.name || 'job'}
                          size="small"
                          variant="outlined"
                        />
                      </Box>
                    </Paper>
                  ))
                )}
              </Box>
            </TabPanel>

            {/* Schedule Tab */}
            <TabPanel value={activeTab} index={2}>
              <Alert severity="info" sx={{ mb: 2 }}>
                Cron scheduling requires an app/agent configured for this project.
                Use the project settings to configure triggers.
              </Alert>
              <Button
                variant="outlined"
                onClick={() => account.orgNavigate('app_settings', { id: selectedProjectId })}
              >
                Open Project Settings
              </Button>
            </TabPanel>

            {/* API Tab */}
            <TabPanel value={activeTab} index={3}>
              <Paper variant="outlined" sx={{ p: 2 }}>
                <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
                  <Typography variant="subtitle2">API Usage</Typography>
                  <Tooltip title="Copy to clipboard">
                    <IconButton
                      size="small"
                      onClick={() => {
                        navigator.clipboard.writeText(curlExample)
                        snackbar.success('Copied to clipboard')
                      }}
                    >
                      <CopyIcon fontSize="small" />
                    </IconButton>
                  </Tooltip>
                </Box>
                <Box
                  component="pre"
                  sx={{
                    p: 2,
                    bgcolor: 'grey.900',
                    color: 'grey.100',
                    borderRadius: 1,
                    overflow: 'auto',
                    fontSize: '0.8rem',
                    fontFamily: 'monospace',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-all',
                  }}
                >
                  {curlExample}
                </Box>
              </Paper>
            </TabPanel>
          </>
        )}
      </Container>
    </Page>
  )
}

export default Jobs
