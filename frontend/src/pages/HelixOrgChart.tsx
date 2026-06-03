import { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import Divider from '@mui/material/Divider'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import CloseIcon from '@mui/icons-material/Close'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useAccount from '../hooks/useAccount'
import useLightTheme from '../hooks/useLightTheme'
import useSnackbar from '../hooks/useSnackbar'
import {
  ChartNode,
  HireWorkerRequest,
  WorkerBadge,
  useCreateHelixOrgPosition,
  useCreateHelixOrgRole,
  useDeleteHelixOrgPosition,
  useDeleteHelixOrgRole,
  useFireHelixOrgWorker,
  useHelixOrgChart,
  useHelixOrgWorker,
  useHireHelixOrgWorker,
} from '../services/helixOrgService'

// The chart visualises a two-level hierarchy:
//   Role (group container)
//   └── Position (card; holds 0 or 1 Worker)
//
// Multiple Positions can share a Role. The owner Role + owner
// Position + owner Worker are server-side protected from deletion.

const OWNER_ROLE = 'r-owner'
const OWNER_WORKER = 'w-owner'
const ROOT_POSITION = 'p-root'

// flatten walks the chart tree into a flat list of (position, workers)
// pairs. The chart can come back nested (Children) when positions
// have a parent_id chain — for the new Role-grouped view we ignore
// that hierarchy and flatten everything.
type FlatPosition = ChartNode & { workers: WorkerBadge[] }

const flatten = (roots: ChartNode[]): FlatPosition[] => {
  const out: FlatPosition[] = []
  const walk = (n: ChartNode) => {
    out.push({ ...n, workers: n.workers ?? [] })
    ;(n.children ?? []).forEach(walk)
  }
  roots.forEach(walk)
  return out
}

// groupByRole returns one entry per known role (from the chart's
// roles list, falling back to whatever role_ids appear on positions
// when the payload is older). Each entry carries every position whose
// role_id matches, sorted by position id.
type RoleGroup = { roleId: string; positions: FlatPosition[] }

const groupByRole = (positions: FlatPosition[], knownRoles: string[]): RoleGroup[] => {
  const byRole = new Map<string, FlatPosition[]>()
  for (const r of knownRoles) {
    if (!byRole.has(r)) byRole.set(r, [])
  }
  for (const p of positions) {
    const list = byRole.get(p.role_id) ?? []
    list.push(p)
    byRole.set(p.role_id, list)
  }
  const out: RoleGroup[] = []
  byRole.forEach((positions, roleId) => {
    out.push({
      roleId,
      positions: positions.slice().sort((a, b) => a.position_id.localeCompare(b.position_id)),
    })
  })
  // Owner role first, then alphabetical.
  out.sort((a, b) => {
    if (a.roleId === OWNER_ROLE) return -1
    if (b.roleId === OWNER_ROLE) return 1
    return a.roleId.localeCompare(b.roleId)
  })
  return out
}

// ---- Position card ------------------------------------------------------

const PositionCard: FC<{
  position: FlatPosition
  onSelectWorker: (workerId: string) => void
  onHire: (positionId: string) => void
  onDeletePosition: (positionId: string) => void
}> = ({ position, onSelectWorker, onHire, onDeletePosition }) => {
  const lightTheme = useLightTheme()
  const worker = position.workers[0]
  const isRoot = position.position_id === ROOT_POSITION

  const border = lightTheme.isLight ? 'rgba(0,0,0,0.12)' : 'rgba(255,255,255,0.16)'
  const bg = lightTheme.isLight ? '#fff' : 'rgba(255,255,255,0.04)'
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'

  return (
    <Box
      sx={{
        minWidth: 220,
        maxWidth: 260,
        border: `1px solid ${border}`,
        borderRadius: 1.5,
        backgroundColor: bg,
        p: 1.5,
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
      }}
    >
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start">
        <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: muted }}>
          {position.position_id}
        </Typography>
        {!isRoot && (
          <Tooltip title="Delete position (fires its worker)">
            <IconButton
              size="small"
              onClick={() => onDeletePosition(position.position_id)}
              sx={{ p: 0.25, color: muted }}
            >
              <DeleteOutlineIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        )}
      </Stack>

      {worker ? (
        <Box
          onClick={() => onSelectWorker(worker.id)}
          sx={{
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            p: 1,
            borderRadius: 1,
            border: `1px solid ${border}`,
            backgroundColor: lightTheme.isLight ? 'rgba(0,0,0,0.02)' : 'rgba(255,255,255,0.02)',
            '&:hover': {
              backgroundColor: lightTheme.isLight ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.06)',
            },
          }}
        >
          {worker.kind === 'ai' ? (
            <SmartToyOutlinedIcon sx={{ fontSize: 18, color: muted }} />
          ) : (
            <PersonOutlineIcon sx={{ fontSize: 18, color: muted }} />
          )}
          <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem', fontWeight: 600 }}>
            {worker.id}
          </Typography>
        </Box>
      ) : (
        <Button
          variant="outlined"
          size="small"
          startIcon={<AddIcon sx={{ fontSize: 16 }} />}
          onClick={() => onHire(position.position_id)}
          sx={{ textTransform: 'none', justifyContent: 'flex-start' }}
        >
          Hire worker
        </Button>
      )}
    </Box>
  )
}

// ---- Role group --------------------------------------------------------

const RoleGroupCard: FC<{
  group: RoleGroup
  onSelectWorker: (workerId: string) => void
  onHire: (positionId: string) => void
  onAddPosition: (roleId: string) => void
  onDeletePosition: (positionId: string) => void
  onDeleteRole: (roleId: string) => void
}> = ({ group, onSelectWorker, onHire, onAddPosition, onDeletePosition, onDeleteRole }) => {
  const lightTheme = useLightTheme()
  const isOwner = group.roleId === OWNER_ROLE

  const border = lightTheme.isLight ? 'rgba(0,0,0,0.1)' : 'rgba(255,255,255,0.1)'
  const bg = lightTheme.isLight ? 'rgba(0,0,0,0.02)' : 'rgba(255,255,255,0.02)'
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.6)' : 'rgba(255,255,255,0.6)'
  const titleColor = lightTheme.isLight ? 'rgba(0,0,0,0.85)' : 'rgba(255,255,255,0.9)'

  return (
    <Box
      sx={{
        border: `1px solid ${border}`,
        borderRadius: 2,
        backgroundColor: bg,
        p: 2.5,
        display: 'flex',
        flexDirection: 'column',
        gap: 2,
      }}
    >
      <Stack direction="row" justifyContent="space-between" alignItems="center">
        <Stack direction="row" spacing={1.5} alignItems="baseline">
          <Typography variant="h6" sx={{ fontWeight: 700, color: titleColor, fontFamily: 'monospace' }}>
            {group.roleId}
          </Typography>
          <Typography variant="caption" sx={{ color: muted }}>
            {group.positions.length} {group.positions.length === 1 ? 'position' : 'positions'}
          </Typography>
        </Stack>
        <Stack direction="row" spacing={0.5}>
          <Tooltip title="Add a position under this role">
            <Button
              size="small"
              variant="outlined"
              startIcon={<AddIcon sx={{ fontSize: 16 }} />}
              onClick={() => onAddPosition(group.roleId)}
              sx={{ textTransform: 'none' }}
            >
              Position
            </Button>
          </Tooltip>
          {!isOwner && (
            <Tooltip title="Delete role (cascade: positions + workers)">
              <IconButton
                size="small"
                onClick={() => onDeleteRole(group.roleId)}
                sx={{ color: muted }}
              >
                <DeleteOutlineIcon sx={{ fontSize: 18 }} />
              </IconButton>
            </Tooltip>
          )}
        </Stack>
      </Stack>

      {group.positions.length === 0 ? (
        <Typography variant="body2" sx={{ color: muted, fontStyle: 'italic' }}>
          No positions yet — click <strong>Position</strong> to add one.
        </Typography>
      ) : (
        <Stack direction="row" sx={{ flexWrap: 'wrap', gap: 1.5 }}>
          {group.positions.map((p) => (
            <PositionCard
              key={p.position_id}
              position={p}
              onSelectWorker={onSelectWorker}
              onHire={onHire}
              onDeletePosition={onDeletePosition}
            />
          ))}
        </Stack>
      )}
    </Box>
  )
}

// ---- Create-Role dialog -------------------------------------------------

const CreateRoleDialog: FC<{
  open: boolean
  onClose: () => void
}> = ({ open, onClose }) => {
  const snackbar = useSnackbar()
  const create = useCreateHelixOrgRole()
  const [id, setId] = useState('')
  const [content, setContent] = useState('')

  const submit = async () => {
    const trimmedId = id.trim()
    if (!trimmedId) {
      snackbar.error('Role ID is required')
      return
    }
    try {
      await create.mutateAsync({ id: trimmedId, content })
      snackbar.success(`role ${trimmedId} created`)
      setId('')
      setContent('')
      onClose()
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

// ---- Create-Position dialog --------------------------------------------

const CreatePositionDialog: FC<{
  open: boolean
  roleId: string | null
  onClose: () => void
}> = ({ open, roleId, onClose }) => {
  const snackbar = useSnackbar()
  const create = useCreateHelixOrgPosition()
  const [id, setId] = useState('')

  const submit = async () => {
    const trimmedId = id.trim()
    if (!trimmedId) {
      snackbar.error('Position ID is required')
      return
    }
    if (!roleId) return
    try {
      await create.mutateAsync({ id: trimmedId, role_id: roleId, parent_id: ROOT_POSITION })
      snackbar.success(`position ${trimmedId} created`)
      setId('')
      onClose()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'create position failed')
    }
  }

  return (
    <Dialog open={open && !!roleId} onClose={onClose} fullWidth maxWidth="sm">
      <DialogTitle>New position</DialogTitle>
      <DialogContent>
        <Stack spacing={2} sx={{ pt: 1 }}>
          <Box>
            <Typography variant="caption" color="text.secondary">Role</Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{roleId}</Typography>
          </Box>
          <TextField
            label="Position ID"
            placeholder="p-eng-1"
            value={id}
            onChange={(e) => setId(e.target.value)}
            helperText="Convention: p-<kebab-case>. The Worker hired into this position fills the slot."
            autoFocus
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

// ---- Confirm-delete dialog ---------------------------------------------

const ConfirmDeleteDialog: FC<{
  open: boolean
  title: string
  body: string
  onConfirm: () => Promise<void> | void
  onClose: () => void
  pending: boolean
}> = ({ open, title, body, onConfirm, onClose, pending }) => (
  <Dialog open={open} onClose={onClose} fullWidth maxWidth="sm">
    <DialogTitle>{title}</DialogTitle>
    <DialogContent>
      <DialogContentText sx={{ whiteSpace: 'pre-wrap' }}>{body}</DialogContentText>
    </DialogContent>
    <DialogActions>
      <Button onClick={onClose}>Cancel</Button>
      <Button onClick={() => onConfirm()} color="error" variant="contained" disabled={pending}>
        {pending ? 'Deleting…' : 'Delete'}
      </Button>
    </DialogActions>
  </Dialog>
)

// ---- Hire drawer --------------------------------------------------------

const HireDrawer: FC<{
  positionId: string
  onClose: () => void
}> = ({ positionId, onClose }) => {
  const snackbar = useSnackbar()
  const hire = useHireHelixOrgWorker()
  const [id, setId] = useState('')
  const [kind, setKind] = useState<'ai' | 'human'>('human')
  const [identity, setIdentity] = useState('')

  const submit = async () => {
    if (!identity.trim()) {
      snackbar.error('identity content is required')
      return
    }
    const body: HireWorkerRequest = {
      position_id: positionId,
      kind,
      identity_content: identity,
    }
    if (id.trim()) body.id = id.trim()
    try {
      const res = await hire.mutateAsync(body)
      snackbar.success(`hired ${res.id}`)
      setId('')
      setIdentity('')
      onClose()
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'hire failed')
    }
  }

  return (
    <Box sx={{ p: 2.5, width: 380 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
        <Typography variant="h6">Hire worker</Typography>
        <IconButton size="small" onClick={onClose}><CloseIcon /></IconButton>
      </Stack>
      <Stack spacing={1.5}>
        <Box>
          <Typography variant="caption" color="text.secondary">Position</Typography>
          <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{positionId}</Typography>
        </Box>
        <Divider sx={{ my: 1 }} />
        <TextField select size="small" label="Kind" value={kind} onChange={(e) => setKind(e.target.value as 'ai' | 'human')} fullWidth>
          <MenuItem value="human">Human</MenuItem>
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
          size="small"
          label="Identity content"
          placeholder="Short persona / profile in markdown."
          value={identity}
          onChange={(e) => setIdentity(e.target.value)}
          multiline
          minRows={6}
          fullWidth
        />
        <Stack direction="row" spacing={1} sx={{ pt: 1 }}>
          <Button variant="contained" onClick={submit} disabled={hire.isPending}>
            {hire.isPending ? 'Hiring…' : 'Hire'}
          </Button>
          <Button variant="text" onClick={onClose}>Cancel</Button>
        </Stack>
      </Stack>
    </Box>
  )
}

// ---- Worker drawer ------------------------------------------------------

const WorkerDrawer: FC<{
  workerId: string
  onClose: () => void
}> = ({ workerId, onClose }) => {
  const snackbar = useSnackbar()
  const { data, isLoading } = useHelixOrgWorker(workerId)
  const fire = useFireHelixOrgWorker()
  const [confirming, setConfirming] = useState(false)

  const isOwner = workerId === OWNER_WORKER

  const fireWorker = async () => {
    try {
      await fire.mutateAsync(workerId)
      snackbar.success(`fired ${workerId}`)
      onClose()
    } catch (err: any) {
      const status = err?.response?.status
      const msg = err?.response?.data?.error ?? err?.message ?? 'fire failed'
      if (status === 409) {
        snackbar.error('owner worker is protected and cannot be fired')
      } else {
        snackbar.error(msg)
      }
    }
  }

  return (
    <Box sx={{ p: 2.5, width: 380 }}>
      <Stack direction="row" justifyContent="space-between" alignItems="center" sx={{ mb: 2 }}>
        <Typography variant="h6">Worker</Typography>
        <IconButton size="small" onClick={onClose}><CloseIcon /></IconButton>
      </Stack>
      {isLoading || !data ? (
        <LoadingSpinner />
      ) : (
        <Stack spacing={1.5}>
          <Box>
            <Typography variant="caption" color="text.secondary">ID</Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{data.worker.id}</Typography>
          </Box>
          <Stack direction="row" spacing={2}>
            <Box>
              <Typography variant="caption" color="text.secondary">Kind</Typography>
              <Typography variant="body2">{data.worker.kind}</Typography>
            </Box>
            {data.position && (
              <Box>
                <Typography variant="caption" color="text.secondary">Position</Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{data.position.id}</Typography>
              </Box>
            )}
            {data.role && (
              <Box>
                <Typography variant="caption" color="text.secondary">Role</Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{data.role.id}</Typography>
              </Box>
            )}
          </Stack>
          {data.worker.identity_content && (
            <Box>
              <Typography variant="caption" color="text.secondary">Identity</Typography>
              <Box
                component="pre"
                sx={{
                  mt: 0.5,
                  p: 1.5,
                  borderRadius: 1,
                  backgroundColor: (theme) => theme.palette.mode === 'light' ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.04)',
                  fontSize: '0.75rem',
                  whiteSpace: 'pre-wrap',
                  fontFamily: 'monospace',
                  maxHeight: 220,
                  overflow: 'auto',
                }}
              >
                {data.worker.identity_content}
              </Box>
            </Box>
          )}
          <Divider sx={{ my: 1 }} />
          <Stack direction="row" spacing={1}>
            {confirming ? (
              <>
                <Button
                  variant="contained"
                  color="error"
                  size="small"
                  startIcon={<DeleteOutlineIcon />}
                  onClick={fireWorker}
                  disabled={fire.isPending}
                >
                  {fire.isPending ? 'Firing…' : 'Confirm fire'}
                </Button>
                <Button size="small" onClick={() => setConfirming(false)}>Cancel</Button>
              </>
            ) : (
              <Button
                variant="outlined"
                color="error"
                size="small"
                startIcon={<DeleteOutlineIcon />}
                onClick={() => setConfirming(true)}
                disabled={isOwner}
              >
                {isOwner ? 'Owner — protected' : 'Fire'}
              </Button>
            )}
          </Stack>
        </Stack>
      )}
    </Box>
  )
}

// ---- Page ---------------------------------------------------------------

type Selection =
  | { kind: 'none' }
  | { kind: 'hire'; positionId: string }
  | { kind: 'worker'; workerId: string }

const HelixOrgChart: FC = () => {
  const account = useAccount()
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const { data, isLoading } = useHelixOrgChart()
  const deleteRole = useDeleteHelixOrgRole()
  const deletePosition = useDeleteHelixOrgPosition()

  const flat = useMemo(() => flatten(data?.roots ?? []), [data])
  const knownRoles = useMemo(() => (data?.roles ?? []).map((r) => r.id), [data])
  const groups = useMemo(() => groupByRole(flat, knownRoles), [flat, knownRoles])

  const [selection, setSelection] = useState<Selection>({ kind: 'none' })
  const [roleDialogOpen, setRoleDialogOpen] = useState(false)
  const [positionDialogRole, setPositionDialogRole] = useState<string | null>(null)
  const [confirmDelete, setConfirmDelete] = useState<
    | { kind: 'role'; id: string }
    | { kind: 'position'; id: string }
    | null
  >(null)

  const titleColor = lightTheme.isLight ? 'rgba(0,0,0,0.87)' : 'rgba(255,255,255,0.95)'
  const subtitleColor = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'

  const handleConfirmDelete = async () => {
    if (!confirmDelete) return
    try {
      if (confirmDelete.kind === 'role') {
        await deleteRole.mutateAsync(confirmDelete.id)
        snackbar.success(`deleted role ${confirmDelete.id}`)
      } else {
        await deletePosition.mutateAsync(confirmDelete.id)
        snackbar.success(`deleted position ${confirmDelete.id}`)
      }
      setConfirmDelete(null)
    } catch (err: any) {
      const status = err?.response?.status
      const msg = err?.response?.data?.error ?? err?.message ?? 'delete failed'
      if (status === 409) {
        snackbar.error(`${confirmDelete.kind} is protected and cannot be deleted`)
      } else {
        snackbar.error(msg)
      }
    }
  }

  const confirmBody = useMemo(() => {
    if (!confirmDelete) return ''
    if (confirmDelete.kind === 'role') {
      const group = groups.find((g) => g.roleId === confirmDelete.id)
      const positions = group?.positions ?? []
      const workers = positions.flatMap((p) => p.workers.map((w) => w.id))
      const lines = [
        `Deleting role ${confirmDelete.id} will cascade:`,
        `  • ${positions.length} position${positions.length === 1 ? '' : 's'} (${positions.map((p) => p.position_id).join(', ') || 'none'})`,
        `  • ${workers.length} worker${workers.length === 1 ? '' : 's'} (${workers.join(', ') || 'none'})`,
        '',
        'This is irreversible.',
      ]
      return lines.join('\n')
    }
    const pos = flat.find((p) => p.position_id === confirmDelete.id)
    const worker = pos?.workers[0]
    const lines = [
      `Deleting position ${confirmDelete.id} will cascade:`,
      worker ? `  • fire worker ${worker.id}` : '  • no worker to fire',
      '',
      'This is irreversible.',
    ]
    return lines.join('\n')
  }, [confirmDelete, groups, flat])

  return (
    <Page
      breadcrumbTitle="Chart"
      orgBreadcrumbs={true}
      organizationId={account.organizationTools.organization?.id}
      globalSearch={true}
      notifications={true}
    >
      <Box sx={{ display: 'flex', flexDirection: 'column', minHeight: 0 }}>
        <Box sx={{ px: 4, pt: 4, pb: 2 }}>
          <Stack direction="row" justifyContent="space-between" alignItems="flex-start">
            <Box>
              <Typography
                variant="h4"
                sx={{ fontWeight: 700, mb: 1, color: titleColor, letterSpacing: '-0.02em' }}
              >
                Chart
              </Typography>
              <Typography variant="body2" sx={{ color: subtitleColor }}>
                Roles group Positions. Each Position holds one Worker. Add a Role, add Positions inside it, then hire a Worker into each.
              </Typography>
            </Box>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => setRoleDialogOpen(true)}
            >
              New role
            </Button>
          </Stack>
        </Box>

        <Box sx={{ px: 4, pb: 4, flex: 1, minHeight: 0 }}>
          {isLoading ? (
            <Box sx={{ p: 4 }}><LoadingSpinner /></Box>
          ) : groups.length === 0 ? (
            <Typography variant="body1" sx={{ color: subtitleColor }}>
              No roles yet. Click <strong>New role</strong> to get started.
            </Typography>
          ) : (
            <Stack spacing={2.5}>
              {groups.map((g) => (
                <RoleGroupCard
                  key={g.roleId}
                  group={g}
                  onSelectWorker={(workerId) => setSelection({ kind: 'worker', workerId })}
                  onHire={(positionId) => setSelection({ kind: 'hire', positionId })}
                  onAddPosition={(roleId) => setPositionDialogRole(roleId)}
                  onDeletePosition={(positionId) => setConfirmDelete({ kind: 'position', id: positionId })}
                  onDeleteRole={(roleId) => setConfirmDelete({ kind: 'role', id: roleId })}
                />
              ))}
            </Stack>
          )}
        </Box>
      </Box>

      <CreateRoleDialog open={roleDialogOpen} onClose={() => setRoleDialogOpen(false)} />
      <CreatePositionDialog
        open={positionDialogRole !== null}
        roleId={positionDialogRole}
        onClose={() => setPositionDialogRole(null)}
      />
      <ConfirmDeleteDialog
        open={confirmDelete !== null}
        title={confirmDelete?.kind === 'role' ? 'Delete role?' : 'Delete position?'}
        body={confirmBody}
        onConfirm={handleConfirmDelete}
        onClose={() => setConfirmDelete(null)}
        pending={deleteRole.isPending || deletePosition.isPending}
      />

      <Drawer
        anchor="right"
        open={selection.kind !== 'none'}
        onClose={() => setSelection({ kind: 'none' })}
        PaperProps={{ sx: { backgroundImage: 'none' } }}
      >
        {selection.kind === 'hire' && (
          <HireDrawer
            positionId={selection.positionId}
            onClose={() => setSelection({ kind: 'none' })}
          />
        )}
        {selection.kind === 'worker' && (
          <WorkerDrawer
            workerId={selection.workerId}
            onClose={() => setSelection({ kind: 'none' })}
          />
        )}
      </Drawer>
    </Page>
  )
}

export default HelixOrgChart
