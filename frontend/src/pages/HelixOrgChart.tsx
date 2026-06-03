import { FC, useCallback, useEffect, useMemo, useState } from 'react'
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

import dagre from 'dagre'
import {
  Background,
  Controls,
  Edge,
  Handle,
  MiniMap,
  Node,
  NodeProps,
  Position as RFPosition,
  ReactFlow,
  ReactFlowProvider,
  useEdgesState,
  useNodesState,
  useReactFlow,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'

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

// The chart visualises the org as a ReactFlow subflow:
//
//   ┌─[Role: r-owner]──────────────────┐
//   │  [Position: p-root]              │
//   │     • w-owner                    │
//   └────────│───────────────────────────┘
//            ↓ (worker-to-worker reporting edge,
//            ↓  derived from position.parent_id chain)
//   ┌─[Role: r-engineer]───────────────────────────────────┐
//   │  [p-eng-1: w-alice]  [p-eng-2: w-bob]  [p-eng-3: …] │
//   └──────────────────────────────────────────────────────┘
//
// Roles are parent group nodes that VISUALLY CONTAIN their Position
// child nodes. Edges link Position → Position based on the position
// parent_id chain; since each Position holds at most one Worker, the
// edge reads as a worker-to-worker reporting line (e.g. w-owner →
// w-alice means alice's position is a child of root, which w-owner
// occupies).
//
// Layout: dagre runs over the position parent tree to get global
// (x, y) for each Position. We then derive each Role group's bounding
// box from its child Positions, and translate the children to be
// parent-relative coords for ReactFlow's subflow.
//
// Empty roles (no positions yet) get a placeholder slot at the right
// edge of the canvas so they're still discoverable + editable.

const OWNER_ROLE = 'r-owner'
const OWNER_WORKER = 'w-owner'
const ROOT_POSITION = 'p-root'

const POSITION_W = 240
const POSITION_H = 140
const POSITION_GAP_X = 40
const POSITION_GAP_Y = 80
const ROLE_PAD_X = 24
const ROLE_PAD_TOP = 56
const ROLE_PAD_BOTTOM = 24
const EMPTY_ROLE_W = 320
const EMPTY_ROLE_H = 140

// ---- Flatten + group ---------------------------------------------------

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
  out.sort((a, b) => {
    if (a.roleId === OWNER_ROLE) return -1
    if (b.roleId === OWNER_ROLE) return 1
    return a.roleId.localeCompare(b.roleId)
  })
  return out
}

// ---- Node renderers ----------------------------------------------------

type RoleNodeData = {
  roleId: string
  positionCount: number
  isOwner: boolean
  onAddPosition: (roleId: string) => void
  onDeleteRole: (roleId: string) => void
}

type PositionNodeData = {
  positionId: string
  workers: WorkerBadge[]
  isRoot: boolean
  onSelectWorker: (workerId: string) => void
  onHire: (positionId: string) => void
  onDeletePosition: (positionId: string) => void
}

// RoleNode is a parent group — ReactFlow renders the child Position
// nodes inside its rect. The Box is sized to fill the node's frame
// (ReactFlow sets style.width/height on the node itself), and just
// paints the header band along the top edge with the role id + the
// add-position / delete-role affordances.
const RoleNode: FC<NodeProps<Node<RoleNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.6)' : 'rgba(255,255,255,0.6)'
  const titleColor = lightTheme.isLight ? 'rgba(0,0,0,0.85)' : 'rgba(255,255,255,0.9)'

  return (
    <Box
      sx={{
        position: 'relative',
        width: '100%',
        height: '100%',
        pointerEvents: 'none',
      }}
    >
      <Box
        sx={{
          position: 'absolute',
          top: 0, left: 0, right: 0,
          height: ROLE_PAD_TOP - 8,
          px: 2,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          pointerEvents: 'auto',
        }}
      >
        <Stack direction="row" alignItems="baseline" spacing={1.5} sx={{ minWidth: 0, flex: 1 }}>
          <Typography
            variant="subtitle1"
            sx={{
              fontWeight: 700,
              color: titleColor,
              fontFamily: 'monospace',
              whiteSpace: 'nowrap',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
            }}
          >
            {data.roleId}
          </Typography>
          <Typography variant="caption" sx={{ color: muted, whiteSpace: 'nowrap' }}>
            {data.positionCount} {data.positionCount === 1 ? 'position' : 'positions'}
          </Typography>
        </Stack>
        <Stack direction="row" spacing={0.5}>
          <Tooltip title="Add a position under this role">
            <Button
              size="small"
              variant="outlined"
              startIcon={<AddIcon sx={{ fontSize: 16 }} />}
              onClick={(e) => { e.stopPropagation(); data.onAddPosition(data.roleId) }}
              sx={{ textTransform: 'none' }}
            >
              Position
            </Button>
          </Tooltip>
          {!data.isOwner && (
            <Tooltip title="Delete role (cascade: positions + workers)">
              <IconButton
                size="small"
                onClick={(e) => { e.stopPropagation(); data.onDeleteRole(data.roleId) }}
                sx={{ color: muted }}
              >
                <DeleteOutlineIcon sx={{ fontSize: 18 }} />
              </IconButton>
            </Tooltip>
          )}
        </Stack>
      </Box>
      {data.positionCount === 0 && (
        <Box
          sx={{
            position: 'absolute',
            top: ROLE_PAD_TOP,
            left: 0,
            right: 0,
            bottom: 0,
            display: 'flex',
            alignItems: 'center',
            justifyContent: 'center',
            color: muted,
            fontStyle: 'italic',
            fontSize: '0.85rem',
            pointerEvents: 'none',
          }}
        >
          No positions yet — click <strong style={{ fontStyle: 'normal', margin: '0 0.25em' }}>Position</strong> to add one
        </Box>
      )}
    </Box>
  )
}

const PositionNode: FC<NodeProps<Node<PositionNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const worker = data.workers[0]
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const border = lightTheme.isLight ? 'rgba(0,0,0,0.14)' : 'rgba(255,255,255,0.18)'
  const bg = lightTheme.isLight ? '#fff' : 'rgba(255,255,255,0.05)'
  const innerBorder = lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.12)'
  const innerBg = lightTheme.isLight ? 'rgba(0,0,0,0.02)' : 'rgba(255,255,255,0.03)'
  const innerHover = lightTheme.isLight ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.06)'
  const handleColor = lightTheme.isLight ? 'rgba(0,0,0,0.35)' : 'rgba(255,255,255,0.35)'

  return (
    <Box
      sx={{
        width: POSITION_W,
        height: POSITION_H,
        border: `1px solid ${border}`,
        borderRadius: 1.5,
        backgroundColor: bg,
        boxShadow: lightTheme.isLight ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
        p: 1.5,
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
      }}
    >
      <Handle type="target" position={RFPosition.Top} style={{ background: handleColor }} />
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start">
        <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: muted }}>
          {data.positionId}
        </Typography>
        {!data.isRoot && (
          <Tooltip title="Delete position (fires its worker)">
            <IconButton
              size="small"
              onClick={(e) => { e.stopPropagation(); data.onDeletePosition(data.positionId) }}
              sx={{ p: 0.25, color: muted }}
            >
              <DeleteOutlineIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        )}
      </Stack>

      {worker ? (
        <Box
          onClick={(e) => { e.stopPropagation(); data.onSelectWorker(worker.id) }}
          sx={{
            cursor: 'pointer',
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            p: 1,
            borderRadius: 1,
            border: `1px solid ${innerBorder}`,
            backgroundColor: innerBg,
            '&:hover': { backgroundColor: innerHover },
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
          onClick={(e) => { e.stopPropagation(); data.onHire(data.positionId) }}
          sx={{ textTransform: 'none', justifyContent: 'flex-start', mt: 'auto' }}
        >
          Hire worker
        </Button>
      )}
      <Handle type="source" position={RFPosition.Bottom} style={{ background: handleColor }} />
    </Box>
  )
}

const nodeTypes = { role: RoleNode, position: PositionNode }

// ---- dagre layout ------------------------------------------------------

// buildGraph computes nodes + edges for the subflow:
//
//   1. Run dagre on the position-parent tree to get global (x, y) for
//      every Position.
//   2. For each Role, compute the bbox of its Positions; that
//      becomes the Role parent node's rect.
//   3. Translate each Position to be parent-relative — ReactFlow
//      subflow children expect coords relative to their parent.
//   4. Edges link position → position based on position.parent_id.
//      They cross role-group boundaries naturally — a Position in
//      r-engineer reporting to one in r-owner draws a worker-to-
//      worker line down the page.
//   5. Empty Roles (no Positions) get a fixed-size slot appended to
//      the right of the connected layout.
const buildGraph = (
  groups: RoleGroup[],
  flat: FlatPosition[],
  handlers: {
    onSelectWorker: (workerId: string) => void
    onHire: (positionId: string) => void
    onAddPosition: (roleId: string) => void
    onDeleteRole: (roleId: string) => void
    onDeletePosition: (positionId: string) => void
  },
  isLight: boolean,
): { nodes: Node[]; edges: Edge[] } => {
  // 1. dagre on the position tree.
  const g = new dagre.graphlib.Graph()
  g.setGraph({
    rankdir: 'TB',
    nodesep: POSITION_GAP_X,
    ranksep: POSITION_GAP_Y + ROLE_PAD_TOP + ROLE_PAD_BOTTOM,
    marginx: 0,
    marginy: 0,
  })
  g.setDefaultEdgeLabel(() => ({}))

  const flatByID = new Map<string, FlatPosition>()
  for (const p of flat) flatByID.set(p.position_id, p)

  for (const p of flat) {
    g.setNode(`pos:${p.position_id}`, { width: POSITION_W, height: POSITION_H })
  }
  for (const p of flat) {
    if (p.parent_id && flatByID.has(p.parent_id)) {
      g.setEdge(`pos:${p.parent_id}`, `pos:${p.position_id}`)
    }
  }
  dagre.layout(g)

  // 2. Compute each role's bbox over its dagre-positioned positions.
  type Box = { x: number; y: number; w: number; h: number }
  const roleBoxes = new Map<string, Box>()
  for (const group of groups) {
    if (group.positions.length === 0) continue
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity
    for (const p of group.positions) {
      const ln = g.node(`pos:${p.position_id}`)
      if (!ln) continue
      const left = ln.x - POSITION_W / 2
      const top = ln.y - POSITION_H / 2
      const right = left + POSITION_W
      const bottom = top + POSITION_H
      if (left < minX) minX = left
      if (top < minY) minY = top
      if (right > maxX) maxX = right
      if (bottom > maxY) maxY = bottom
    }
    roleBoxes.set(group.roleId, {
      x: minX - ROLE_PAD_X,
      y: minY - ROLE_PAD_TOP,
      w: (maxX - minX) + 2 * ROLE_PAD_X,
      h: (maxY - minY) + ROLE_PAD_TOP + ROLE_PAD_BOTTOM,
    })
  }

  // 2b. Resolve horizontal overlap between role frames. dagre spaces
  //     positions by POSITION_GAP_X, which is smaller than 2*ROLE_PAD_X,
  //     so adjacent roles at the same rank visually overlap. Sweep
  //     left-to-right and shift each role (and, implicitly via subflow
  //     coords, its child positions) so frames are separated by
  //     ROLE_GAP_X. Vertical bands at different ranks are left alone.
  const ROLE_GAP_X = POSITION_GAP_X
  const filled = groups
    .filter((g) => g.positions.length > 0 && roleBoxes.has(g.roleId))
    .map((g) => ({ roleId: g.roleId, box: roleBoxes.get(g.roleId)! }))
    .sort((a, b) => a.box.x - b.box.x)
  for (let i = 0; i < filled.length; i++) {
    for (let j = i + 1; j < filled.length; j++) {
      const a = filled[i].box
      const b = filled[j].box
      const yOverlap = !(b.y >= a.y + a.h || a.y >= b.y + b.h)
      if (!yOverlap) continue
      const minLeft = a.x + a.w + ROLE_GAP_X
      if (b.x < minLeft) b.x = minLeft
    }
  }

  // 3. Append empty-role placeholder slots in a column to the right
  //    of the connected layout, so they're discoverable + editable.
  const layoutMaxX = Math.max(0, ...Array.from(roleBoxes.values()).map((b) => b.x + b.w))
  let emptyCursorY = 0
  for (const group of groups) {
    if (group.positions.length > 0) continue
    roleBoxes.set(group.roleId, {
      x: layoutMaxX + POSITION_GAP_X,
      y: emptyCursorY,
      w: EMPTY_ROLE_W,
      h: EMPTY_ROLE_H,
    })
    emptyCursorY += EMPTY_ROLE_H + POSITION_GAP_Y / 2
  }

  // 4. Emit nodes — role parents first, position children second
  //    (ReactFlow needs parents before children in the array).
  const nodes: Node[] = []
  const roleStyle = {
    backgroundColor: isLight ? 'rgba(0,0,0,0.025)' : 'rgba(255,255,255,0.03)',
    border: `1px solid ${isLight ? 'rgba(0,0,0,0.1)' : 'rgba(255,255,255,0.12)'}`,
    borderRadius: 12,
    boxShadow: isLight ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
  }
  for (const group of groups) {
    const box = roleBoxes.get(group.roleId)
    if (!box) continue
    nodes.push({
      id: `role:${group.roleId}`,
      type: 'role',
      position: { x: box.x, y: box.y },
      style: { ...roleStyle, width: box.w, height: box.h },
      data: {
        roleId: group.roleId,
        positionCount: group.positions.length,
        isOwner: group.roleId === OWNER_ROLE,
        onAddPosition: handlers.onAddPosition,
        onDeleteRole: handlers.onDeleteRole,
      } as RoleNodeData,
      draggable: false,
      selectable: false,
    })
  }
  for (const group of groups) {
    const box = roleBoxes.get(group.roleId)
    if (!box) continue
    for (const p of group.positions) {
      const ln = g.node(`pos:${p.position_id}`)
      if (!ln) continue
      const globalX = ln.x - POSITION_W / 2
      const globalY = ln.y - POSITION_H / 2
      nodes.push({
        id: `pos:${p.position_id}`,
        type: 'position',
        parentId: `role:${group.roleId}`,
        extent: 'parent',
        position: { x: globalX - box.x, y: globalY - box.y },
        data: {
          positionId: p.position_id,
          workers: p.workers,
          isRoot: p.position_id === ROOT_POSITION,
          onSelectWorker: handlers.onSelectWorker,
          onHire: handlers.onHire,
          onDeletePosition: handlers.onDeletePosition,
        } as PositionNodeData,
        draggable: false,
      })
    }
  }

  // 5. Edges: worker-to-worker reporting lines, drawn between
  //    Position nodes via the position parent_id chain.
  const edges: Edge[] = []
  for (const p of flat) {
    if (!p.parent_id || !flatByID.has(p.parent_id)) continue
    edges.push({
      id: `pos:${p.parent_id}->pos:${p.position_id}`,
      source: `pos:${p.parent_id}`,
      target: `pos:${p.position_id}`,
      type: 'smoothstep',
      animated: false,
      style: {
        stroke: isLight ? 'rgba(0,0,0,0.3)' : 'rgba(255,255,255,0.35)',
        strokeWidth: 1.5,
      },
    })
  }

  return { nodes, edges }
}

// ---- Dialogs (Create role, Create position, Confirm delete) ------------

const CreateRoleDialog: FC<{ open: boolean; onClose: () => void }> = ({ open, onClose }) => {
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
      setId(''); setContent(''); onClose()
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
      setId(''); onClose()
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

// ---- Hire + Worker drawers ---------------------------------------------

const HireDrawer: FC<{ positionId: string; onClose: () => void }> = ({ positionId, onClose }) => {
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
      setId(''); setIdentity(''); onClose()
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

const WorkerDrawer: FC<{ workerId: string; onClose: () => void }> = ({ workerId, onClose }) => {
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

// ---- ReactFlow canvas --------------------------------------------------

const ChartCanvas: FC<{
  groups: RoleGroup[]
  flat: FlatPosition[]
  handlers: {
    onSelectWorker: (workerId: string) => void
    onHire: (positionId: string) => void
    onAddPosition: (roleId: string) => void
    onDeleteRole: (roleId: string) => void
    onDeletePosition: (positionId: string) => void
  }
}> = ({ groups, flat, handlers }) => {
  const lightTheme = useLightTheme()
  const { fitView } = useReactFlow()

  const { nodes: computedNodes, edges: computedEdges } = useMemo(
    () => buildGraph(groups, flat, handlers, lightTheme.isLight),
    [groups, flat, handlers, lightTheme.isLight],
  )
  const [nodes, setNodes, onNodesChange] = useNodesState(computedNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(computedEdges)

  useEffect(() => {
    setNodes(computedNodes)
    setEdges(computedEdges)
    requestAnimationFrame(() => fitView({ padding: 0.2, duration: 250 }))
  }, [computedNodes, computedEdges, fitView, setNodes, setEdges])

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      nodeTypes={nodeTypes}
      fitView
      fitViewOptions={{ padding: 0.2 }}
      proOptions={{ hideAttribution: true }}
      colorMode={lightTheme.isLight ? 'light' : 'dark'}
      nodesDraggable={false}
      nodesConnectable={false}
      elementsSelectable={false}
      panOnDrag
      zoomOnScroll
    >
      <Background gap={20} size={1} />
      <Controls showInteractive={false} />
      <MiniMap pannable zoomable maskColor={lightTheme.isLight ? 'rgba(0,0,0,0.06)' : 'rgba(0,0,0,0.6)'} />
    </ReactFlow>
  )
}

// ---- Page --------------------------------------------------------------

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
  const canvasBorder = lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'
  const canvasBg = lightTheme.isLight ? '#fafafa' : 'rgba(255,255,255,0.02)'

  const onSelectWorker = useCallback(
    (workerId: string) => setSelection({ kind: 'worker', workerId }),
    [],
  )
  const onHire = useCallback(
    (positionId: string) => setSelection({ kind: 'hire', positionId }),
    [],
  )
  const onAddPosition = useCallback((roleId: string) => setPositionDialogRole(roleId), [])
  const onDeleteRole = useCallback(
    (roleId: string) => setConfirmDelete({ kind: 'role', id: roleId }),
    [],
  )
  const onDeletePosition = useCallback(
    (positionId: string) => setConfirmDelete({ kind: 'position', id: positionId }),
    [],
  )
  const handlers = useMemo(
    () => ({ onSelectWorker, onHire, onAddPosition, onDeleteRole, onDeletePosition }),
    [onSelectWorker, onHire, onAddPosition, onDeleteRole, onDeletePosition],
  )

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
      return [
        `Deleting role ${confirmDelete.id} will cascade:`,
        `  • ${positions.length} position${positions.length === 1 ? '' : 's'} (${positions.map((p) => p.position_id).join(', ') || 'none'})`,
        `  • ${workers.length} worker${workers.length === 1 ? '' : 's'} (${workers.join(', ') || 'none'})`,
        '',
        'This is irreversible.',
      ].join('\n')
    }
    const pos = flat.find((p) => p.position_id === confirmDelete.id)
    const worker = pos?.workers[0]
    return [
      `Deleting position ${confirmDelete.id} will cascade:`,
      worker ? `  • fire worker ${worker.id}` : '  • no worker to fire',
      '',
      'This is irreversible.',
    ].join('\n')
  }, [confirmDelete, groups, flat])

  return (
    <Page
      breadcrumbTitle="Chart"
      orgBreadcrumbs={true}
      organizationId={account.organizationTools.organization?.id}
      globalSearch={true}
      notifications={true}
    >
      <Box sx={{ display: 'flex', flexDirection: 'column', height: 'calc(100vh - 64px)', minHeight: 0 }}>
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

        <Box
          sx={{
            flex: 1,
            minHeight: 0,
            mx: 4,
            mb: 4,
            position: 'relative',
            border: `1px solid ${canvasBorder}`,
            borderRadius: 1,
            backgroundColor: canvasBg,
            overflow: 'hidden',
          }}
        >
          {isLoading ? (
            <Box sx={{ p: 4 }}><LoadingSpinner /></Box>
          ) : groups.length === 0 ? (
            <Box sx={{ p: 4 }}>
              <Typography variant="body1" sx={{ color: subtitleColor }}>
                No roles yet. Click <strong>New role</strong> to get started.
              </Typography>
            </Box>
          ) : (
            <ReactFlowProvider>
              <ChartCanvas groups={groups} flat={flat} handlers={handlers} />
            </ReactFlowProvider>
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
