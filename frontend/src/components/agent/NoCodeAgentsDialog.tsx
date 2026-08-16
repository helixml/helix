import { FC } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import useRouter from '../../hooks/useRouter'
import AgentHarness from './AgentHarness'

// The agents named in the copy, in the same order, so the marks read as a
// caption for the sentence above them rather than an unrelated row of logos.
const HARNESSES = ['claude_code', 'codex_cli', 'opencode', 'zed_agent']

/**
 * Shown when a task surface has no coding agent it can start with.
 *
 * Framed as the next onboarding step rather than an error: nothing has gone
 * wrong, the org simply has not picked a harness yet. One action and no dismiss
 * button — clicking outside closes it.
 */
const NoCodeAgentsDialog: FC<{
  open: boolean
  onClose: () => void
}> = ({ open, onClose }) => {
  const router = useRouter()
  const orgId = router.params.org_id

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth="xs"
      fullWidth
      PaperProps={{ sx: { borderRadius: 1.5 } }}
    >
      <DialogContent sx={{ px: 3, pt: 3, pb: 1 }}>
        <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 0.5 }}>
          Configure harness
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Configure your coding agent like Claude Code, Codex, OpenCode or Zed.
        </Typography>
        <Stack direction="row" spacing={1.5} alignItems="center" sx={{ mt: 2.5, color: 'text.secondary' }}>
          {HARNESSES.map((runtime) => (
            <AgentHarness key={runtime} runtime={runtime} variant="short" size={22} />
          ))}
        </Stack>
      </DialogContent>
      <DialogActions sx={{ px: 3, pb: 2.5, pt: 1 }}>
        <Button
          variant="contained"
          disabled={!orgId}
          onClick={() => {
            onClose()
            router.navigate('org_providers', { org_id: orgId })
          }}
        >
          Configure
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default NoCodeAgentsDialog
