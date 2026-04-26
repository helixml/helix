import React, { FC } from 'react'
import { useRoute } from 'react-router5'
import { Box, CircularProgress, useTheme } from '@mui/material'
import SpecTaskDetailContent from '../components/tasks/SpecTaskDetailContent'
import { useSpecTask } from '../services/specTaskService'

const EmbedTaskPage: FC = () => {
  const { route } = useRoute()
  const theme = useTheme()
  const taskId = route.params.taskId as string
  const { isLoading } = useSpecTask(taskId, { enabled: !!taskId })

  // Embed contexts (iframes) don't have a parent body bg, so the white iframe
  // default leaks through anywhere SpecTaskDetailContent doesn't paint. Force
  // the theme bg here.
  const bg = theme.palette.background.default

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh', backgroundColor: bg }}>
        <CircularProgress />
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
      <SpecTaskDetailContent taskId={taskId} />
    </Box>
  )
}

export default EmbedTaskPage
