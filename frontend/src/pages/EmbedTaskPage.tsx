import React, { FC } from 'react'
import { useRoute } from 'react-router5'
import { Box, CircularProgress, useTheme } from '@mui/material'
import SpecTaskDetailContent from '../components/tasks/SpecTaskDetailContent'
import AgentChat from '../components/session/AgentChat'
import { useSpecTask } from '../services/specTaskService'

const EmbedTaskPage: FC = () => {
  const { route } = useRoute()
  const theme = useTheme()
  const taskId = route.params.taskId as string
  const { data: task, isLoading } = useSpecTask(taskId, { enabled: !!taskId })

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
  const welcomeHeading = (route.params.heading as string) || undefined
  const welcomeSubheading = (route.params.subheading as string) || undefined

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
      <SpecTaskDetailContent taskId={taskId} enableForegroundPRRefresh={false} />
    </Box>
  )
}

export default EmbedTaskPage
