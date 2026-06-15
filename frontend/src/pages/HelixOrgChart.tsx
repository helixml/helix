import { FC, useCallback, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import IconButton from '@mui/material/IconButton'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import PersonAddOutlinedIcon from '@mui/icons-material/PersonAddOutlined'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import dagre from 'dagre'
import {
  Background,
  BaseEdge,
  Controls,
  Edge,
  EdgeLabelRenderer,
  EdgeProps,
  getStraightPath,
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
import HireWorkerDrawer from '../components/helix-org/HireWorkerDrawer'
import NewRoleDialog from '../components/helix-org/NewRoleDialog'
import useLightTheme from '../hooks/useLightTheme'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  WorkerDTO,
  useDeleteHelixOrgRole,
  useDeleteHelixOrgStream,
  useFireHelixOrgWorker,
  useListHelixOrgRoles,
  useListHelixOrgStreams,
  useListHelixOrgWorkers,
  useAddWorkerParent,
  useRemoveWorkerParent,
  useSubscribeWorkerAtChart,
  useUnsubscribeWorkerAtChart,
} from '../services/helixOrgService'

// The chart visualises the org as a ReactFlow subflow. After Positions
// were removed from the domain, a Role groups Workers directly:
//
//   ┌─[Role: r-owner]──────────────────┐
//   │  [w-owner]                       │
//   └────────│───────────────────────────┘
//            ↓ (worker-to-worker reporting edge, from a reporting line)
//   ┌─[Role: r-engineer]───────────────────────────┐
//   │  [w-alice]  [w-bob]  [w-carol]               │
//   └───────────────────────────────────────────────┘
//
// Roles are parent group nodes that VISUALLY CONTAIN their Worker child
// nodes. A Role can hold many Workers. Reporting is a many-to-many
// relation: each (manager → report) reporting line becomes a Worker →
// Worker edge (a Worker may have several incoming edges). Streams hang
// off the right of the tree; an edge from a Worker to a Stream is a
// subscription.
//
// Layout: dagre runs over the role tree (edges derived from cross-role
// reporting lines) to get global (x, y) for each Role. Workers sit in a
// horizontal row inside their Role's frame.

const WORKER_W = 220
const WORKER_H = 96
const WORKER_GAP_X = 32
const WORKER_GAP_Y = 90
const ROLE_PAD_X = 24
const ROLE_PAD_TOP = 56
const ROLE_PAD_BOTTOM = 24

// ---- Flatten + group ---------------------------------------------------

type FlatWorker = {
  id: string
  kind: string
  roleId: string
  // Reporting is many-to-many: a Worker may report to several managers.
  parentIds: string[]
}

type RoleGroup = { roleId: string; workers: FlatWorker[] }

const groupByRole = (workers: FlatWorker[], knownRoles: string[]): RoleGroup[] => {
  const byRole = new Map<string, FlatWorker[]>()
  for (const r of knownRoles) {
    if (!byRole.has(r)) byRole.set(r, [])
  }
  for (const wk of workers) {
    const list = byRole.get(wk.roleId) ?? []
    list.push(wk)
    byRole.set(wk.roleId, list)
  }
  const out: RoleGroup[] = []
  byRole.forEach((ws, roleId) => {
    out.push({
      roleId,
      workers: ws.slice().sort((a, b) => a.id.localeCompare(b.id)),
    })
  })
  out.sort((a, b) => a.roleId.localeCompare(b.roleId))
  return out
}

// ---- Node renderers ----------------------------------------------------

type RoleNodeData = {
  roleId: string
  workerCount: number
  onSelectRole: (roleId: string) => void
  onHire: (roleId: string) => void
  onDeleteRole: (roleId: string) => void
}

type WorkerNodeData = {
  workerId: string
  kind: string
  onSelectWorker: (workerId: string) => void
  onFireWorker: (workerId: string) => void
}

// StreamNodeData drives the small pseudo-nodes the chart renders for
// each Stream beside the org tree. Edges from Workers to these nodes
// (subscriptions) are styled distinctly from the accountability edges
// between Workers.
type StreamNodeData = {
  streamId: string
  name: string
  kind: string
  subscriberCount: number
  onSelectStream: (streamId: string) => void
  onDeleteStream: (streamId: string) => void
}

// ReactFlow uses these CSS class names internally — children of a node
// that carry `nodrag` won't start a node-drag, and `nopan` won't pan
// the canvas. The combination is the documented way to make buttons,
// menus and form inputs inside custom nodes work correctly. See
// https://reactflow.dev/learn/customization/custom-nodes#interactive-children.
const NO_DRAG_NO_PAN = 'nodrag nopan'

// RoleNode is a parent group — ReactFlow renders the child Worker nodes
// inside its rect. The Box fills the node's frame and paints the header
// band along the top edge with the role id + the hire / delete-role
// affordances.
const RoleNode: FC<NodeProps<Node<RoleNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.6)' : 'rgba(255,255,255,0.6)'
  const titleColor = lightTheme.isLight ? 'rgba(0,0,0,0.85)' : 'rgba(255,255,255,0.9)'

  return (
    <Box sx={{ position: 'relative', width: '100%', height: '100%' }}>
      <Box
        className={NO_DRAG_NO_PAN}
        onClick={(e) => { e.stopPropagation(); data.onSelectRole(data.roleId) }}
        sx={{
          position: 'absolute',
          top: 0, left: 0, right: 0,
          height: ROLE_PAD_TOP - 8,
          px: 2,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          cursor: 'pointer',
          borderTopLeftRadius: 12,
          borderTopRightRadius: 12,
          '&:hover': {
            backgroundColor: lightTheme.isLight ? 'rgba(0,0,0,0.025)' : 'rgba(255,255,255,0.03)',
          },
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
            {data.workerCount} {data.workerCount === 1 ? 'worker' : 'workers'}
          </Typography>
        </Stack>
        <Stack direction="row" spacing={0.25}>
          <Tooltip title="Hire a worker into this role">
            <IconButton
              className={NO_DRAG_NO_PAN}
              size="small"
              onClick={(e) => { e.stopPropagation(); data.onHire(data.roleId) }}
              sx={{ color: muted }}
            >
              <PersonAddOutlinedIcon sx={{ fontSize: 18 }} />
            </IconButton>
          </Tooltip>
          {(
            <Tooltip title="Delete role (fires every Worker holding it)">
              <IconButton
                className={NO_DRAG_NO_PAN}
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
      {data.workerCount === 0 && (
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
            px: 2,
            textAlign: 'center',
          }}
        >
          No workers yet — click the hire icon to add one
        </Box>
      )}
    </Box>
  )
}

const WorkerNode: FC<NodeProps<Node<WorkerNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const border = lightTheme.isLight ? 'rgba(0,0,0,0.14)' : 'rgba(255,255,255,0.18)'
  const bg = lightTheme.isLight ? '#fff' : 'rgba(255,255,255,0.05)'
  const hoverBg = lightTheme.isLight ? 'rgba(0,0,0,0.02)' : 'rgba(255,255,255,0.08)'
  const handleColor = lightTheme.isLight ? 'rgba(0,0,0,0.35)' : 'rgba(255,255,255,0.35)'

  return (
    <Box
      className={NO_DRAG_NO_PAN}
      onClick={(e) => { e.stopPropagation(); data.onSelectWorker(data.workerId) }}
      sx={{
        width: WORKER_W,
        height: WORKER_H,
        border: `1px solid ${border}`,
        borderRadius: 1.5,
        backgroundColor: bg,
        boxShadow: lightTheme.isLight ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
        p: 1.5,
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
        cursor: 'pointer',
        '&:hover': { backgroundColor: hoverBg },
      }}
    >
      {/* Target handle = where a manager's edge LANDS, marking this
          worker as the subordinate. Source handle = where the user drags
          FROM when this worker becomes the manager. */}
      <Handle
        type="target"
        position={RFPosition.Top}
        style={{ background: handleColor, width: 12, height: 12 }}
      />
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start">
        <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
          {data.kind === 'ai' ? (
            <SmartToyOutlinedIcon sx={{ fontSize: 18, color: muted }} />
          ) : (
            <PersonOutlineIcon sx={{ fontSize: 18, color: muted }} />
          )}
          <Typography
            variant="body2"
            sx={{ fontFamily: 'monospace', fontSize: '0.85rem', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
          >
            {data.workerId}
          </Typography>
        </Stack>
        {(
          <Tooltip title="Fire worker">
            <IconButton
              className={NO_DRAG_NO_PAN}
              size="small"
              onClick={(e) => { e.stopPropagation(); data.onFireWorker(data.workerId) }}
              sx={{ p: 0.25, color: muted }}
            >
              <DeleteOutlineIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        )}
      </Stack>
      <Typography variant="caption" sx={{ color: muted, fontSize: '0.65rem', mt: 'auto' }}>
        {data.kind === 'ai' ? 'AI agent' : 'Human'}
      </Typography>
      <Handle
        type="source"
        position={RFPosition.Bottom}
        style={{ background: handleColor, width: 12, height: 12 }}
      />
      {/* Dedicated source handle for stream/subscription edges, anchored
          on the right side of the card. Decoupling stream edges from the
          bottom-center reporting handle means a subscription edge and a
          manager → subordinate edge can never share the same geometry.
          id="stream" is what buildGraph passes as sourceHandle when
          emitting subscription edges.

          Unlike the top/bottom reporting handles (which sit clear above
          and below the card), this one lands at the card's vertical
          centre — right where the name/caption Typography rows are. It
          must be large enough to grab and explicitly stacked above that
          content (zIndex), or the label intercepts the pointer and the
          subscription drag can't start. */}
      <Handle
        id="stream"
        type="source"
        position={RFPosition.Right}
        isConnectable
        style={{ background: 'rgba(180,100,0,0.85)', border: 'none', width: 14, height: 14, zIndex: 5 }}
      />
    </Box>
  )
}

// StreamNode is a small pseudo-node — narrower than a Worker card —
// rendered beside the org tree to anchor subscription edges. Clicking
// the body navigates to the per-stream detail page; the trash icon
// deletes the Stream row (irreversible).
const STREAM_W = 180
const STREAM_H = 80
const StreamNode: FC<NodeProps<Node<StreamNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const accent = lightTheme.isLight ? 'rgba(180,100,0,0.85)' : 'rgba(255,180,80,0.85)'
  const bg = 'rgba(255,180,80,0.06)'
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const handleColor = lightTheme.isLight ? 'rgba(180,100,0,0.55)' : 'rgba(255,180,80,0.55)'
  return (
    <Box
      onClick={(e) => { e.stopPropagation(); data.onSelectStream(data.streamId) }}
      sx={{
        width: STREAM_W,
        height: STREAM_H,
        border: `1px dashed ${accent}`,
        borderRadius: 1.5,
        backgroundColor: bg,
        p: 1,
        display: 'flex',
        flexDirection: 'column',
        gap: 0.25,
        cursor: 'pointer',
        position: 'relative',
        '&:hover': { backgroundColor: 'rgba(255,180,80,0.12)' },
      }}
    >
      <Handle type="target" position={RFPosition.Left} style={{ background: handleColor, width: 8, height: 8 }} />
      <Tooltip title="Delete stream">
        <IconButton
          className={NO_DRAG_NO_PAN}
          size="small"
          onClick={(e) => { e.stopPropagation(); data.onDeleteStream(data.streamId) }}
          sx={{ position: 'absolute', top: 2, right: 2, p: 0.25, color: muted }}
        >
          <DeleteOutlineIcon sx={{ fontSize: 14 }} />
        </IconButton>
      </Tooltip>
      <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: muted, pr: 2 }}>
        {data.streamId}
      </Typography>
      <Typography variant="body2" sx={{ fontSize: '0.8rem', fontWeight: 600, color: accent, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {data.name}
      </Typography>
      <Typography variant="caption" sx={{ fontSize: '0.65rem', color: muted, mt: 'auto' }}>
        {data.kind} · {data.subscriberCount} sub{data.subscriberCount === 1 ? '' : 's'}
      </Typography>
    </Box>
  )
}

const nodeTypes = { role: RoleNode, worker: WorkerNode, stream: StreamNode }

// ---- dagre layout ------------------------------------------------------

type StreamSummary = {
  id: string
  name: string
  kind: string
  created_by?: string
  subscribers?: string[]
}

// buildGraph computes nodes + edges for the chart. Roles are laid out by
// dagre over a role-level graph whose edges come from reporting lines
// that cross role boundaries. Workers sit in a horizontal row inside
// their role's frame. Worker → Worker reporting edges and Worker →
// Stream subscription edges are drawn on top.
// layoutStreamColumns positions worker-anchored stream pseudo-nodes to
// the right of the org tree without overlaps. Each stream prefers to sit
// at its subject Worker's y (so the subscription edge is short and
// roughly horizontal), but two streams may never occupy the same space.
//
// Algorithm:
//  1. Sort streams by their anchor y (then id, for a stable order).
//  2. Decide how many vertical columns are needed: a single column can
//     hold `floor((band + gap) / slot)` streams within the tree's
//     vertical extent. More streams than that spill into extra columns
//     to the right, so the column never grows unboundedly tall and
//     overruns the canvas.
//  3. Split the sorted list into balanced, contiguous column chunks
//     (contiguous in anchor-y, so each column owns a vertical band and
//     edges don't criss-cross between columns).
//  4. Within a column, place each stream at `max(anchorY, cursor)` and
//     advance the cursor past it — anchor-biased greedy packing, which
//     keeps streams beside their worker while guaranteeing no overlap.
const STREAM_VERTICAL_GAP = 16
const layoutStreamColumns = (
  items: { stream: StreamSummary; anchorY: number }[],
  opts: { columnX: number; columnGap: number; top: number; bottom: number },
): { stream: StreamSummary; x: number; y: number }[] => {
  if (items.length === 0) return []
  const sorted = items
    .slice()
    .sort((a, b) => a.anchorY - b.anchorY || a.stream.id.localeCompare(b.stream.id))

  const slot = STREAM_H + STREAM_VERTICAL_GAP
  // A column should at least span the tree's height, but never spill to a
  // second column until it holds a decent stack — otherwise a short tree
  // (e.g. one Worker) would fan a handful of streams across many columns.
  const MIN_PER_COLUMN = 6
  const band = Math.max(opts.bottom - opts.top, slot)
  const perColumn = Math.max(MIN_PER_COLUMN, Math.floor((band + STREAM_VERTICAL_GAP) / slot))
  const columnCount = Math.ceil(sorted.length / perColumn)
  // Re-balance so columns are evenly filled (e.g. 5 into 2 cols → 3 + 2,
  // not 4 + 1) — avoids a near-empty trailing column.
  const chunkSize = Math.ceil(sorted.length / columnCount)

  const out: { stream: StreamSummary; x: number; y: number }[] = []
  for (let col = 0; col < columnCount; col++) {
    const x = opts.columnX + col * (STREAM_W + opts.columnGap)
    let cursor = -Infinity
    const chunk = sorted.slice(col * chunkSize, (col + 1) * chunkSize)
    for (const it of chunk) {
      const y = Math.max(it.anchorY, cursor)
      out.push({ stream: it.stream, x, y })
      cursor = y + slot
    }
  }
  return out
}

const buildGraph = (
  groups: RoleGroup[],
  flat: FlatWorker[],
  handlers: {
    onSelectWorker: (workerId: string) => void
    onSelectRole: (roleId: string) => void
    onHire: (roleId: string) => void
    onDeleteRole: (roleId: string) => void
    onFireWorker: (workerId: string) => void
    onSelectStream: (streamId: string) => void
    onDeleteStream: (streamId: string) => void
  },
  isLight: boolean,
  streams: StreamSummary[],
): { nodes: Node[]; edges: Edge[] } => {
  const flatByID = new Map<string, FlatWorker>()
  for (const wk of flat) flatByID.set(wk.id, wk)

  const workerToRole = new Map<string, string>()
  for (const group of groups) {
    for (const wk of group.workers) workerToRole.set(wk.id, group.roleId)
  }

  // 1. Size each role frame from its worker count. Empty roles get a
  //    one-slot-wide placeholder so they're still discoverable.
  type Size = { w: number; h: number }
  const roleSize = new Map<string, Size>()
  for (const group of groups) {
    const n = Math.max(1, group.workers.length)
    roleSize.set(group.roleId, {
      w: n * WORKER_W + (n - 1) * WORKER_GAP_X + 2 * ROLE_PAD_X,
      h: WORKER_H + ROLE_PAD_TOP + ROLE_PAD_BOTTOM,
    })
  }

  // 2. Role-level dagre graph. Edges: any reporting line that crosses
  //    a role boundary contributes a role → role edge.
  const g = new dagre.graphlib.Graph()
  g.setGraph({
    rankdir: 'TB',
    nodesep: WORKER_GAP_X,
    ranksep: WORKER_GAP_Y,
    marginx: 0,
    marginy: 0,
  })
  g.setDefaultEdgeLabel(() => ({}))
  for (const group of groups) {
    const sz = roleSize.get(group.roleId)!
    g.setNode(`role:${group.roleId}`, { width: sz.w, height: sz.h })
  }
  const seenEdge = new Set<string>()
  for (const wk of flat) {
    for (const parentId of wk.parentIds) {
    if (!parentId || !flatByID.has(parentId)) continue
    const childRole = workerToRole.get(wk.id)
    const parentRole = workerToRole.get(parentId)
    if (!childRole || !parentRole || childRole === parentRole) continue
    const key = `${parentRole}->${childRole}`
    if (seenEdge.has(key)) continue
    seenEdge.add(key)
    g.setEdge(`role:${parentRole}`, `role:${childRole}`)
    }
  }
  dagre.layout(g)

  // 3. Emit nodes — role parents first, then their worker children.
  const nodes: Node[] = []
  const roleStyle = {
    backgroundColor: isLight ? 'rgba(0,0,0,0.025)' : 'rgba(255,255,255,0.03)',
    border: `1px solid ${isLight ? 'rgba(0,0,0,0.1)' : 'rgba(255,255,255,0.12)'}`,
    borderRadius: 12,
    boxShadow: isLight ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
  }
  type RoleOrigin = { x: number; y: number; w: number; h: number }
  const roleOrigin = new Map<string, RoleOrigin>()
  for (const group of groups) {
    const ln = g.node(`role:${group.roleId}`)
    const sz = roleSize.get(group.roleId)!
    if (!ln) continue
    roleOrigin.set(group.roleId, {
      x: ln.x - sz.w / 2,
      y: ln.y - sz.h / 2,
      w: sz.w,
      h: sz.h,
    })
  }

  for (const group of groups) {
    const ro = roleOrigin.get(group.roleId)
    if (!ro) continue
    nodes.push({
      id: `role:${group.roleId}`,
      type: 'role',
      position: { x: ro.x, y: ro.y },
      style: { ...roleStyle, width: ro.w, height: ro.h },
      data: {
        roleId: group.roleId,
        workerCount: group.workers.length,
        onSelectRole: handlers.onSelectRole,
        onHire: handlers.onHire,
        onDeleteRole: handlers.onDeleteRole,
      } as RoleNodeData,
      // selectable: true keeps the role's pointer-events on so the
      // header controls stay clickable; draggable is off (dagre owns
      // layout). The canvas-level elementsSelectable still applies.
      draggable: false,
      selectable: true,
    })
  }
  for (const group of groups) {
    const ro = roleOrigin.get(group.roleId)
    if (!ro) continue
    group.workers.forEach((wk, i) => {
      nodes.push({
        id: `worker:${wk.id}`,
        type: 'worker',
        position: {
          x: ro.x + ROLE_PAD_X + i * (WORKER_W + WORKER_GAP_X),
          y: ro.y + ROLE_PAD_TOP,
        },
        data: {
          workerId: wk.id,
          kind: wk.kind,
          onSelectWorker: handlers.onSelectWorker,
          onFireWorker: handlers.onFireWorker,
        } as WorkerNodeData,
        draggable: false,
        connectable: true,
      })
    })
  }

  // 4. Reporting edges: manager → subordinate, one per reporting line
  //    (a Worker may report to several). Bezier (the default) gives every pair its own
  //    arc so multiple reports from one manager never overlap.
  const edges: Edge[] = []
  for (const wk of flat) {
    for (const parentId of wk.parentIds) {
    if (!parentId || !flatByID.has(parentId)) continue
    edges.push({
      id: `report:${parentId}->${wk.id}`,
      source: `worker:${parentId}`,
      target: `worker:${wk.id}`,
      type: 'deletable',
      animated: false,
      data: { kind: 'report', childWorkerId: wk.id, parentWorkerId: parentId },
      style: {
        stroke: isLight ? 'rgba(0,0,0,0.3)' : 'rgba(255,255,255,0.35)',
        strokeWidth: 1.5,
      },
    })
    }
  }

  // 5. Stream pseudo-nodes + subscription edges. Subscriptions are
  //    worker-anchored, so subscribers carries Worker ids — one dashed
  //    edge per subscribed Worker. Streams sit in column(s) to the right
  //    of the org tree. Each stream is vertically anchored to the
  //    "subject" Worker: for transcripts (`s-transcript-<id>`)
  //    that's the encoded worker; otherwise created_by. Streams whose
  //    subject isn't on the chart park in an orphan strip below.
  //
  //    Layout engine (see `layoutStreamColumns`): the old code stacked
  //    each worker-row's streams independently in ONE shared column with
  //    no global collision check, so a tall stack from one row would
  //    overrun the stack of the row below (streams literally overlapped).
  //    The replacement is an anchor-biased, collision-free packer: it
  //    sorts streams by their subject's y, splits them into as many
  //    vertical columns as needed to fit the tree's height, then within
  //    each column places each stream at `max(anchorY, cursor)` so it
  //    stays beside its worker yet never overlaps the one above it.
  if (streams.length > 0) {
    const TRANSCRIPT_PREFIX = 's-transcript-'
    const workerAbs = new Map<string, { x: number; y: number }>()
    for (const group of groups) {
      const ro = roleOrigin.get(group.roleId)
      if (!ro) continue
      group.workers.forEach((wk, i) => {
        workerAbs.set(wk.id, {
          x: ro.x + ROLE_PAD_X + i * (WORKER_W + WORKER_GAP_X),
          y: ro.y + ROLE_PAD_TOP,
        })
      })
    }

    let minTop = Infinity, maxY = 0
    let minLeft = Infinity, maxRight = -Infinity
    for (const ro of roleOrigin.values()) {
      const bottom = ro.y + ro.h
      if (ro.y < minTop) minTop = ro.y
      if (bottom > maxY) maxY = bottom
      if (ro.x < minLeft) minLeft = ro.x
      if (ro.x + ro.w > maxRight) maxRight = ro.x + ro.w
    }
    if (!isFinite(minTop)) minTop = 0
    if (!isFinite(minLeft)) minLeft = 0
    if (!isFinite(maxRight)) maxRight = 0

    const STREAM_GAP_X = 32
    const STREAM_COLUMN_GAP = 120
    const ORPHAN_VERTICAL_GAP = 120

    const resolved: { stream: StreamSummary; subjectWorker: string | null }[] = []
    for (const s of streams) {
      let subjectWorker: string | undefined
      if (s.id.startsWith(TRANSCRIPT_PREFIX)) {
        subjectWorker = s.id.slice(TRANSCRIPT_PREFIX.length)
      } else if (s.created_by) {
        subjectWorker = s.created_by
      }
      const onChart = subjectWorker && workerAbs.has(subjectWorker) ? subjectWorker : null
      resolved.push({ stream: s, subjectWorker: onChart })
    }

    // Anchored streams: lay them out beside their subject Worker.
    const anchored = resolved.filter((r) => r.subjectWorker)
    const placed = layoutStreamColumns(
      anchored.map((r) => ({ stream: r.stream, anchorY: workerAbs.get(r.subjectWorker!)!.y })),
      { columnX: maxRight + STREAM_COLUMN_GAP, columnGap: STREAM_COLUMN_GAP, top: minTop, bottom: maxY },
    )
    const streamPos = new Map<string, { x: number; y: number }>()
    let streamsBottom = maxY
    for (const p of placed) {
      streamPos.set(p.stream.id, { x: p.x, y: p.y })
      if (p.y + STREAM_H > streamsBottom) streamsBottom = p.y + STREAM_H
    }

    // Orphans: a centred strip below everything else.
    const orphans = resolved.filter((r) => !r.subjectWorker)
    if (orphans.length > 0) {
      const stripWidth = orphans.length * STREAM_W + (orphans.length - 1) * STREAM_GAP_X
      let cursorX = (minLeft + maxRight) / 2 - stripWidth / 2
      const orphanY = streamsBottom + ORPHAN_VERTICAL_GAP
      for (const r of orphans) {
        streamPos.set(r.stream.id, { x: cursorX, y: orphanY })
        cursorX += STREAM_W + STREAM_GAP_X
      }
    }

    for (const { stream: s } of resolved) {
      const pos = streamPos.get(s.id)!
      const { x, y } = pos
      nodes.push({
        id: `stream:${s.id}`,
        type: 'stream',
        position: { x, y },
        data: {
          streamId: s.id,
          name: s.name,
          kind: s.kind,
          subscriberCount: s.subscribers?.length ?? 0,
          onSelectStream: handlers.onSelectStream,
          onDeleteStream: handlers.onDeleteStream,
        } as StreamNodeData,
        draggable: false,
        connectable: true,
        selectable: true,
      })
      const subscribingWorkers = (s.subscribers ?? []).filter((wid) => workerAbs.has(wid))
      for (const wid of subscribingWorkers) {
        edges.push({
          id: `sub:${wid}->${s.id}`,
          source: `worker:${wid}`,
          sourceHandle: 'stream',
          target: `stream:${s.id}`,
          type: 'deletable',
          animated: false,
          data: { kind: 'sub', workerId: wid, streamId: s.id },
          style: {
            stroke: isLight ? 'rgba(180,100,0,0.7)' : 'rgba(255,180,80,0.7)',
            strokeWidth: 1.25,
            strokeDasharray: '6 4',
          },
        })
      }
    }
  }

  return { nodes, edges }
}

// ---- Dialogs (Confirm delete) -----------------------------------------

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

// ---- Custom edge: deletable on hover -----------------------------------
//
// Wraps the default straight edge with a hover affordance: a small × button
// appears at the edge midpoint while the pointer is over the edge (or the
// button), and clicking it routes through ReactFlow's deleteElements API so
// the existing onEdgesDelete dispatch fires unchanged. A transparent wider
// stroke overlay widens the hover hit-area to ~20px so the 1.25–1.5px
// visible line is not the only target.
const DeletableEdge: FC<EdgeProps> = ({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  style,
  markerEnd,
  data,
  selected,
}) => {
  const [hover, setHover] = useState(false)
  const { deleteElements } = useReactFlow()
  const [edgePath, labelX, labelY] = getStraightPath({ sourceX, sourceY, targetX, targetY })
  const kind = (data as { kind?: string } | undefined)?.kind
  const ariaLabel = kind === 'sub' ? 'Remove subscription' : 'Remove reporting line'
  const show = hover || selected
  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} interactionWidth={20} />
      {/* invisible wider hit-area; must NOT inherit strokeDasharray or hover
          becomes spotty between dashes on subscription edges */}
      <path
        d={edgePath}
        fill="none"
        stroke="transparent"
        strokeWidth={20}
        strokeDasharray="none"
        style={{ cursor: 'pointer' }}
        onMouseEnter={() => setHover(true)}
        onMouseLeave={() => setHover(false)}
      />
      {show && (
        <EdgeLabelRenderer>
          <button
            type="button"
            aria-label={ariaLabel}
            title={ariaLabel}
            onMouseEnter={() => setHover(true)}
            onMouseLeave={() => setHover(false)}
            onClick={(e) => {
              e.stopPropagation()
              deleteElements({ edges: [{ id }] })
            }}
            style={{
              position: 'absolute',
              transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
              pointerEvents: 'all',
              width: 18,
              height: 18,
              borderRadius: '50%',
              border: '1px solid rgba(0,0,0,0.2)',
              background: '#ffffff',
              color: '#444',
              padding: 0,
              display: 'inline-flex',
              alignItems: 'center',
              justifyContent: 'center',
              cursor: 'pointer',
              boxShadow: '0 1px 2px rgba(0,0,0,0.15)',
              fontSize: 14,
              lineHeight: 1,
            }}
            onFocus={(e) => {
              e.currentTarget.style.outline = '2px solid #1976d2'
            }}
            onBlur={(e) => {
              e.currentTarget.style.outline = 'none'
            }}
            onMouseDown={(e) => e.stopPropagation()}
          >
            ×
          </button>
        </EdgeLabelRenderer>
      )}
    </>
  )
}

const edgeTypes = { deletable: DeletableEdge }

// ---- ReactFlow canvas --------------------------------------------------

const ChartCanvas: FC<{
  groups: RoleGroup[]
  flat: FlatWorker[]
  handlers: {
    onSelectWorker: (workerId: string) => void
    onSelectRole: (roleId: string) => void
    onHire: (roleId: string) => void
    onDeleteRole: (roleId: string) => void
    onFireWorker: (workerId: string) => void
    onSelectStream: (streamId: string) => void
    onDeleteStream: (streamId: string) => void
  }
  // onAddParent fires when the user wires manager → subordinate (an
  // onConnect); onRemoveParent fires when they delete a reporting edge,
  // and carries the specific manager since a Worker may have several.
  onAddParent: (childWorkerId: string, newParentWorkerId: string) => void
  onRemoveParent: (childWorkerId: string, parentWorkerId: string) => void
  // onSubscribeWorker fires when the user wires a Worker node → a stream
  // pseudo-node; onUnsubscribeWorker fires when they delete that edge.
  onSubscribeWorker: (workerId: string, streamId: string) => void
  onUnsubscribeWorker: (workerId: string, streamId: string) => void
  streams: StreamSummary[]
}> = ({ groups, flat, handlers, onAddParent, onRemoveParent, onSubscribeWorker, onUnsubscribeWorker, streams }) => {
  const lightTheme = useLightTheme()
  const { fitView } = useReactFlow()

  const { nodes: computedNodes, edges: computedEdges } = useMemo(
    () => buildGraph(groups, flat, handlers, lightTheme.isLight, streams),
    [groups, flat, handlers, lightTheme.isLight, streams],
  )
  const [nodes, setNodes, onNodesChange] = useNodesState(computedNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(computedEdges)

  useEffect(() => {
    setNodes(computedNodes)
    setEdges(computedEdges)
    requestAnimationFrame(() => fitView({ padding: 0.2, duration: 250 }))
  }, [computedNodes, computedEdges, fitView, setNodes, setEdges])

  // onConnect handles both wire shapes:
  //   - worker→worker: manager wires their report. Source = manager,
  //     target = subordinate. Persists by adding a reporting line.
  //   - worker→stream:  the worker consumes a stream. Persists by
  //     POSTing a (worker, stream) subscription.
  const onConnect = useCallback(
    ({ source, target }: { source: string | null; target: string | null }) => {
      if (!source || !target) return
      if (!source.startsWith('worker:')) return
      const sourceId = source.replace(/^worker:/, '')
      if (!sourceId) return
      if (target.startsWith('stream:')) {
        const streamId = target.replace(/^stream:/, '')
        if (!streamId) return
        onSubscribeWorker(sourceId, streamId)
        return
      }
      if (target.startsWith('worker:')) {
        const targetId = target.replace(/^worker:/, '')
        if (!targetId || sourceId === targetId) return
        onAddParent(targetId, sourceId)
      }
    },
    [onAddParent, onSubscribeWorker],
  )

  // onEdgesDelete severs whatever the edge represented: a reporting edge
  // drops that one (manager → report) line; a subscription edge drops
  // the (worker, stream) row.
  const onEdgesDelete = useCallback(
    (deleted: Edge[]) => {
      for (const e of deleted) {
        const d = e.data as { kind?: string; childWorkerId?: string; parentWorkerId?: string; workerId?: string; streamId?: string } | undefined
        if (d?.kind === 'sub' && d.workerId && d.streamId) {
          onUnsubscribeWorker(d.workerId, d.streamId)
          continue
        }
        // Reporting edge: remove the specific manager line. Fall back to
        // parsing "report:<parent>-><child>" from the edge id when data
        // is missing (e.g. an edge synthesised by ReactFlow).
        const childId = d?.childWorkerId ?? (e.target ?? '').replace(/^worker:/, '')
        const parentId = d?.parentWorkerId ?? (e.source ?? '').replace(/^worker:/, '')
        if (childId && parentId && (e.target ?? '').startsWith('worker:')) onRemoveParent(childId, parentId)
      }
    },
    [onRemoveParent, onUnsubscribeWorker],
  )

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      onEdgesDelete={onEdgesDelete}
      nodeTypes={nodeTypes}
      edgeTypes={edgeTypes}
      fitView
      fitViewOptions={{ padding: 0.2 }}
      proOptions={{ hideAttribution: true }}
      colorMode={lightTheme.isLight ? 'light' : 'dark'}
      nodesConnectable
      elementsSelectable
      // @xyflow/react v12's deleteKeyCode defaults to Backspace only, so
      // Linux/Windows users hitting Delete on a selected edge get
      // nothing. Accept both.
      deleteKeyCode={['Backspace', 'Delete']}
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
  | { kind: 'hire'; roleId: string }

const HelixOrgChart: FC = () => {
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const router = useRouter()
  const { data: workersData, isLoading } = useListHelixOrgWorkers()
  const { data: rolesData } = useListHelixOrgRoles()
  const { data: streamsData } = useListHelixOrgStreams()
  const deleteRole = useDeleteHelixOrgRole()
  const deleteStream = useDeleteHelixOrgStream()
  const fireWorker = useFireHelixOrgWorker()
  const addParent = useAddWorkerParent()
  const removeParent = useRemoveWorkerParent()
  const subscribe = useSubscribeWorkerAtChart()
  const unsubscribe = useUnsubscribeWorkerAtChart()

  const flat = useMemo<FlatWorker[]>(
    () => (workersData ?? []).map((w: WorkerDTO) => ({
      id: w.id ?? '',
      kind: w.kind ?? 'human',
      roleId: w.role_id ?? '',
      parentIds: w.parent_ids ?? [],
    })),
    [workersData],
  )
  const knownRoles = useMemo(() => (rolesData ?? []).map((r) => r.id ?? ''), [rolesData])
  const groups = useMemo(() => groupByRole(flat, knownRoles), [flat, knownRoles])
  const streams = useMemo<StreamSummary[]>(
    () => (streamsData?.streams ?? []).map((s) => ({
      id: s.id ?? '',
      name: s.name ?? '',
      kind: s.kind ?? '',
      created_by: s.created_by,
      subscribers: s.subscribers,
    })),
    [streamsData],
  )

  const [selection, setSelection] = useState<Selection>({ kind: 'none' })
  const [roleDialogOpen, setRoleDialogOpen] = useState(false)
  const [confirmDelete, setConfirmDelete] = useState<
    | { kind: 'role'; id: string }
    | { kind: 'worker'; id: string }
    | { kind: 'stream'; id: string }
    | null
  >(null)

  const titleColor = lightTheme.isLight ? 'rgba(0,0,0,0.87)' : 'rgba(255,255,255,0.95)'
  const subtitleColor = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const canvasBorder = lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'
  const canvasBg = lightTheme.isLight ? '#fafafa' : 'rgba(255,255,255,0.02)'

  const orgSlug = (router.params.org_id as string | undefined) ?? ''
  const onSelectWorker = useCallback(
    (workerId: string) => {
      if (!orgSlug) return
      router.navigate('helix_org_worker_detail', { org_id: orgSlug, worker_id: workerId })
    },
    [router, orgSlug],
  )
  const onSelectRole = useCallback(
    (roleId: string) => {
      if (!orgSlug) return
      router.navigate('helix_org_role_detail', { org_id: orgSlug, role_id: roleId })
    },
    [router, orgSlug],
  )
  const onHire = useCallback((roleId: string) => setSelection({ kind: 'hire', roleId }), [])
  const onDeleteRole = useCallback((roleId: string) => setConfirmDelete({ kind: 'role', id: roleId }), [])
  const onFireWorker = useCallback((workerId: string) => setConfirmDelete({ kind: 'worker', id: workerId }), [])
  const onSelectStream = useCallback(
    (streamId: string) => {
      if (!orgSlug) return
      router.navigate('helix_org_stream_detail', { org_id: orgSlug, stream_id: streamId })
    },
    [router, orgSlug],
  )
  const onDeleteStream = useCallback((streamId: string) => setConfirmDelete({ kind: 'stream', id: streamId }), [])
  const handlers = useMemo(
    () => ({ onSelectWorker, onSelectRole, onHire, onDeleteRole, onFireWorker, onSelectStream, onDeleteStream }),
    [onSelectWorker, onSelectRole, onHire, onDeleteRole, onFireWorker, onSelectStream, onDeleteStream],
  )

  const onAddParent = useCallback(
    async (childWorkerId: string, newParentWorkerId: string) => {
      try {
        await addParent.mutateAsync({ workerID: childWorkerId, parentID: newParentWorkerId })
        snackbar.success(`${childWorkerId} now reports to ${newParentWorkerId}`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'add reporting line failed')
      }
    },
    [addParent, snackbar],
  )

  const onRemoveParent = useCallback(
    async (childWorkerId: string, parentWorkerId: string) => {
      try {
        await removeParent.mutateAsync({ workerID: childWorkerId, parentID: parentWorkerId })
        snackbar.success(`${childWorkerId} no longer reports to ${parentWorkerId}`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'remove reporting line failed')
      }
    },
    [removeParent, snackbar],
  )

  const onSubscribeWorker = useCallback(
    async (workerId: string, streamId: string) => {
      try {
        await subscribe.mutateAsync({ workerID: workerId, streamID: streamId })
        snackbar.success(`${workerId} now consumes ${streamId}`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'subscribe failed')
      }
    },
    [subscribe, snackbar],
  )

  const onUnsubscribeWorker = useCallback(
    async (workerId: string, streamId: string) => {
      try {
        await unsubscribe.mutateAsync({ workerID: workerId, streamID: streamId })
        snackbar.success(`${workerId} no longer consumes ${streamId}`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'unsubscribe failed')
      }
    },
    [unsubscribe, snackbar],
  )

  const handleConfirmDelete = async () => {
    if (!confirmDelete) return
    try {
      if (confirmDelete.kind === 'role') {
        await deleteRole.mutateAsync(confirmDelete.id)
        snackbar.success(`deleted role ${confirmDelete.id}`)
      } else if (confirmDelete.kind === 'stream') {
        await deleteStream.mutateAsync(confirmDelete.id)
        snackbar.success(`deleted stream ${confirmDelete.id}`)
      } else {
        await fireWorker.mutateAsync(confirmDelete.id)
        snackbar.success(`fired worker ${confirmDelete.id}`)
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
      const workers = (group?.workers ?? []).map((w) => w.id)
      return [
        `Deleting role ${confirmDelete.id} will cascade:`,
        `  • fires ${workers.length} worker${workers.length === 1 ? '' : 's'} (${workers.join(', ') || 'none'})`,
        '',
        'This is irreversible.',
      ].join('\n')
    }
    if (confirmDelete.kind === 'stream') {
      const s = (streamsData?.streams ?? []).find((x) => x.id === confirmDelete.id)
      const subs = s?.subscribers ?? []
      return [
        `Deleting stream ${confirmDelete.id}:`,
        `  • removes the Stream row`,
        `  • drops ${subs.length} subscription${subs.length === 1 ? '' : 's'}${subs.length > 0 ? ' (' + subs.join(', ') + ')' : ''}`,
        `  • events on this stream survive as an audit trail`,
        '',
        'This is irreversible.',
      ].join('\n')
    }
    const reports = flat.filter((w) => w.parentIds.includes(confirmDelete.id)).map((w) => w.id)
    return [
      `Firing worker ${confirmDelete.id} will cascade:`,
      `  • stops sessions, deletes its project + agent app, drops its subscriptions`,
      reports.length > 0
        ? `  • ${reports.length} direct report${reports.length === 1 ? '' : 's'} (${reports.join(', ')}) lose their manager`
        : `  • no direct reports`,
      '',
      'This is irreversible.',
    ].join('\n')
  }, [confirmDelete, groups, flat, streamsData])

  return (
    <Page breadcrumbTitle="Chart">
      <Box sx={{ display: 'flex', flexDirection: 'column', height: 'calc(100vh - 64px)', minHeight: 0 }}>
        <Box sx={{ px: 4, pt: 4, pb: 2 }}>
          <Box>
            <Typography
              variant="h4"
              sx={{ fontWeight: 700, mb: 1, color: titleColor, letterSpacing: '-0.02em' }}
            >
              Chart
            </Typography>
            <Typography variant="body2" sx={{ color: subtitleColor }}>
              Roles group Workers. Hire Workers into a Role, then drag from a manager's
              bottom handle to a subordinate to set who reports to whom, or from a
              Worker's right handle to a Stream to subscribe.
            </Typography>
          </Box>
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
          {/* New role lives on the canvas (floating top-right) rather than
              in the page header — it reads as a canvas action, and keeps
              the header to title + description. zIndex sits above the
              ReactFlow surface / controls. */}
          <Button
            variant="contained"
            startIcon={<AddIcon />}
            onClick={() => setRoleDialogOpen(true)}
            sx={{ position: 'absolute', top: 12, right: 12, zIndex: 5 }}
          >
            New role
          </Button>

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
              <ChartCanvas
                groups={groups}
                flat={flat}
                handlers={handlers}
                onAddParent={onAddParent}
                onRemoveParent={onRemoveParent}
                onSubscribeWorker={onSubscribeWorker}
                onUnsubscribeWorker={onUnsubscribeWorker}
                streams={streams}
              />
            </ReactFlowProvider>
          )}
        </Box>
      </Box>

      <NewRoleDialog open={roleDialogOpen} onClose={() => setRoleDialogOpen(false)} />
      <ConfirmDeleteDialog
        open={confirmDelete !== null}
        title={
          confirmDelete?.kind === 'role' ? 'Delete role?' :
          confirmDelete?.kind === 'stream' ? 'Delete stream?' :
          'Fire worker?'
        }
        body={confirmBody}
        onConfirm={handleConfirmDelete}
        onClose={() => setConfirmDelete(null)}
        pending={deleteRole.isPending || deleteStream.isPending || fireWorker.isPending}
      />

      <HireWorkerDrawer
        open={selection.kind === 'hire'}
        onClose={() => setSelection({ kind: 'none' })}
        presetRoleId={selection.kind === 'hire' ? selection.roleId : undefined}
      />
    </Page>
  )
}

export default HelixOrgChart
