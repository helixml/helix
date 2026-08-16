import { FC } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { Settings2 } from 'lucide-react'

import useRouter from '../../hooks/useRouter'

/**
 * Shown when a task surface has no coding agent it can start with.
 *
 * The organization either enabled none, or enabled only subscription-backed
 * ones this member has not connected. Both are fixed in the same place, so the
 * dialog links there rather than describing what to do in prose.
 */
const NoCodeAgentsDialog: FC<{
  open: boolean
  onClose: () => void
}> = ({ open, onClose }) => {
  const router = useRouter()
  const orgId = router.params.org_id

  return (
    <Dialog open={open} onClose={onClose} maxWidth="xs" fullWidth>
      <DialogTitle>No coding agents available</DialogTitle>
      <DialogContent>
        <Stack spacing={1.5}>
          <Typography variant="body2" color="text.secondary">
            Tasks need a coding agent to run. This organization has not enabled one
            yet — or the ones it enabled use a personal subscription you have not
            connected.
          </Typography>
          <Typography variant="body2" color="text.secondary">
            Enable an agent and give it a provider or subscription, then come back
            and start your task.
          </Typography>
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Close</Button>
        <Button
          variant="contained"
          startIcon={<Settings2 size={16} />}
          disabled={!orgId}
          onClick={() => {
            onClose()
            router.navigate('org_providers', { org_id: orgId })
          }}
        >
          Configure providers
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default NoCodeAgentsDialog
