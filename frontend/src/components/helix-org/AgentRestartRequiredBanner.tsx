// Shown when a bot's sandbox is running but still holds the tool list and
// instructions from before the operator's last save. The MCP tool list is
// fetched once at agent startup and never refreshed, so the only way to
// apply those changes is a restart.
//
// The restart mints a brand-new session and thread on purpose: a preserved
// transcript still contains successful tool calls for tools that no longer
// exist, and the model reads its own history as proof of capability. It is
// never automatic — an in-flight turn is the one thing a restart destroys,
// so the button is gated while the agent is working and the cost is spelled
// out in a confirm dialog.

import { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { alpha, useTheme } from '@mui/material/styles'
import { RotateCcw } from 'lucide-react'

import { APP_FONT_FAMILY } from '../../styles/typography'

export interface AgentRestartRequiredBannerProps {
  visible: boolean
  working?: boolean
  busy?: boolean
  // Pin the banner to the top of its scrolling ancestor. Only for pages
  // where the banner is mounted inside scrolling content (so it would
  // otherwise scroll out of view); leave off wherever the banner already
  // sits in a non-scrolling header row.
  sticky?: boolean
  onRestart: () => void
}

const AgentRestartRequiredBanner: FC<AgentRestartRequiredBannerProps> = ({
  visible,
  working = false,
  busy = false,
  sticky = false,
  onRestart,
}) => {
  const theme = useTheme()
  const [dismissed, setDismissed] = useState(false)
  const [confirming, setConfirming] = useState(false)

  // Re-arm on a genuine new restart-required cycle. "Not now" is meant to
  // quiet the banner for the current staleness, not to switch it off for the
  // life of the page — the parent mounts this component permanently, so
  // without this the next real config change after a dismissal would be
  // silently swallowed.
  useEffect(() => {
    setDismissed(false)
  }, [visible])

  if (!visible || dismissed) return null

  const gated = working || busy
  const gateReason = working
    ? 'The agent is working — restart when the current turn finishes'
    : busy
      ? 'Another action is in progress'
      : ''

  const confirm = () => {
    setConfirming(false)
    onRestart()
  }

  const banner = (
    <Box
      data-testid="agent-restart-required-banner"
      role="status"
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        px: 1.5,
        py: 1,
        mb: 1,
        borderRadius: 1,
        border: `1px solid ${alpha(theme.palette.warning.main, 0.35)}`,
        backgroundColor: alpha(theme.palette.warning.main, 0.08),
      }}
    >
      <RotateCcw size={18} strokeWidth={1.8} />
      <Typography
        variant="body2"
        sx={{ flexGrow: 1, fontSize: '0.8rem', fontFamily: APP_FONT_FAMILY }}
      >
        Tool and instruction changes apply after a restart.
      </Typography>
      <Stack direction="row" alignItems="center" spacing={0.75}>
        <Button size="small" onClick={() => setDismissed(true)}>
          Not now
        </Button>
        <Tooltip title={gateReason}>
          <span>
            <Button
              size="small"
              variant="contained"
              color="secondary"
              disabled={gated}
              onClick={() => setConfirming(true)}
            >
              Restart sandbox
            </Button>
          </span>
        </Tooltip>
      </Stack>
    </Box>
  )

  return (
    <>
      {sticky ? (
        // Opaque backdrop behind the banner's translucent warning tint —
        // without it, scrolled content shows through the pinned banner.
        <Box
          data-testid="agent-restart-required-banner-sticky-wrapper"
          sx={{
            position: 'sticky',
            top: 0,
            zIndex: (t) => t.zIndex.appBar - 1,
            backgroundColor: 'background.default',
            pt: 1,
            pb: 0.5,
          }}
        >
          {banner}
        </Box>
      ) : banner}

      <Dialog open={confirming} onClose={() => setConfirming(false)}>
        <DialogTitle>Restart sandbox?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Restarts the sandbox with a fresh conversation. The workspace and
            committed work are kept; the current chat history is discarded.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button data-testid="agent-restart-cancel" onClick={() => setConfirming(false)}>
            Cancel
          </Button>
          <Button
            data-testid="agent-restart-confirm"
            variant="contained"
            color="secondary"
            onClick={confirm}
          >
            Restart
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default AgentRestartRequiredBanner
