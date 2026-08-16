import { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogContent from '@mui/material/DialogContent'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import useRouter from '../../hooks/useRouter'

/**
 * Shown when a task surface has no coding agent it can start with.
 *
 * Framed as the next onboarding step rather than an error: nothing has gone
 * wrong, the org simply has not picked a harness yet. One action and no dismiss
 * button — clicking outside closes it, so the only thing to press is the thing
 * worth pressing.
 */
const NoCodeAgentsDialog: FC<{
  open: boolean
  onClose: () => void
}> = ({ open, onClose }) => {
  const router = useRouter()
  const orgId = router.params.org_id

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogContent sx={{ py: 3 }}>
        <Stack spacing={1}>
          <Typography variant="h6">Configure harness</Typography>
          <Typography variant="body2" color="text.secondary">
            Configure your coding agent like Claude Code, Codex, OpenCode or Zed.
          </Typography>
          <Box sx={{ pt: 1.5 }}>
            <Button
              variant="contained"
              color="secondary"
              disabled={!orgId}
              onClick={() => {
                onClose()
                router.navigate('org_providers', { org_id: orgId })
              }}
            >
              Configure providers
            </Button>
          </Box>
        </Stack>
      </DialogContent>
    </Dialog>
  )
}

export default NoCodeAgentsDialog
