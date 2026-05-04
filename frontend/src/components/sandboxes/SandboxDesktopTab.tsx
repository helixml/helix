import { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

import ExternalAgentDesktopViewer from '../external-agent/ExternalAgentDesktopViewer'
import { TypesSandbox } from '../../api/api'

interface SandboxDesktopTabProps {
  sandbox: TypesSandbox
}

// Map sandbox.status to the state strings ExternalAgentDesktopViewer expects
// (running / starting / absent / loading). The viewer was originally written
// for Helix sessions; sandbox statuses are similar but differ in name.
const mapSandboxStatusToViewerState = (status?: string): string => {
  switch (status) {
    case 'running':
      return 'running'
    case 'pending':
    case 'starting':
      return 'starting'
    case 'stopping':
    case 'stopped':
    case 'failed':
      return 'absent'
    default:
      return 'loading'
  }
}

// SandboxDesktopTab renders the live, interactive desktop stream for a
// running ubuntu-desktop sandbox. The backend registers the desktop-bridge
// inside the sandbox under sessionId == sandbox.id, so we can hand the
// sandbox id straight to the same viewer the spec task page uses.
const SandboxDesktopTab: FC<SandboxDesktopTabProps> = ({ sandbox }) => {
  if (!sandbox.id) {
    return (
      <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
        Sandbox id missing.
      </Typography>
    )
  }

  return (
    <Box
      sx={{
        // Same chrome as the rest of the sandbox tabs — bordered, no Paper fill.
        border: '1px solid',
        borderColor: 'divider',
        borderRadius: 1,
        overflow: 'hidden',
        // Give the stream a fixed-ish viewport so it sits comfortably mid-page.
        height: 'min(70vh, 720px)',
        display: 'flex',
        flexDirection: 'column',
      }}
    >
      <ExternalAgentDesktopViewer
        sessionId={sandbox.id}
        sandboxId={sandbox.id}
        mode="stream"
        displayWidth={sandbox.display_width || undefined}
        displayHeight={sandbox.display_height || undefined}
        displayFps={sandbox.display_fps || undefined}
        initialSandboxState={mapSandboxStatusToViewerState(sandbox.status)}
        initialSandboxStatusMessage={sandbox.status_message}
        startupErrorMessage={sandbox.status === 'failed' ? sandbox.status_message : undefined}
        sandboxMode
      />
    </Box>
  )
}

export default SandboxDesktopTab
