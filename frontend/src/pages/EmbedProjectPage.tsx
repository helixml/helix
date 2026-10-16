import React, { FC, useState } from 'react'
import { useRoute } from 'react-router5'
import { Box, Button, CircularProgress, Typography, useTheme } from '@mui/material'
import { Panel, Group as PanelGroup, Separator as PanelResizeHandle } from 'react-resizable-panels'
import { ArrowLeft, Bot, KanbanSquare } from 'lucide-react'
import AgentChat from '../components/session/AgentChat'
import ExternalAgentDesktopViewer from '../components/external-agent/ExternalAgentDesktopViewer'
import SpecTaskKanbanBoard from '../components/tasks/SpecTaskKanbanBoard'
import SpecTaskDetailContent from '../components/tasks/SpecTaskDetailContent'
import { useGetProject, useGetProjectExploratorySession } from '../services'
import { initialQueryParams } from '../router'

// EmbedProjectPage is the token-authenticated embed for a PROJECT and the agent that
// lives in it — built for an org bot, which owns exactly one project and one durable
// exploratory session. HelixOS embeds it in an iframe (see /embed/project/:projectId).
//
// Two views, because an org bot is both a colleague you talk to and the manager of its
// own coding work:
//
//   - Bot:   the project's exploratory session — the full operator chat beside the live
//            desktop, the same split the task embed gives a spec task. EmbedSessionPage
//            cannot serve this: it shows the desktop OR a customer-tuned minimal chat.
//   - Board: the project's Kanban board. Spec tasks the bot creates (create_spectask)
//            land here; clicking one opens the same task view /embed/task renders, with
//            a way back.
//
// Navigation stays inside this page (state, seeded from ?view= and ?task=) rather than
// moving between routes, so the iframe never leaves /embed/*. Auth is the usual embed
// flow: ?access_token= is lifted into the Authorization header by useApi, which also
// patches fetch, WebSocket and EventSource, so every reused component authenticates.
type View = 'bot' | 'board'

const EmbedProjectPage: FC = () => {
  const { route } = useRoute()
  const theme = useTheme()
  const projectId = route.params.projectId as string
  const initialView: View = (initialQueryParams.view || route.params.view) === 'board' ? 'board' : 'bot'
  const [view, setView] = useState<View>(initialView)
  const [taskId, setTaskId] = useState<string | null>(
    (initialQueryParams.task as string) || (route.params.task as string) || null,
  )

  const { data: project, isLoading: projectLoading } = useGetProject(projectId, !!projectId)
  // Polls (5.3s): a bot's session is created asynchronously after it is hired or run.
  const { data: session } = useGetProjectExploratorySession(projectId, !!projectId)
  const sessionId = session?.id

  // Embed contexts have no parent body background; paint the theme's.
  const bg = theme.palette.background.default

  if (projectLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100dvh', backgroundColor: bg }}>
        <CircularProgress />
      </Box>
    )
  }

  const tab = (value: View, label: string, Icon: typeof Bot) => (
    <Button
      size="small"
      onClick={() => {
        setView(value)
        setTaskId(null)
      }}
      startIcon={<Icon size={16} />}
      color={view === value ? 'primary' : 'inherit'}
      variant={view === value ? 'contained' : 'text'}
      disableElevation
      sx={{ textTransform: 'none' }}
    >
      {label}
    </Button>
  )

  return (
    <Box sx={{ height: '100dvh', overflow: 'hidden', backgroundColor: bg, display: 'flex', flexDirection: 'column' }}>
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          px: 1.5,
          minHeight: 44,
          flexShrink: 0,
          borderBottom: 1,
          borderColor: 'divider',
        }}
      >
        {tab('bot', 'Bot', Bot)}
        {tab('board', 'Board', KanbanSquare)}
        {view === 'board' && taskId && (
          <Button
            size="small"
            onClick={() => setTaskId(null)}
            startIcon={<ArrowLeft size={16} />}
            color="inherit"
            sx={{ textTransform: 'none', ml: 1 }}
          >
            Back to board
          </Button>
        )}
        <Typography variant="body2" color="text.secondary" sx={{ ml: 'auto' }} noWrap>
          {project?.name}
        </Typography>
      </Box>

      <Box sx={{ flex: 1, minHeight: 0, display: 'flex', flexDirection: 'column' }}>
        {view === 'bot' &&
          (sessionId ? (
            <PanelGroup id={`embed-project-${projectId}`} orientation="horizontal" style={{ height: '100%', flex: 1 }}>
              <Panel id="embed-project-chat" defaultSize="40%" minSize="20%" style={{ overflow: 'hidden' }}>
                <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
                  <AgentChat sessionId={sessionId} projectId={projectId} placeholder="Send message to agent..." />
                </Box>
              </Panel>
              <PanelResizeHandle style={{ width: 6, cursor: 'col-resize', background: theme.palette.divider }} />
              <Panel id="embed-project-desktop" defaultSize="60%" minSize="20%" style={{ overflow: 'hidden' }}>
                <Box sx={{ height: '100%', display: 'flex', flexDirection: 'column' }}>
                  <ExternalAgentDesktopViewer sessionId={sessionId} sandboxId={sessionId} mode="stream" />
                </Box>
              </Panel>
            </PanelGroup>
          ) : (
            <Box sx={{ display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center', flex: 1, gap: 1 }}>
              <CircularProgress size={24} />
              <Typography variant="body2" color="text.secondary">
                Waiting for the agent&apos;s session — it starts the first time the bot runs.
              </Typography>
            </Box>
          ))}

        {view === 'board' && !taskId && (
          <Box sx={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column', px: 1.5, pt: 1 }}>
            <SpecTaskKanbanBoard projectId={projectId} onTaskClick={(task) => setTaskId(task.id)} />
          </Box>
        )}

        {view === 'board' && taskId && (
          <Box sx={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
            <SpecTaskDetailContent taskId={taskId} enableForegroundPRRefresh={false} />
          </Box>
        )}
      </Box>
    </Box>
  )
}

export default EmbedProjectPage
