import React, { FC } from 'react'
import { useRoute } from 'react-router5'
import { Box, useTheme } from '@mui/material'
import ExternalAgentDesktopViewer from '../components/external-agent/ExternalAgentDesktopViewer'
import AgentChat from '../components/session/AgentChat'
import { initialQueryParams } from '../router'

// EmbedSessionPage is the fullscreen, token-authenticated embed for a live desktop
// SESSION (agent_type=zed_external), mirroring EmbedTaskPage but keyed on a session id
// rather than a spec task. It is how HelixOS embeds a running bot / hypothesis /
// candidate-search desktop in an iframe (see /embed/session/:sessionId).
//
// Auth: pages under /embed/* carry the Helix API key as ?access_token=... which
// useApi lifts into the Authorization header (and strips from the URL). The key owns
// the session it launched, so it's authorized to view the stream.
const EmbedSessionPage: FC = () => {
  const { route } = useRoute()
  const theme = useTheme()
  const sessionId = route.params.sessionId as string

  // Minimal mode: a conversation and nothing else.
  //
  // Mirrors EmbedTaskPage's ?minimal=1, and exists because an ORG BOT INSTANCE
  // has no spec task. An instance is a bare session (session_role =
  // org_bot_instance) that a product creates per end user, so the task-keyed
  // embed cannot address it — and the default here, a live desktop video
  // stream, is the wrong surface twice over for a customer-facing chat: it
  // shows them a desktop, and a headless instance has no desktop to show.
  //
  // AgentChat is already session-keyed (specTaskId and projectId are optional;
  // they only drive the spec-task status refresh), so this needs no new chat
  // plumbing — just the route that reaches it.
  const minimal = route.params.minimal === '1' || route.params.minimal === 'true'
  // Read the welcome copy from the RAW query, not from route.params: router5
  // form-encodes a space as "+" when it re-serialises the URL on start, so a
  // host that correctly sent %20 had its heading rendered with literal plus
  // signs. See initialQueryParams in router.tsx, and the matching note in
  // EmbedTaskPage.
  const welcomeHeading = initialQueryParams.heading || (route.params.heading as string) || undefined
  const welcomeSubheading = initialQueryParams.subheading || (route.params.subheading as string) || undefined

  // Embed contexts (iframes) have no parent body bg, so force the theme bg here.
  const bg = theme.palette.background.default

  if (minimal) {
    return (
      // display:flex is load-bearing: AgentChat sizes itself with `flex: 1` and
      // has no height of its own, so in a plain block wrapper it collapses.
      <Box sx={{ height: '100dvh', overflow: 'hidden', backgroundColor: bg, display: 'flex', flexDirection: 'column' }}>
        <AgentChat
          sessionId={sessionId}
          minimal
          {...(welcomeHeading ? { welcomeHeading } : {})}
          welcomeSubheading={welcomeSubheading}
        />
      </Box>
    )
  }

  return (
    // display:flex is load-bearing, not cosmetic. ExternalAgentDesktopViewer's root
    // sizes itself with `flex: 1` (it has no height of its own), so in a plain block
    // wrapper that flex is inert and the viewer collapses to its content height —
    // the desktop renders small and top-aligned with dead space beneath it.
    <Box sx={{ height: '100dvh', overflow: 'hidden', backgroundColor: bg, display: 'flex', flexDirection: 'column' }}>
      <ExternalAgentDesktopViewer
        sessionId={sessionId}
        sandboxId={sessionId}
        mode="stream"
      />
    </Box>
  )
}

export default EmbedSessionPage
