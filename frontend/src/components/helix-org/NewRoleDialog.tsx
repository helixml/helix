// NewRoleDialog is the shared "create role" dialog used by the Chart
// canvas's floating top-right button and the Roles tab's "+ New Role"
// header action. Lifted from HelixOrgChart.tsx so both call sites get
// the same form, validation, and error handling.

import { FC, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'

import useSnackbar from '../../hooks/useSnackbar'
import { useCreateHelixOrgRole } from '../../services/helixOrgService'

export type NewRoleDialogProps = {
  open: boolean
  onClose: () => void
  onCreated?: (roleId: string) => void
}

const NewRoleDialog: FC<NewRoleDialogProps> = ({ open, onClose, onCreated }) => {
  const snackbar = useSnackbar()
  const create = useCreateHelixOrgRole()
  const [id, setId] = useState('')
  const [content, setContent] = useState('')

  useEffect(() => {
    if (!open) return
    setId('')
    setContent('')
  }, [open])

  const submit = async () => {
    const trimmedId = id.trim()
    if (!trimmedId) {
      snackbar.error('Role ID is required')
      return
    }
    try {
      await create.mutateAsync({ id: trimmedId, content })
      snackbar.success(`role ${trimmedId} created`)
      onClose()
      onCreated?.(trimmedId)
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'create role failed')
    }
  }

  return (
    <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>New role</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <TextField
            label="Role ID"
            placeholder="r-engineer"
            value={id}
            onChange={(e) => setId(e.target.value)}
            helperText="Convention: r-<kebab-case>. Stays as-is — the LLM and operator both refer to roles by this handle."
            autoFocus
            fullWidth
          />
          <TextField
            label="Content (markdown)"
            placeholder="# Engineer&#10;Builds and ships software."
            value={content}
            onChange={(e) => setContent(e.target.value)}
            multiline
            minRows={6}
            fullWidth
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

export default NewRoleDialog
