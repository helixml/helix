// HireWorkerDrawer is the shared hire form rendered from three places:
//   - the Chart canvas's per-role + icon (presetRoleId set; Role field
//     is read-only),
//   - the Workers tab's "+ New Worker" button (presetRoleId omitted; a
//     Role <select> is rendered and required),
//   - any future entry point that wants to hire (same API).
//
// Form fields: Role, Kind, Handle (optional), Reports to (optional),
// Identity content. parent_id is already part of HireWorkerRequest, so
// no service-layer change is needed to wire the new selector.

import { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Divider from '@mui/material/Divider'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import CloseIcon from '@mui/icons-material/Close'

import useSnackbar from '../../hooks/useSnackbar'
import {
  HireWorkerRequest,
  useHireHelixOrgWorker,
  useListHelixOrgRoles,
  useListHelixOrgWorkers,
} from '../../services/helixOrgService'

export type HireWorkerDrawerProps = {
  open: boolean
  onClose: () => void
  // When set, the Role field is rendered read-only and prefilled. When
  // omitted, a required Role <select> is rendered.
  presetRoleId?: string
}

const HireWorkerDrawer: FC<HireWorkerDrawerProps> = ({ open, onClose, presetRoleId }) => {
  const snackbar = useSnackbar()
  const hire = useHireHelixOrgWorker()
  const { data: rolesData } = useListHelixOrgRoles({ enabled: open && !presetRoleId })
  const { data: workersData } = useListHelixOrgWorkers({ enabled: open })

  const [id, setId] = useState('')
  // Human workers aren't supported yet — default to AI and disable the
  // Human option below.
  const [kind, setKind] = useState<'ai' | 'human'>('ai')
  const [identity, setIdentity] = useState('')
  const [roleId, setRoleId] = useState(presetRoleId ?? '')
  const [parentId, setParentId] = useState('')

  // Reset form on each open so reopening doesn't show stale state from
  // a previous hire.
  useEffect(() => {
    if (!open) return
    setId('')
    setKind('ai')
    setIdentity('')
    setRoleId(presetRoleId ?? '')
    setParentId('')
  }, [open, presetRoleId])

  const roles = rolesData ?? []
  const workers = workersData ?? []

  const canSubmit = Boolean(identity.trim()) && Boolean(roleId)

  const submit = async () => {
    if (!identity.trim()) {
      snackbar.error('identity content is required')
      return
    }
    if (!roleId) {
      snackbar.error('role is required')
      return
    }
    const body: HireWorkerRequest = {
      role_id: roleId,
      kind,
      identity_content: identity,
    }
    if (id.trim()) body.id = id.trim()
    if (parentId) body.parent_id = parentId
    try {
      const res = await hire.mutateAsync(body)
      if (parentId) {
        snackbar.success(`hired ${res.id} reporting to ${parentId}`)
      } else {
        snackbar.success(`hired ${res.id} — drag an edge from a manager to set who they report to`)
      }
      onClose()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'hire failed')
    }
  }

  return (
    <Drawer
      anchor="right"
      open={open}
      onClose={onClose}
      PaperProps={{ sx: { backgroundImage: 'none' } }}
    >
      <Box sx={{ p: 2.5, width: 380 }}>
        <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
          <Typography variant="h6">Hire worker</Typography>
          <IconButton size="small" onClick={onClose}><CloseIcon /></IconButton>
        </Stack>
        <Stack spacing={1.5}>
          {presetRoleId ? (
            <Box>
              <Typography variant="caption" color="text.secondary">Role</Typography>
              <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{presetRoleId}</Typography>
            </Box>
          ) : (
            <TextField
              select
              size="small"
              label="Role"
              value={roleId}
              onChange={(e) => setRoleId(e.target.value)}
              fullWidth
              required
              helperText={roles.length === 0 ? 'No roles defined yet — create one first.' : 'Job description + MCP tools the worker holds.'}
            >
              {roles.map((r) => (
                <MenuItem key={r.id} value={r.id ?? ''} sx={{ fontFamily: 'monospace' }}>
                  {r.id}
                </MenuItem>
              ))}
            </TextField>
          )}
          <Divider sx={{ my: 1 }} />
          <TextField
            select
            size="small"
            label="Kind"
            value={kind}
            onChange={(e) => setKind(e.target.value as 'ai' | 'human')}
            fullWidth
            helperText="Human workers aren't supported yet."
          >
            <MenuItem value="human" disabled>Human</MenuItem>
            <MenuItem value="ai">AI</MenuItem>
          </TextField>
          <TextField
            size="small"
            label="Handle (optional)"
            placeholder="w-alice"
            helperText="Lowercase first name, prefixed with w-. Leave blank to auto-assign."
            value={id}
            onChange={(e) => setId(e.target.value)}
            fullWidth
          />
          <TextField
            select
            size="small"
            label="Reports to (optional)"
            value={parentId}
            onChange={(e) => setParentId(e.target.value)}
            fullWidth
            helperText="Manager this worker reports to. Leave blank and wire later by dragging an edge in the Chart."
          >
            <MenuItem value="">(none)</MenuItem>
            {workers.map((w) => (
              <MenuItem key={w.id} value={w.id ?? ''} sx={{ fontFamily: 'monospace' }}>
                {w.id}{w.role_id ? ` — ${w.role_id}` : ''}
              </MenuItem>
            ))}
          </TextField>
          <TextField
            size="small"
            label="Identity content"
            placeholder="Short persona / profile in markdown."
            value={identity}
            onChange={(e) => setIdentity(e.target.value)}
            multiline
            minRows={6}
            fullWidth
            required
          />
          <Stack direction="row" spacing={1} sx={{ pt: 1 }}>
            <Button variant="contained" onClick={submit} disabled={hire.isPending || !canSubmit}>
              {hire.isPending ? 'Hiring…' : 'Hire'}
            </Button>
            <Button variant="text" onClick={onClose}>Cancel</Button>
          </Stack>
        </Stack>
      </Box>
    </Drawer>
  )
}

export default HireWorkerDrawer
