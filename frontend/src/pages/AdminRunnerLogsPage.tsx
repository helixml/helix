import { FC } from 'react'
import { useRoute } from 'react-router5'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import RunnerLogs from '../components/admin/RunnerLogs'

// AdminRunnerLogsPage renders the live Runner Logs viewer full-screen as a
// standalone tab. The component reads `runner_id` from the route params and
// hands off to <RunnerLogs/> which owns the WS lifecycle, snapshot,
// pause/clear, and connection-status UI.
//
// Route: /admin/runner-logs/:runner_id (top-level, no org scope — Runners
// are global infrastructure, see the handler comment in
// api/pkg/server/admin_runner_logs.go).
const AdminRunnerLogsPage: FC = () => {
  const { route } = useRoute()
  const runnerId = (route?.params?.runner_id as string) || ''

  if (!runnerId) {
    return (
      <Box sx={{ p: 4 }}>
        <Typography variant="h6">Missing runner_id</Typography>
        <Typography variant="body2" color="text.secondary">
          This page expects a runner id in the URL path.
        </Typography>
      </Box>
    )
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        position: 'fixed',
        inset: 0,
        p: 2,
        backgroundColor: 'background.default',
        // Prevent the page from being scrolled by the terminal's content;
        // the terminal does its own internal scroll.
        overflow: 'hidden',
      }}
    >
      <Box sx={{ mb: 1, flexShrink: 0 }}>
        <Typography variant="subtitle1" sx={{ fontFamily: 'monospace' }}>
          Runner Logs · {runnerId}
        </Typography>
      </Box>
      <Box sx={{ flexGrow: 1, minHeight: 0, minWidth: 0 }}>
        <RunnerLogs runnerId={runnerId} height="100%" showOpenInNewTab={false} />
      </Box>
    </Box>
  )
}

export default AdminRunnerLogsPage
