// NewBotDialog is the shared "create bot" dialog used by the Chart
// canvas's floating top-right button / per-node "new bot" affordance and
// the Bots list's "+ New bot" header action. A Bot is created in one
// step: an id, its content (markdown identity/prompt), and an optional
// parent bot it reports to. There is no kind selector and no separate
// identity field — a Bot's content IS its identity.

import { FC, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'

import useSnackbar from '../../hooks/useSnackbar'
import { useCreateBot, useListHelixOrgBots } from '../../services/helixOrgService'

export type NewBotDialogProps = {
  open: boolean
  onClose: () => void
  // When set, the Reports-to field is prefilled with this parent bot id.
  presetParentId?: string
}

const NewBotDialog: FC<NewBotDialogProps> = ({ open, onClose, presetParentId }) => {
  const snackbar = useSnackbar()
  const create = useCreateBot()
  const { data: botsData } = useListHelixOrgBots({ enabled: open })

  const [id, setId] = useState('')
  const [content, setContent] = useState('')
  const [parentId, setParentId] = useState(presetParentId ?? '')

  useEffect(() => {
    if (!open) return
    setId('')
    setContent('')
    setParentId(presetParentId ?? '')
  }, [open, presetParentId])

  const bots = botsData ?? []

  const submit = async () => {
    const trimmedId = id.trim()
    if (!trimmedId) {
      snackbar.error('Bot ID is required')
      return
    }
    try {
      const res = await create.mutateAsync({
        id: trimmedId,
        content,
        ...(parentId ? { parent_id: parentId } : {}),
      })
      if (parentId) {
        snackbar.success(`bot ${res.id ?? trimmedId} created, reporting to ${parentId}`)
      } else {
        snackbar.success(`bot ${res.id ?? trimmedId} created — drag an edge from a manager to set who it reports to`)
      }
      onClose()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'create bot failed')
    }
  }

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>New bot</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Bot ID"
            placeholder="engineer"
            value={id}
            onChange={(e) => setId(e.target.value)}
            helperText="A short handle in kebab-case, e.g. engineer. Stays as-is — the LLM and operator both refer to the bot by it."
            autoFocus
            fullWidth
          />
          {presetParentId ? (
            <TextField
              label="Reports to"
              value={presetParentId}
              InputProps={{ readOnly: true }}
              helperText="Manager this bot reports to."
              fullWidth
              sx={{ '& input': { fontFamily: 'monospace' } }}
            />
          ) : (
            <TextField
              select
              label="Reports to (optional)"
              value={parentId}
              onChange={(e) => setParentId(e.target.value)}
              helperText="Manager this bot reports to. Leave blank and wire later by dragging an edge in the Chart."
              fullWidth
            >
              <MenuItem value="">(none)</MenuItem>
              {bots.map((b) => (
                <MenuItem key={b.id} value={b.id ?? ''} sx={{ fontFamily: 'monospace' }}>
                  {b.id}
                </MenuItem>
              ))}
            </TextField>
          )}
          <TextField
            label="Content (markdown)"
            placeholder="# Engineer&#10;Builds and ships software."
            value={content}
            onChange={(e) => setContent(e.target.value)}
            multiline
            minRows={6}
            fullWidth
            helperText="The bot's prompt / identity. Read on every activation."
          />
        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose}>Cancel</Button>
        <Button onClick={submit} variant="contained" disabled={create.isPending}>
          {create.isPending ? 'Creating…' : 'Create'}
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default NewBotDialog
