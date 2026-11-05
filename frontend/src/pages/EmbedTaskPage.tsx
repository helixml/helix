import React, { FC } from 'react'
import { useRoute } from 'react-router5'
import { Box, Button, CircularProgress, Typography, useTheme } from '@mui/material'
import SpecTaskDetailContent from '../components/tasks/SpecTaskDetailContent'
import AgentChat from '../components/session/AgentChat'
import { useSpecTask } from '../services/specTaskService'
import { initialQueryParams } from '../router'

const EmbedTaskPage: FC = () => {
  const { route } = useRoute()
  const theme = useTheme()
  const taskId = route.params.taskId as string
  const { data: task, isLoading, error } = useSpecTask(taskId, { enabled: !!taskId })

  // Minimal mode: a conversation and nothing else.
  //
  // The default embed is the full task workspace — view switcher, desktop
  // controls, model and sandbox pickers, the task's opening prompt rendered as
  // a user message. That is right for a developer watching their own task and
  // wrong for a product that has embedded an agent for its customers: a
  // candidate on the Find AI job board was shown the entire system prompt, the
  // tool list and the sandbox's repository paths before they had typed a word.
  //
  // Opt-in via ?minimal=1 so existing embeds are untouched. The heading and
  // subheading let the host name its own agent; they only show on the welcome
  // screen, before anyone has said anything.
  const minimal = route.params.minimal === '1' || route.params.minimal === 'true'
  // ?controls=0: the full workspace (chat, desktop, views) without the task's
  // own controls — for a host that embeds a customer's agent for that customer.
  const hideControls = route.params.controls === '0' || route.params.controls === 'false'
  // Read the welcome copy from the RAW query, not from route.params: router5
  // form-encodes a space as "+" when it re-serialises the URL on start, so a
  // host that correctly sent %20 had its heading rendered with literal plus
  // signs. See initialQueryParams in router.tsx. Fall back to route.params so
  // an in-app navigation that sets them still works.
  const welcomeHeading = initialQueryParams.heading || (route.params.heading as string) || undefined
  const welcomeSubheading = initialQueryParams.subheading || (route.params.subheading as string) || undefined

  // Embed contexts (iframes) don't have a parent body bg, so the white iframe
  // default leaks through anywhere the content doesn't paint. Force the theme
  // bg here.
  const bg = theme.palette.background.default

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', backgroundColor: bg }}>
        <CircularProgress />
      </Box>
    )
  }

  // The task could not be loaded at all. Say so instead of spinning: the
  // workspace below waits for a task that will never arrive, which is how a
  // Helix Recruiter customer was left watching an endless spinner (a
  // permissions fault, Helix #3455). Any future fault of that kind now shows
  // the reason, and the host's reload or a new link fixes the rest.
  if (!task && error) {
    return <EmbedLoadError error={error} background={bg} />
  }

  if (minimal) {
    const sessionId = task?.planning_session_id
    if (!sessionId) {
      // The sandbox is still coming up. A spinner is the honest answer — the
      // host shows its own "starting your assistant" copy around this iframe.
      return (
        <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100dvh', backgroundColor: bg }}>
          <CircularProgress />
        </Box>
      )
    }
    return (
      // display:flex is load-bearing: AgentChat sizes itself with `flex: 1` and
      // has no height of its own, so in a plain block wrapper it collapses.
      <Box sx={{ height: '100dvh', overflow: 'hidden', backgroundColor: bg, display: 'flex', flexDirection: 'column' }}>
        <AgentChat
          sessionId={sessionId}
          specTaskId={taskId}
          projectId={task?.project_id}
          minimal
          {...(welcomeHeading ? { welcomeHeading } : {})}
          welcomeSubheading={welcomeSubheading}
        />
      </Box>
    )
  }

  return (
    // 100dvh (instead of 100vh) lets mobile Safari handle its dynamic
    // viewport correctly. When this page is itself put in fullscreen
    // (e.g. via a Gatewaze iframe embed and the user clicks the
    // fullscreen button on the desktop viewer — see
    // DesktopStreamViewer.tsx toggleFullscreen), the iframe's window
    // is resized to the browser viewport and 100dvh expands with it.
    <Box sx={{ height: '100dvh', overflow: 'hidden', backgroundColor: bg }}>
      <SpecTaskDetailContent taskId={taskId} enableForegroundPRRefresh={false} hideControls={hideControls} />
    </Box>
  )
}

// EmbedLoadError explains, inside the iframe, why the embedded task did not load.
const EmbedLoadError: FC<{ error: unknown; background: string }> = ({ error, background }) => {
  const response = (error as { response?: { status?: number; data?: unknown } })?.response
  const status = response?.status
  const reason = typeof response?.data === 'string' ? response.data.trim() : ''
  let message = "This agent view couldn't load."
  if (status === 401 || status === 403) {
    message = "This agent view isn't allowed to open its task. The link may have expired: reload the page."
  } else if (status === 404) {
    message = 'This agent run no longer exists.'
  }
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        gap: 2,
        justifyContent: 'center',
        alignItems: 'center',
        height: '100dvh',
        p: 3,
        textAlign: 'center',
        backgroundColor: background,
      }}
    >
      <Typography variant="body1">{message}</Typography>
      {(reason || status) && (
        <Typography variant="body2" color="text.secondary">
          {reason || `Error ${status}`}
        </Typography>
      )}
      <Button variant="outlined" size="small" onClick={() => window.location.reload()}>
        Reload
      </Button>
    </Box>
  )
}

export default EmbedTaskPage
