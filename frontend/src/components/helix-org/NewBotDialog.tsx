// NewBotDialog is the shared "create bot" dialog used by the Chart
// canvas's floating top-right button / per-node "new bot" affordance and
// the Bots list's "+ New bot" header action. A Bot is created in one
// step: a display name, an id (the immutable handle, auto-derived from the
// name but overridable), its content (markdown prompt), and an optional
// parent bot it reports to.

import { FC, useEffect, useMemo, useState } from 'react'
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

  const [name, setName] = useState('')
  const [id, setId] = useState('')
  const [idEdited, setIdEdited] = useState(false)
  const [content, setContent] = useState('')
  const [parentId, setParentId] = useState(presetParentId ?? '')

  useEffect(() => {
    if (!open) return
    setName('')
    setId('')
    setIdEdited(false)
    setContent('')
    setParentId(presetParentId ?? '')
  }, [open, presetParentId])

  const bots = botsData ?? []
  const existingIds = useMemo(() => new Set(bots.map((b) => b.id)), [bots])

  // Append -1, -2, ... to a base slug until it's free within the org. Matches
  // the backend's suffix-on-conflict (bots.Create), so the previewed id is what
  // actually gets stored.
  const uniqueSlug = (base: string): string => {
    if (!base || !existingIds.has(base)) return base
    for (let i = 1; i < 100; i++) {
      const cand = `${base}-${i}`
      if (!existingIds.has(cand)) return cand
    }
    return base
  }

  // Slugify a display name into a kebab-case handle for the id field, unless
  // the operator has typed their own id. Two bots named the same (e.g. a second
  // "Chief of Staff") would otherwise derive the same id and collide.
  const onNameChange = (value: string) => {
    setName(value)
    if (!idEdited) {
      const base = value.toLowerCase().trim().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '')
      setId(uniqueSlug(base))
    }
  }

  const submit = async () => {
    const trimmedId = id.trim()
    if (!trimmedId) {
      snackbar.error('Bot ID is required')
      return
    }
    try {
      const res = await create.mutateAsync({
        id: trimmedId,
        name: name.trim(),
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
            label="Name"
            placeholder="Chief of Staff"
            value={name}
            onChange={(e) => onNameChange(e.target.value)}
            helperText="Human-readable display name shown in the chart and bot page."
            autoFocus
            fullWidth
          />
          <TextField
            label="Bot ID"
            placeholder="chief-of-staff"
            value={id}
            onChange={(e) => { setIdEdited(true); setId(e.target.value) }}
            helperText="Immutable kebab-case handle (auto-filled from the name). Referenced by the LLM, repos and MCP tools; can be anything."
            fullWidth
            sx={{ '& input': { fontFamily: 'monospace' } }}
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
                <MenuItem key={b.id} value={b.id ?? ''}>
                  {b.name || b.id}
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
