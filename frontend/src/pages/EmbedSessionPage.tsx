import React, { FC } from 'react'
import { useRoute } from 'react-router5'
import { Box, useTheme } from '@mui/material'
import ExternalAgentDesktopViewer from '../components/external-agent/ExternalAgentDesktopViewer'

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

  // Embed contexts (iframes) have no parent body bg, so force the theme bg here.
  const bg = theme.palette.background.default

  return (
    <Box sx={{ height: '100dvh', overflow: 'hidden', backgroundColor: bg }}>
      <ExternalAgentDesktopViewer
        sessionId={sessionId}
        sandboxId={sessionId}
        mode="stream"
      />
    </Box>
  )
}

export default EmbedSessionPage
