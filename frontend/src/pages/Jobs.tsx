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
  Collapse,
} from '@mui/material'
import {
  PlayArrow as PlayIcon,
  Stop as StopIcon,
  Save as SaveIcon,
  ContentCopy as CopyIcon,
  Code as CodeIcon,
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  Add as AddIcon,
  Close as CloseIcon,
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

const AGENTS_MD_DEFAULT = `# Nightly README review
This is the spec for the nightly agent that keeps \`README.md\` consistent with
the current Kodit code. Only the human edits this file. The agent reads it.
The aim is a narrow, boring task: keep the README accurate. Do not let the
agent turn it into a different document.
## How this file grows
Every morning, review the previous night's diff. If the agent did something
you did not want, add a bullet under **Out of scope** or **Anti-patterns**
that would have stopped it. Over three or four runs the file stops changing.
---
## Repository setup
If a \`repo/\` directory does not already exist, clone the target repository:
\`\`\`bash
gh repo clone helixml/kodit repo
\`\`\`
If \`repo/\` already exists, run \`git -C repo pull\` to get the latest changes.
Stay in this directory (do not cd into repo). Reference repository files as
\`repo/README.md\`, \`repo/cmd/...\`, etc. State files (\`TASKS.md\`, \`LOG.md\`,
etc.) are in the current directory. Git operations on the repo use
\`git -C repo\`. Run verify as \`bash verify.sh\` (it should reference repo/
paths internally).
## State files
Read these in order, once, at the start of the run:
1. \`AGENTS.md\` (this file) -- permanent rules and scope. Only the human
   edits this. You must not violate anything in it.
2. \`QUESTIONS.md\` -- check whether any of your previous questions have been
   answered. Questions still marked \`unanswered\` block any related task.
3. \`TASKS.md\` -- the task list.
4. Last 50 lines of \`LOG.md\` for recent context.
## Execution model
You run once. There is no outer loop. You must keep working until every task
is either \`done\` or \`blocked\`, and no further diagnose findings remain.
The run has two repeating phases: **Diagnose** and **Fix**. Start with
Diagnose. After Diagnose, if there are \`pending\` tasks, move to Fix. After
finishing all pending tasks, run Diagnose again. Repeat until there is
nothing left to do.
Stop when all of the following hold:
- A Diagnose pass produced no new \`pending\` tasks.
- Every task in \`TASKS.md\` is \`done\` or \`blocked\`.
- \`./verify.sh\` exits 0.
### Diagnose
Goal: produce a drift report between README and reality. Make no changes to
\`README.md\`.
1. Run \`./verify.sh > /tmp/verify.log 2>&1 || true\` and read the output.
2. Read \`repo/README.md\` in full.
3. Analyse the codebase to understand current functionality and capabilities.
4. Compare README claims against reality. For each discrepancy, append an
   entry to \`LOG.md\`:
   \`\`\`markdown
   ## YYYY-MM-DD HH:MM diagnose
   - observation: <one sentence>
     evidence: <line number, or short quote>
   \`\`\`
5. For each finding that is actionable and in scope, append a task to
   \`TASKS.md\` with state \`pending\`. One task per finding. Task titles
   under 80 chars, phrased so completion is objectively verifiable.
6. If you find something you cannot classify as in or out of scope, write it
   to \`QUESTIONS.md\` with state \`unanswered\`. Do not create a task for it.
7. Update \`LOG.md\`, \`TASKS.md\`, and \`QUESTIONS.md\` only. Do not touch
   \`README.md\` during Diagnose.
### Fix
Work through every \`pending\` task in \`TASKS.md\`, one at a time, in order.
For each task:
1. Move the task to \`in-progress\`.
2. If the task depends on an \`unanswered\` question, mark it \`blocked\` with
   a pointer to the question and move to the next task.
3. Make the minimum edit to \`repo/README.md\` required by the task. Respect
   the rules in this file.
4. Run \`./verify.sh\`. If non-zero, either fix your edit and re-run, or
   revert and mark the task \`blocked\` with a note explaining why. Do not
   mark a task \`done\` while verify is failing.
5. Mark the task \`done\` in \`TASKS.md\` with a one-line note: what you
   changed and what proved it (which verify check, or a specific
   inspection).
6. Append to \`LOG.md\`:
   \`\`\`markdown
   ## YYYY-MM-DD HH:MM fix
   - task: <task title>
   - change: <one sentence>
   - verified by: <which verify check, or inspection detail>
   \`\`\`
7. Move to the next pending task. When none remain, return to Diagnose.
## Committing and pull request
After you stop, if \`repo/README.md\` has changed:
1. Create a branch in the repo: \`git -C repo checkout -b readme/YYYY-MM-DD\`.
2. Stage and commit only \`README.md\` with a conventional commit git message.
3. Push and open a pull request against the default branch using \`gh\`. The PR title should match the commit message. The body should include information about what tasks were completed in this commit. If this is a follow-on commit, rewrite the PR body and title to match all commits.
If \`README.md\` has not changed, do not create a branch or pull request.
State files (\`TASKS.md\`, \`LOG.md\`, \`QUESTIONS.md\`) are not committed to the
repo. They live in the current directory only.
---
## Scope
### In scope
- Correcting factual errors in \`README.md\` against the current Kodit code.
- Removing documentation for features that no longer exist.
- Adding brief documentation for subcommands, flags, config keys, or MCP
  tool names that exist in code but are not mentioned in the README.
- Fixing broken relative links and broken shell examples.
- Replacing stale version numbers, install instructions, or pasted command
  output.
### Out of scope
- Structural changes to the README: new top-level sections, section
  reorders, moving content between sections.
- Marketing prose of any kind: "Why Kodit", "Who is this for", taglines,
  benefit bullets, problem statements.
- Badges.
- Emoji, including in headings.
- FAQs, unless one already exists and contains a factual error to correct.
- Expanding acronyms already defined earlier in the file.
- Softening or hedging direct statements.
- Long examples or tutorials. Examples stay minimal.
## Style rules
- UK English.
- No em dashes anywhere.
- No contractions in prose. Code and command output are exempt.
- Sentence case in headings.
- No emoji.
- Second person ("you run", "you pass") for user-facing instructions.
- Imperative or third person for reference material.
- Direct statements. "required", not "we recommend". "returns", not
  "will return". "use", not "you may wish to use".
- Fenced code blocks get explicit language tags: \`bash\`, \`go\`, \`yaml\`,
  \`toml\`, \`json\`.
## Anti-patterns
If you catch yourself doing any of these, stop and revert:
- Reorganising the README to be "cleaner" or "more logical".
- Adding a "Why Kodit?", "Overview", or "Features" section.
- Replacing a direct sentence with a bulleted list.
- Expanding a one-line install instruction into a walk-through.
- Adding warnings, notes, or callouts around existing content that was fine.
- Rewriting in a different voice than the surrounding paragraphs.
- Introducing placeholder phrases like "Note that", "It is important to",
  "Please be aware".
## Hard rules
- You edit only \`TASKS.md\`, \`LOG.md\`, \`QUESTIONS.md\`, and \`repo/README.md\`.
- If \`verify.sh\` is failing, prioritise tasks that will make it pass.
- If you are about to make an edit that is not covered by a task in
  \`TASKS.md\`, stop and write the question to \`QUESTIONS.md\`.
## Golden anchors
The following sections of the current README are the style reference. Do not
edit them unless a task explicitly targets them. Match their voice and length
when writing elsewhere:
- [fill in after first pass: e.g. "Installation"]
- [fill in after first pass: e.g. "Configuration"]
`

interface JobFile {
  name: string
  placeholder: string
}

const DEFAULT_JOB_FILES: JobFile[] = [
  { name: 'AGENTS.md', placeholder: 'Define the agent spec — role, scope, execution model, constraints...' },
  { name: 'TASK.md', placeholder: 'List the tasks the agent should perform...' },
  { name: 'LOG.md', placeholder: 'Agent run log — the agent appends entries here...' },
  { name: 'QUESTIONS.md', placeholder: 'Questions for the human to answer between runs...' },
  { name: 'verify.sh', placeholder: 'Verification script — the reward signal. Exit 0 = pass, non-zero = fail...' },
]

const VERIFY_SH_DEFAULT = `#!/usr/bin/env bash
set -euo pipefail

# Verification script — the reward signal.
# Exit 0 means all checks pass. Non-zero means the agent has work to do.
# Customise these checks for your specific job.

echo "Running verification..."

# Example: check that repo/README.md exists
# [ -f repo/README.md ] || { echo "FAIL: repo/README.md missing"; exit 1; }

echo "All checks passed."
`

const FILE_DEFAULTS: Record<string, string> = {
  'AGENTS.md': AGENTS_MD_DEFAULT,
  'verify.sh': VERIFY_SH_DEFAULT,
}

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

const codeBlockSx = {
  p: 1.5,
  bgcolor: 'grey.900',
  color: 'grey.100',
  borderRadius: 1,
  overflow: 'auto',
  fontSize: '0.75rem',
  fontFamily: 'monospace',
  whiteSpace: 'pre-wrap' as const,
  wordBreak: 'break-all' as const,
  maxHeight: 300,
}

interface ApiCallBlockProps {
  label: string
  curl: string
}

const ApiCallBlock: FC<ApiCallBlockProps> = ({ label, curl }) => {
  const [open, setOpen] = useState(false)
  const snackbar = useSnackbar()

  return (
    <Paper variant="outlined" sx={{ mt: 2, bgcolor: 'transparent' }}>
      <Box
        sx={{ display: 'flex', alignItems: 'center', px: 1.5, py: 0.5, cursor: 'pointer' }}
        onClick={() => setOpen(!open)}
      >
        <CodeIcon sx={{ fontSize: 16, mr: 1, color: 'text.secondary' }} />
        <Typography variant="caption" color="text.secondary" sx={{ flex: 1 }}>
          {label}
        </Typography>
        <Tooltip title="Copy">
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation()
              navigator.clipboard.writeText(curl)
              snackbar.success('Copied')
            }}
          >
            <CopyIcon sx={{ fontSize: 14 }} />
          </IconButton>
        </Tooltip>
        {open ? <ExpandLessIcon sx={{ fontSize: 18 }} /> : <ExpandMoreIcon sx={{ fontSize: 18 }} />}
      </Box>
      <Collapse in={open}>
        <Box component="pre" sx={codeBlockSx}>
          {curl}
        </Box>
      </Collapse>
    </Paper>
  )
}

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
  const [jobFiles, setJobFiles] = useState<JobFile[]>(DEFAULT_JOB_FILES)
  const [newFileName, setNewFileName] = useState('')

  const orgId = account.organizationTools.organization?.id || ''
  const origin = window.location.origin

  // Fetch projects
  const { data: projects = [], isLoading: projectsLoading } = useListProjects(orgId)

  // Fetch selected project details
  const { data: project } = useGetProject(selectedProjectId, !!selectedProjectId)

  // Fetch project repositories to get the default repo ID
  const { data: repos } = useGetProjectRepositories(selectedProjectId, !!selectedProjectId)
  const defaultRepoId = project?.default_repo_id || repos?.[0]?.id || ''

  // Fetch all job files from helix-specs branch in one query
  const apiClient = api.getApiClient()
  const fileNames = useMemo(() => jobFiles.map(f => f.name), [jobFiles])

  const { data: filesFromBranch, isLoading: filesLoading } = useQuery({
    queryKey: ['job-files', defaultRepoId, fileNames],
    queryFn: async () => {
      const results: Record<string, string> = {}
      for (const name of fileNames) {
        try {
          const response = await apiClient.getGitRepositoryFile(defaultRepoId, { path: `job/${name}`, branch: 'helix-specs' })
          results[name] = (response.data as any)?.content || ''
        } catch {
          // File doesn't exist yet
        }
      }
      return results
    },
    enabled: !!defaultRepoId,
    retry: false,
  })

  // Populate file contents when loaded from branch
  React.useEffect(() => {
    if (!filesFromBranch) return
    setFileContents(prev => {
      const next = { ...prev }
      for (const [name, content] of Object.entries(filesFromBranch)) {
        if (!fileDirty[name]) {
          next[name] = content
        }
      }
      // Pre-fill defaults for files that don't exist on branch yet
      for (const file of jobFiles) {
        if (!filesFromBranch[file.name] && !fileDirty[file.name] && !prev[file.name] && FILE_DEFAULTS[file.name]) {
          next[file.name] = FILE_DEFAULTS[file.name]
        }
      }
      return next
    })
  }, [filesFromBranch])

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
      for (const file of jobFiles) {
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
  }, [defaultRepoId, fileContents, fileDirty, jobFiles, updateFileMutation, snackbar])

  const handleStartJob = useCallback(async () => {
    if (!selectedProjectId) return
    setStarting(true)
    try {
      const session = await streaming.NewInference({
        type: SESSION_TYPE_TEXT,
        message: fileContents['TASK.md'] || 'Run the job tasks as specified in the job files.',
        projectId: selectedProjectId,
        agentType: 'zed_external',
        sessionRole: 'job',
      })
      setActiveRunSessionId(session.id || '')
      setActiveTab(1)
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

  const handleAddFile = useCallback(() => {
    const name = newFileName.trim()
    if (!name) return
    if (jobFiles.some(f => f.name === name)) {
      snackbar.error('File already exists')
      return
    }
    setJobFiles(prev => [...prev, { name, placeholder: '' }])
    setNewFileName('')
  }, [newFileName, jobFiles, snackbar])

  const handleRemoveFile = useCallback((name: string) => {
    setJobFiles(prev => prev.filter(f => f.name !== name))
    setFileContents(prev => {
      const next = { ...prev }
      delete next[name]
      return next
    })
    setFileDirty(prev => {
      const next = { ...prev }
      delete next[name]
      return next
    })
  }, [])

  const hasDirtyFiles = Object.values(fileDirty).some(Boolean)

  // --- API call curl strings ---

  const buildSaveFileCurl = useCallback((fileName: string) => {
    const content = (fileContents[fileName] || '').replace(/'/g, "'\\''").slice(0, 200)
    const truncated = (fileContents[fileName] || '').length > 200 ? '...' : ''
    return `curl -X PUT ${origin}/api/v1/git/repositories/${defaultRepoId}/contents \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "path": "job/${fileName}",
    "content": "${content}${truncated}",
    "branch": "helix-specs"
  }'`
  }, [origin, defaultRepoId, fileContents])

  const saveFilesCurl = useMemo(() => {
    const dirty = jobFiles.filter(f => fileDirty[f.name])
    if (dirty.length === 0) {
      return jobFiles.map(f => buildSaveFileCurl(f.name)).join('\n\n')
    }
    return dirty.map(f => buildSaveFileCurl(f.name)).join('\n\n')
  }, [fileDirty, jobFiles, buildSaveFileCurl])

  const runJobCurl = useMemo(() => {
    const prompt = (fileContents['TASK.md'] || 'Run the job tasks as specified in the job files.')
      .replace(/'/g, "'\\''")
      .replace(/\n/g, '\\n')
      .slice(0, 200)
    const truncated = (fileContents['TASK.md'] || '').length > 200 ? '...' : ''
    return `curl -X POST ${origin}/api/v1/sessions/chat \\
  -H "Authorization: Bearer YOUR_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "project_id": "${selectedProjectId}",
    "agent_type": "zed_external",
    "session_role": "job",
    "messages": [{"role": "user", "content": {"parts": ["${prompt}${truncated}"]}}]
  }'`
  }, [origin, selectedProjectId, fileContents])

  const stopJobCurl = `curl -X DELETE ${origin}/api/v1/sessions/${activeRunSessionId}/stop-external-agent \\
  -H "Authorization: Bearer YOUR_API_KEY"`

  const pollOutputCurl = `curl ${origin}/api/v1/sessions/${activeRunSessionId}/output \\
  -H "Authorization: Bearer YOUR_API_KEY"`

  const listSessionsCurl = `curl "${origin}/api/v1/sessions?project_id=${selectedProjectId}&session_role=job" \\
  -H "Authorization: Bearer YOUR_API_KEY"`

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
              setJobFiles(DEFAULT_JOB_FILES)
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
          <Typography variant="caption" color="text.secondary">
            1:1 mapping between jobs and projects.{' '}
            <span
              style={{ color: '#90caf9', cursor: 'pointer', textDecoration: 'underline' }}
              onClick={() => account.orgNavigate('projects')}
            >
              Go to Projects page
            </span>
            {' '}to create new projects, then come back here.
          </Typography>
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
                  {jobFiles.map(file => (
                    <Paper key={file.name} variant="outlined" sx={{ p: 2 }}>
                      <Box sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
                        <Typography variant="subtitle2" sx={{ fontFamily: 'monospace', flex: 1 }}>
                          job/{file.name}
                        </Typography>
                        {fileDirty[file.name] && (
                          <Chip label="modified" size="small" color="warning" sx={{ ml: 1 }} />
                        )}
                        <Tooltip title="Remove file">
                          <IconButton
                            size="small"
                            onClick={() => handleRemoveFile(file.name)}
                            sx={{ ml: 0.5 }}
                          >
                            <CloseIcon sx={{ fontSize: 16 }} />
                          </IconButton>
                        </Tooltip>
                      </Box>
                      <TextField
                        fullWidth
                        multiline
                        minRows={file.name === 'AGENTS.md' ? 10 : 4}
                        maxRows={30}
                        value={fileContents[file.name] || ''}
                        onChange={(e) => handleFileChange(file.name, e.target.value)}
                        placeholder={file.placeholder}
                        sx={{
                          '& .MuiInputBase-input': { fontFamily: 'monospace', fontSize: '0.875rem' },
                        }}
                      />
                    </Paper>
                  ))}

                  {/* Add file */}
                  <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
                    <TextField
                      size="small"
                      placeholder="filename.md"
                      value={newFileName}
                      onChange={(e) => setNewFileName(e.target.value)}
                      onKeyDown={(e) => { if (e.key === 'Enter') handleAddFile() }}
                      sx={{ flex: 1, maxWidth: 300, '& .MuiInputBase-input': { fontFamily: 'monospace' } }}
                    />
                    <Button
                      size="small"
                      startIcon={<AddIcon />}
                      onClick={handleAddFile}
                      disabled={!newFileName.trim()}
                    >
                      Add File
                    </Button>
                  </Box>

                  {/* API call for Save Files */}
                  <ApiCallBlock
                    label="Save Files → PUT /api/v1/git/repositories/:id/contents"
                    curl={saveFilesCurl}
                  />
                </Box>
              )}
            </TabPanel>

            {/* Runs Tab */}
            <TabPanel value={activeTab} index={1}>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                {/* Run Job API call */}
                <ApiCallBlock
                  label="Run Job → POST /api/v1/sessions/chat"
                  curl={runJobCurl}
                />

                {/* Active run desktop viewer */}
                {activeRunSessionId ? (
                  <>
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

                    {/* Stop + Poll API calls */}
                    <ApiCallBlock
                      label="Stop Job → DELETE /api/v1/sessions/:id/stop-external-agent"
                      curl={stopJobCurl}
                    />
                    <ApiCallBlock
                      label="Poll Output → GET /api/v1/sessions/:id/output"
                      curl={pollOutputCurl}
                    />
                  </>
                ) : (
                  <Alert severity="info">
                    No active run. Click &quot;Run Job&quot; to start one, or select a previous run below.
                  </Alert>
                )}

                {/* List sessions API call */}
                <ApiCallBlock
                  label="List Job Sessions → GET /api/v1/sessions?session_role=job"
                  curl={listSessionsCurl}
                />

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
                onClick={() => account.orgNavigate('project-settings', { id: selectedProjectId })}
              >
                Open Project Settings
              </Button>
            </TabPanel>

            {/* API Tab */}
            <TabPanel value={activeTab} index={3}>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                Full API reference. Each tab above also shows the relevant API calls inline.
              </Typography>
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <ApiCallBlock
                  label="Save a job file → PUT /api/v1/git/repositories/:id/contents"
                  curl={buildSaveFileCurl('TASK.md')}
                />
                <ApiCallBlock
                  label="Start a job → POST /api/v1/sessions/chat"
                  curl={runJobCurl}
                />
                <ApiCallBlock
                  label="Stop a job → DELETE /api/v1/sessions/:id/stop-external-agent"
                  curl={stopJobCurl}
                />
                <ApiCallBlock
                  label="Poll output → GET /api/v1/sessions/:id/output"
                  curl={pollOutputCurl}
                />
                <ApiCallBlock
                  label="List job sessions → GET /api/v1/sessions?session_role=job"
                  curl={listSessionsCurl}
                />
              </Box>
            </TabPanel>
          </>
        )}
      </Container>
    </Page>
  )
}

export default Jobs
