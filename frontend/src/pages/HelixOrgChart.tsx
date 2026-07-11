import { FC, Fragment, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Paper from '@mui/material/Paper'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import PersonAddOutlinedIcon from '@mui/icons-material/PersonAddOutlined'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import PlayArrowIcon from '@mui/icons-material/PlayArrow'
import RestartAltIcon from '@mui/icons-material/RestartAlt'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'
import StopIcon from '@mui/icons-material/Stop'
import TransformIcon from '@mui/icons-material/Transform'

import dagre from 'dagre'
import {
  Background,
  BaseEdge,
  Controls,
  Edge,
  EdgeLabelRenderer,
  EdgeProps,
  ConnectionLineType,
  getBezierPath,
  Handle,
  MarkerType,
  Node,
  NodeProps,
  ConnectionMode,
  Position as RFPosition,
  ReactFlow,
  ReactFlowProvider,
  Viewport,
  useEdgesState,
  useNodesState,
  useReactFlow,
} from '@xyflow/react'
import '@xyflow/react/dist/style.css'

import LoadingSpinner from '../components/widgets/LoadingSpinner'
import {
  loadChartViewport,
  saveChartViewport,
} from '../components/helix-org/chartViewportStorage'
import { focusChatBot } from '../components/helix-org/chatBotFocus'
import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import NewBotDialog from '../components/helix-org/NewBotDialog'
import ProcessorConfigDrawer from '../components/helix-org/ProcessorConfigDrawer'
import ProcessorNode, { ProcessorNodeData, PROC_W, procNodeHeight } from '../components/helix-org/ProcessorNode'
import useAccount from '../hooks/useAccount'
import useLightTheme from '../hooks/useLightTheme'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  BotDTO,
  ChartPositionMap,
  chartPositionKey,
  ProcessorDTO,
  useActivateBot,
  useClearChartPositions,
  useDeleteBot,
  useDeleteHelixOrgTopic,
  useListChartPositions,
  useListHelixOrgBots,
  useListHelixOrgTopics,
  useTopicMessageCounts,
  useListHelixOrgProcessors,
  useDeleteHelixOrgProcessor,
  useUpdateHelixOrgProcessor,
  useAddBotParent,
  useRemoveBotParent,
  useRestartBotAgent,
  useStopBotAgent,
  useSubscribeBotAtChart,
  useUnsubscribeBotAtChart,
  useUpsertChartPositions,
} from '../services/helixOrgService'

// The chart visualises the org as a ReactFlow graph. Bots are plain
// nodes wired by reporting edges:
//
//   [b-alice] ──reports to──▶ [b-owner]
//   [b-bob]   ──reports to──▶ [b-owner]
//
// Reporting is many-to-many: each (subordinate → manager) "reports to"
// line is a closest-side Bot → Bot edge with an arrow at the manager.
// Topics hang off the right of the tree; an edge from a Bot to a Topic is
// a subscription.
//
// Layout: dagre runs over the bot graph (edges = reporting lines) to get
// global (x, y) for each Bot node. Saved free-placed coordinates from
// GET /chart/positions override auto-layout per node; nodes without a
// saved row stay on the auto-layout position. Camera (pan/zoom) is
// personal — localStorage keyed by user id + org id, not shared.

const BOT_W = 220
const BOT_H = 96
const BOT_GAP_X = 32
const BOT_GAP_Y = 90
const STREAM_W = 180
const STREAM_H = 80

// ---- Flatten -----------------------------------------------------------

type FlatBot = {
  id: string
  // Human-readable display label; empty falls back to the id.
  name: string
  // Reporting is many-to-many: a Bot may report to several managers.
  parentIds: string[]
  // Desktop sandbox online-ness for the presence dot.
  agentStatus: 'running' | 'stopped'
}

// ---- Node renderers ----------------------------------------------------

type BotNodeData = {
  botId: string
  botName: string
  // running = desktop sandbox online; stopped (or missing) = offline.
  agentStatus: 'running' | 'stopped'
  /** Card body click — focus the left chat rail on this bot. */
  onSelectBot: (botId: string) => void
  /** ⋮ → Details — open the bot detail page. */
  onOpenBotDetails: (botId: string) => void
  onNewBot: (parentBotId: string) => void
  onDeleteBot: (botId: string) => void
  onStartBot: (botId: string) => void
  onStopBot: (botId: string) => void
  onRestartBot: (botId: string) => void
}

// TopicNodeData drives the small pseudo-nodes the chart renders for each
// Topic beside the org tree. Edges from Bots to these nodes
// (subscriptions) are styled distinctly from the reporting edges between
// Bots.
type TopicNodeData = {
  topicId: string
  name: string
  kind: string
  subscriberCount: number
  messageCount: number
  // When set, this topic is a processor's auto-provisioned output — it is
  // managed by that processor and must not be deleted independently
  // (delete the processor instead, which cascades it).
  ownedByProcessor?: string
  onSelectTopic: (topicId: string) => void
  onDeleteTopic: (topicId: string) => void
}

// ReactFlow uses these CSS class names internally — children of a node
// that carry `nodrag` won't start a node-drag, and `nopan` won't pan the
// canvas. The combination is the documented way to make buttons, menus
// and form inputs inside custom nodes work correctly.
const NO_DRAG_NO_PAN = 'nodrag nopan'

// ---- Closest-side geometry ---------------------------------------------
// Subscription (and similar free-form) edges should attach to whichever
// sides of the two cards are nearest, not a fixed right→left pair. The
// edge renderer recomputes this every frame from live node positions so
// it stays correct while cards are dragged.

type CardSide = 'left' | 'right' | 'top' | 'bottom'
type CardRect = { x: number; y: number; w: number; h: number }

const CARD_SIDES: CardSide[] = ['left', 'right', 'top', 'bottom']

const sideMidpoint = (r: CardRect, side: CardSide): { x: number; y: number } => {
  switch (side) {
    case 'left': return { x: r.x, y: r.y + r.h / 2 }
    case 'right': return { x: r.x + r.w, y: r.y + r.h / 2 }
    case 'top': return { x: r.x + r.w / 2, y: r.y }
    case 'bottom': return { x: r.x + r.w / 2, y: r.y + r.h }
  }
}

// Point just outside a card side. Used so arrowheads sit in the gap
// between nodes (nodes paint above edges and would otherwise clip them).
const sideOutward = (r: CardRect, side: CardSide, dist: number): { x: number; y: number } => {
  const p = sideMidpoint(r, side)
  switch (side) {
    case 'left': return { x: p.x - dist, y: p.y }
    case 'right': return { x: p.x + dist, y: p.y }
    case 'top': return { x: p.x, y: p.y - dist }
    case 'bottom': return { x: p.x, y: p.y + dist }
  }
}

// How far outside the target card to park the path end when an arrow
// marker is drawn — keeps the full head visible above the node z-order.
const ARROW_CLEARANCE_PX = 12

// Pick the (fromSide, toSide) pair whose midpoints are closest.
const closestSidePair = (a: CardRect, b: CardRect): { from: CardSide; to: CardSide } => {
  let bestFrom: CardSide = 'right'
  let bestTo: CardSide = 'left'
  let bestD = Infinity
  for (const from of CARD_SIDES) {
    const p1 = sideMidpoint(a, from)
    for (const to of CARD_SIDES) {
      const p2 = sideMidpoint(b, to)
      const dx = p1.x - p2.x
      const dy = p1.y - p2.y
      const d = dx * dx + dy * dy
      if (d < bestD) {
        bestD = d
        bestFrom = from
        bestTo = to
      }
    }
  }
  return { from: bestFrom, to: bestTo }
}

// Map a card side to the RF Position so bezier control points leave /
// enter perpendicular to that edge (rounder, more natural curves).
const sideToPosition = (side: CardSide): RFPosition => {
  switch (side) {
    case 'left': return RFPosition.Left
    case 'right': return RFPosition.Right
    case 'top': return RFPosition.Top
    case 'bottom': return RFPosition.Bottom
  }
}

const nodeCardRect = (n: Node, fallbackW: number, fallbackH: number): CardRect => {
  const w = (n.measured?.width ?? n.width ?? fallbackW) as number
  const h = (n.measured?.height ?? n.height ?? fallbackH) as number
  return { x: n.position.x, y: n.position.y, w, h }
}

const fallbackSizeForNode = (n: Node): { w: number; h: number } => {
  if (n.type === 'topic') return { w: STREAM_W, h: STREAM_H }
  if (n.type === 'processor') {
    const outs = (n.data as ProcessorNodeData | undefined)?.outputs?.length ?? 1
    return { w: PROC_W, h: procNodeHeight(outs) }
  }
  return { w: BOT_W, h: BOT_H }
}

// Orange handles on all four sides so the user can drag a subscription
// wire from/to any side. Outward (source) + inward (target) share a side
// so ConnectionMode.Loose can start and end a connection on either end.
// Reporting lines keep the default (id-less) top target / bottom source.
const SubSideHandles: FC<{ color: string; size?: number }> = ({ color, size = 12 }) => (
  <Fragment>
    {([
      [RFPosition.Left, 'left'],
      [RFPosition.Right, 'right'],
      [RFPosition.Top, 'top'],
      [RFPosition.Bottom, 'bottom'],
    ] as const).map(([pos, side]) => (
      <Fragment key={side}>
        <Handle
          id={`sub-${side}`}
          type="source"
          position={pos}
          isConnectable
          style={{ background: color, border: 'none', width: size, height: size, zIndex: 5 }}
        />
        <Handle
          id={`sub-${side}-in`}
          type="target"
          position={pos}
          isConnectable
          style={{ background: color, border: 'none', opacity: 0, width: size + 4, height: size + 4, zIndex: 4 }}
        />
      </Fragment>
    ))}
  </Fragment>
)

const BotNode: FC<NodeProps<Node<BotNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const border = lightTheme.isLight ? 'rgba(0,0,0,0.14)' : 'rgba(255,255,255,0.18)'
  const bg = lightTheme.isLight ? '#fff' : 'rgba(255,255,255,0.05)'
  const hoverBg = lightTheme.isLight ? 'rgba(0,0,0,0.02)' : 'rgba(255,255,255,0.08)'
  const handleColor = lightTheme.isLight ? 'rgba(0,0,0,0.35)' : 'rgba(255,255,255,0.35)'
  const subColor = 'rgba(180,100,0,0.85)'
  const [menuEl, setMenuEl] = useState<null | HTMLElement>(null)

  const online = data.agentStatus === 'running'
  const statusColor = online ? 'rgb(46, 160, 67)' : (lightTheme.isLight ? 'rgba(0,0,0,0.28)' : 'rgba(255,255,255,0.28)')
  const statusLabel = online ? 'Agent sandbox online' : 'Agent sandbox stopped'

  const closeMenu = () => setMenuEl(null)

  return (
    <Box
      onClick={(e) => { e.stopPropagation(); data.onSelectBot(data.botId) }}
      sx={{
        width: BOT_W,
        height: BOT_H,
        border: `1px solid ${border}`,
        borderRadius: 1.5,
        backgroundColor: bg,
        boxShadow: lightTheme.isLight ? '0 1px 2px rgba(0,0,0,0.04)' : 'none',
        p: 1.5,
        display: 'flex',
        flexDirection: 'column',
        gap: 1,
        cursor: 'grab',
        position: 'relative',
        '&:hover': { backgroundColor: hoverBg },
        '&:active': { cursor: 'grabbing' },
      }}
    >
      {/* Reporting: top = land as subordinate, bottom = drag as manager.
          Subscriptions use the orange sub-* handles on all four sides. */}
      <Handle
        type="target"
        position={RFPosition.Top}
        style={{ background: handleColor, width: 12, height: 12 }}
      />
      <SubSideHandles color={subColor} size={14} />
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={0.5}>
        <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0, flex: 1 }}>
          <SmartToyOutlinedIcon sx={{ fontSize: 18, color: muted, flexShrink: 0 }} />
          <Typography
            variant="body2"
            sx={{ fontSize: '0.85rem', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
          >
            {data.botName || data.botId}
          </Typography>
        </Stack>
        {/* Top-right: status dot + ⋮ menu (same pattern as table/card lists). */}
        <Stack
          direction="row"
          alignItems="center"
          spacing={0.25}
          className={NO_DRAG_NO_PAN}
          sx={{ flexShrink: 0, mt: -0.25, mr: -0.5 }}
          onClick={(e) => e.stopPropagation()}
        >
          <Tooltip title={statusLabel}>
            <Box
              sx={{
                width: 9,
                height: 9,
                borderRadius: '50%',
                backgroundColor: statusColor,
                boxShadow: online ? `0 0 0 2px ${lightTheme.isLight ? 'rgba(46,160,67,0.2)' : 'rgba(46,160,67,0.35)'}` : 'none',
                flexShrink: 0,
              }}
            />
          </Tooltip>
          <IconButton
            className={NO_DRAG_NO_PAN}
            size="small"
            aria-label="Bot actions"
            onClick={(e) => {
              e.stopPropagation()
              setMenuEl(e.currentTarget)
            }}
            sx={{ p: 0.25, color: muted }}
          >
            <MoreVertIcon sx={{ fontSize: 16 }} />
          </IconButton>
          <Menu
            className={NO_DRAG_NO_PAN}
            anchorEl={menuEl}
            open={Boolean(menuEl)}
            onClose={closeMenu}
            onClick={(e) => e.stopPropagation()}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            transformOrigin={{ vertical: 'top', horizontal: 'right' }}
          >
            <MenuItem
              onClick={() => {
                closeMenu()
                data.onOpenBotDetails(data.botId)
              }}
            >
              <OpenInNewIcon sx={{ mr: 1, fontSize: 20 }} />
              Details
            </MenuItem>
            {online ? (
              <>
                <MenuItem
                  onClick={() => {
                    closeMenu()
                    data.onStopBot(data.botId)
                  }}
                >
                  <StopIcon sx={{ mr: 1, fontSize: 20 }} />
                  Stop agent
                </MenuItem>
                <MenuItem
                  onClick={() => {
                    closeMenu()
                    data.onRestartBot(data.botId)
                  }}
                >
                  <RestartAltIcon sx={{ mr: 1, fontSize: 20 }} />
                  Restart agent
                </MenuItem>
              </>
            ) : (
              <MenuItem
                onClick={() => {
                  closeMenu()
                  data.onStartBot(data.botId)
                }}
              >
                <PlayArrowIcon sx={{ mr: 1, fontSize: 20 }} />
                Start agent
              </MenuItem>
            )}
            <MenuItem
              onClick={() => {
                closeMenu()
                data.onNewBot(data.botId)
              }}
            >
              <PersonAddOutlinedIcon sx={{ mr: 1, fontSize: 20 }} />
              New bot reporting here
            </MenuItem>
            <MenuItem
              onClick={() => {
                closeMenu()
                data.onDeleteBot(data.botId)
              }}
            >
              <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
              Delete bot
            </MenuItem>
          </Menu>
        </Stack>
      </Stack>
      <Typography variant="caption" sx={{ color: muted, fontSize: '0.65rem', mt: 'auto' }}>
        Bot
      </Typography>
      <Handle
        type="source"
        position={RFPosition.Bottom}
        style={{ background: handleColor, width: 12, height: 12 }}
      />
    </Box>
  )
}

// TopicNode is a small pseudo-node — narrower than a Bot card — rendered
// beside the org tree to anchor subscription edges. Clicking the body
// navigates to the per-topic detail page; the trash icon deletes the
// Topic row (irreversible).
const TopicNode: FC<NodeProps<Node<TopicNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const accent = lightTheme.isLight ? 'rgba(180,100,0,0.85)' : 'rgba(255,180,80,0.85)'
  const bg = 'rgba(255,180,80,0.06)'
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const [menuEl, setMenuEl] = useState<null | HTMLElement>(null)
  const closeMenu = () => setMenuEl(null)
  return (
    <Box
      onClick={(e) => { e.stopPropagation(); data.onSelectTopic(data.topicId) }}
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
        cursor: 'grab',
        position: 'relative',
        '&:hover': { backgroundColor: 'rgba(255,180,80,0.12)' },
        '&:active': { cursor: 'grabbing' },
      }}
    >
      {/* All four sides: drag to/from a bot to subscribe, or to a
          processor IN port (right side also carries id "src" for the
          legacy processor-wiring path). */}
      <SubSideHandles color={accent} size={10} />
      <Handle id="src" type="source" position={RFPosition.Right} isConnectable style={{ background: accent, width: 10, height: 10, zIndex: 6 }} />
      {data.ownedByProcessor ? (
        <Tooltip title={`Output of processor ${data.ownedByProcessor} — delete the processor to remove this topic`}>
          <Box sx={{ position: 'absolute', top: 2, right: 4, fontSize: '0.6rem', color: muted, fontFamily: 'monospace' }}>
            ⟜ {data.ownedByProcessor}
          </Box>
        </Tooltip>
      ) : (
        <Box
          className={NO_DRAG_NO_PAN}
          sx={{ position: 'absolute', top: 0, right: 0, zIndex: 2 }}
          onClick={(e) => e.stopPropagation()}
        >
          <IconButton
            className={NO_DRAG_NO_PAN}
            size="small"
            aria-label="Topic actions"
            onClick={(e) => {
              e.stopPropagation()
              setMenuEl(e.currentTarget)
            }}
            sx={{ p: 0.25, color: muted }}
          >
            <MoreVertIcon sx={{ fontSize: 14 }} />
          </IconButton>
          <Menu
            className={NO_DRAG_NO_PAN}
            anchorEl={menuEl}
            open={Boolean(menuEl)}
            onClose={closeMenu}
            onClick={(e) => e.stopPropagation()}
            anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
            transformOrigin={{ vertical: 'top', horizontal: 'right' }}
          >
            <MenuItem
              onClick={() => {
                closeMenu()
                data.onDeleteTopic(data.topicId)
              }}
            >
              <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
              Delete topic
            </MenuItem>
          </Menu>
        </Box>
      )}
      <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: muted, pr: 2.5 }}>
        {data.topicId}
      </Typography>
      <Typography variant="body2" sx={{ fontSize: '0.8rem', fontWeight: 600, color: accent, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
        {data.name}
      </Typography>
      <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mt: 'auto' }}>
        <Typography variant="caption" sx={{ fontSize: '0.65rem', color: muted }}>
          {data.kind} · {data.subscriberCount} sub{data.subscriberCount === 1 ? '' : 's'}
        </Typography>
        {/* Waiting-message count. Kept deliberately tiny — the card is
            already dense — and tinted with the topic accent so it reads as
            a topic stat rather than chrome. */}
        <Tooltip title={`${data.messageCount} message${data.messageCount === 1 ? '' : 's'} waiting`}>
          <Typography
            variant="caption"
            sx={{ fontSize: '0.65rem', fontFamily: 'monospace', fontWeight: 700, color: accent, lineHeight: 1 }}
          >
            {data.messageCount} msg
          </Typography>
        </Tooltip>
      </Stack>
    </Box>
  )
}

const nodeTypes = { bot: BotNode, topic: TopicNode, processor: ProcessorNode }

// ---- dagre layout ------------------------------------------------------

type TopicSummary = {
  id: string
  name: string
  kind: string
  created_by?: string
  subscribers?: string[]
  // Set to the owning processor id when this topic is that processor's
  // auto-provisioned output (managed; not independently deletable).
  ownedByProcessor?: string
}

type ProcessorSummary = {
  id: string
  name: string
  kind: string
  inputTopicId: string
  outputs: { topicId: string; label: string; match: string; owned: boolean }[]
}

// layoutTopicColumns positions bot-anchored topic pseudo-nodes to the
// right of the org tree without overlaps. Each topic prefers to sit at its
// subject Bot's y (so the subscription edge is short and roughly
// horizontal), but two topics may never occupy the same space.
//
// Algorithm:
//  1. Sort topics by their anchor y (then id, for a stable order).
//  2. Decide how many vertical columns are needed: a single column can
//     hold `floor((band + gap) / slot)` topics within the tree's vertical
//     extent. More topics than that spill into extra columns to the right.
//  3. Split the sorted list into balanced, contiguous column chunks.
//  4. Within a column, place each topic at `max(anchorY, cursor)` and
//     advance the cursor past it — anchor-biased greedy packing.
const STREAM_VERTICAL_GAP = 16
const layoutTopicColumns = (
  items: { topic: TopicSummary; anchorY: number }[],
  opts: { columnX: number; columnGap: number; top: number; bottom: number },
): { topic: TopicSummary; x: number; y: number }[] => {
  if (items.length === 0) return []
  const sorted = items
    .slice()
    .sort((a, b) => a.anchorY - b.anchorY || a.topic.id.localeCompare(b.topic.id))

  const slot = STREAM_H + STREAM_VERTICAL_GAP
  const MIN_PER_COLUMN = 6
  const band = Math.max(opts.bottom - opts.top, slot)
  const perColumn = Math.max(MIN_PER_COLUMN, Math.floor((band + STREAM_VERTICAL_GAP) / slot))
  const columnCount = Math.ceil(sorted.length / perColumn)
  const chunkSize = Math.ceil(sorted.length / columnCount)

  const out: { topic: TopicSummary; x: number; y: number }[] = []
  for (let col = 0; col < columnCount; col++) {
    const x = opts.columnX + col * (STREAM_W + opts.columnGap)
    let cursor = -Infinity
    const chunk = sorted.slice(col * chunkSize, (col + 1) * chunkSize)
    for (const it of chunk) {
      const y = Math.max(it.anchorY, cursor)
      out.push({ topic: it.topic, x, y })
      cursor = y + slot
    }
  }
  return out
}

const buildGraph = (
  flat: FlatBot[],
  handlers: {
    onSelectBot: (botId: string) => void
    onOpenBotDetails: (botId: string) => void
    onNewBot: (parentBotId: string) => void
    onDeleteBot: (botId: string) => void
    onStartBot: (botId: string) => void
    onStopBot: (botId: string) => void
    onRestartBot: (botId: string) => void
    onSelectTopic: (topicId: string) => void
    onDeleteTopic: (topicId: string) => void
    onSelectProcessor: (processorId: string) => void
    onDeleteProcessor: (processorId: string) => void
  },
  isLight: boolean,
  topics: TopicSummary[],
  messageCounts: Record<string, number>,
  processors: ProcessorSummary[],
  // Saved free-placed coordinates keyed by `${kind}:${id}`. Missing
  // entries keep the auto-layout position for that node.
  savedPositions: ChartPositionMap = {},
): { nodes: Node[]; edges: Edge[] } => {
  const place = (kind: string, id: string, auto: { x: number; y: number }) =>
    savedPositions[chartPositionKey(kind, id)] ?? auto
  const flatByID = new Map<string, FlatBot>()
  for (const b of flat) flatByID.set(b.id, b)

  // 1. Bot-level dagre graph. Edges: each reporting line is a
  //    parent → child edge.
  const g = new dagre.graphlib.Graph()
  g.setGraph({
    rankdir: 'TB',
    nodesep: BOT_GAP_X,
    ranksep: BOT_GAP_Y,
    marginx: 0,
    marginy: 0,
  })
  g.setDefaultEdgeLabel(() => ({}))
  for (const b of flat) {
    g.setNode(`bot:${b.id}`, { width: BOT_W, height: BOT_H })
  }
  const seenEdge = new Set<string>()
  for (const b of flat) {
    for (const parentId of b.parentIds) {
      if (!parentId || !flatByID.has(parentId)) continue
      const key = `${parentId}->${b.id}`
      if (seenEdge.has(key)) continue
      seenEdge.add(key)
      g.setEdge(`bot:${parentId}`, `bot:${b.id}`)
    }
  }
  dagre.layout(g)

  // 2. Emit bot nodes.
  // botAutoAbs = pure dagre coords (used to auto-place topics so free-
  // placing a bot does NOT reflow unpinned yellow topic cards).
  // botAbs = rendered coords (saved override or auto) for bounds / edges.
  const nodes: Node[] = []
  const botAbs = new Map<string, { x: number; y: number }>()
  const botAutoAbs = new Map<string, { x: number; y: number }>()
  for (const b of flat) {
    const ln = g.node(`bot:${b.id}`)
    if (!ln) continue
    const auto = { x: ln.x - BOT_W / 2, y: ln.y - BOT_H / 2 }
    botAutoAbs.set(b.id, auto)
    const pos = place('bot', b.id, auto)
    botAbs.set(b.id, pos)
    nodes.push({
      id: `bot:${b.id}`,
      type: 'bot',
      position: pos,
      data: {
        botId: b.id,
        botName: b.name,
        agentStatus: b.agentStatus,
        onSelectBot: handlers.onSelectBot,
        onOpenBotDetails: handlers.onOpenBotDetails,
        onNewBot: handlers.onNewBot,
        onDeleteBot: handlers.onDeleteBot,
        onStartBot: handlers.onStartBot,
        onStopBot: handlers.onStopBot,
        onRestartBot: handlers.onRestartBot,
      } as BotNodeData,
      draggable: true,
      connectable: true,
    })
  }

  // 3. Reporting edges: subordinate → manager ("reports to"), one per
  //    reporting line. Closest-side bezier so free-placed cards attach
  //    from the nearest sides; arrow points at the manager.
  const edges: Edge[] = []
  const reportStroke = isLight ? 'rgba(0,0,0,0.35)' : 'rgba(255,255,255,0.4)'
  for (const b of flat) {
    for (const parentId of b.parentIds) {
      if (!parentId || !flatByID.has(parentId)) continue
      edges.push({
        id: `report:${parentId}->${b.id}`,
        // Source = who reports, target = who they report to (arrow end).
        source: `bot:${b.id}`,
        target: `bot:${parentId}`,
        type: 'closest',
        animated: false,
        data: {
          kind: 'report',
          childBotId: b.id,
          parentBotId: parentId,
          label: 'reports to',
        },
        style: {
          stroke: reportStroke,
          strokeWidth: 1.5,
        },
        markerEnd: {
          type: MarkerType.ArrowClosed,
          width: 24,
          height: 24,
          color: reportStroke,
        },
      })
    }
  }

  // 4. Topic pseudo-nodes + subscription edges. Subscriptions are
  //    bot-anchored, so subscribers carries Bot ids — one dashed edge per
  //    subscribed Bot. Topics sit in column(s) to the right of the org
  //    tree. Each topic is vertically anchored to the "subject" Bot: for
  //    transcripts (`s-transcript-<id>`) that's the encoded bot;
  //    otherwise created_by. Topics whose subject isn't on the chart park
  //    in an orphan strip below.
  //
  //    Processor-owned output topics are collapsed into their processor
  //    node (rendered as labelled branch ports), so they are not drawn as
  //    their own Topic boxes. We still need their subscriber lists below
  //    to draw the branch → Bot edges, so they stay in `topics` (just not
  //    rendered).
  const ownedOutputTopicIds = new Set<string>()
  // branchOwner maps a (collapsed) output-topic id → the processor that
  // produces it, so a downstream processor reading that topic can be wired
  // straight from the upstream branch port (chaining).
  const branchOwner = new Map<string, string>()
  for (const p of processors) for (const o of p.outputs) {
    if (o.owned && o.topicId) ownedOutputTopicIds.add(o.topicId)
    if (o.topicId) branchOwner.set(o.topicId, p.id)
  }

  // Bounds for the topic-column auto-layout use *dagre* bot positions,
  // not free-placed ones — otherwise dragging a bot would slide every
  // still-auto-laid topic by the same delta (the bug users hit).
  let maxRight = -Infinity
  let minTop = Infinity, maxBottom = -Infinity, minLeft = Infinity
  for (const pos of botAutoAbs.values()) {
    if (pos.x + BOT_W > maxRight) maxRight = pos.x + BOT_W
    if (pos.x < minLeft) minLeft = pos.x
    if (pos.y < minTop) minTop = pos.y
    if (pos.y + BOT_H > maxBottom) maxBottom = pos.y + BOT_H
  }
  if (!isFinite(maxRight)) maxRight = 0
  if (!isFinite(minLeft)) minLeft = 0
  if (!isFinite(minTop)) minTop = 0
  if (!isFinite(maxBottom)) maxBottom = 0

  if (topics.length > 0) {
    const TRANSCRIPT_PREFIX = 's-transcript-'
    const STREAM_GAP_X = 32
    const STREAM_COLUMN_GAP = 120
    const ORPHAN_VERTICAL_GAP = 120

    const resolved: { topic: TopicSummary; subjectBot: string | null }[] = []
    for (const s of topics) {
      if (ownedOutputTopicIds.has(s.id)) continue // collapsed into its processor's branch ports
      let subjectBot: string | undefined
      if (s.id.startsWith(TRANSCRIPT_PREFIX)) {
        subjectBot = s.id.slice(TRANSCRIPT_PREFIX.length)
      } else if (s.created_by) {
        subjectBot = s.created_by
      }
      const onChart = subjectBot && botAutoAbs.has(subjectBot) ? subjectBot : null
      resolved.push({ topic: s, subjectBot: onChart })
    }

    // Anchored topics: auto-layout beside the *dagre* y of the subject
    // bot (not its free-placed y). Free-placing a bot must not reflow
    // unpinned topics.
    const anchored = resolved.filter((r) => r.subjectBot)
    const placed = layoutTopicColumns(
      anchored.map((r) => ({ topic: r.topic, anchorY: botAutoAbs.get(r.subjectBot!)!.y })),
      { columnX: maxRight + STREAM_COLUMN_GAP, columnGap: STREAM_COLUMN_GAP, top: minTop, bottom: maxBottom },
    )
    const topicPos = new Map<string, { x: number; y: number }>()
    let streamsBottom = maxBottom
    for (const p of placed) {
      topicPos.set(p.topic.id, { x: p.x, y: p.y })
      if (p.y + STREAM_H > streamsBottom) streamsBottom = p.y + STREAM_H
    }

    // Orphans: a centred strip below everything else.
    const orphans = resolved.filter((r) => !r.subjectBot)
    if (orphans.length > 0) {
      const stripWidth = orphans.length * STREAM_W + (orphans.length - 1) * STREAM_GAP_X
      let cursorX = (minLeft + maxRight) / 2 - stripWidth / 2
      const orphanY = streamsBottom + ORPHAN_VERTICAL_GAP
      for (const r of orphans) {
        topicPos.set(r.topic.id, { x: cursorX, y: orphanY })
        cursorX += STREAM_W + STREAM_GAP_X
      }
    }

    for (const { topic: s } of resolved) {
      const auto = topicPos.get(s.id)!
      const pos = place('topic', s.id, auto)
      // Keep topicPosById (used for processor placement) in sync with
      // the rendered position when a topic has been free-placed.
      topicPos.set(s.id, pos)
      nodes.push({
        id: `topic:${s.id}`,
        type: 'topic',
        position: pos,
        data: {
          topicId: s.id,
          name: s.name,
          kind: s.kind,
          subscriberCount: s.subscribers?.length ?? 0,
          messageCount: messageCounts[s.id] ?? 0,
          ownedByProcessor: s.ownedByProcessor,
          onSelectTopic: handlers.onSelectTopic,
          onDeleteTopic: handlers.onDeleteTopic,
        } as TopicNodeData,
        draggable: true,
        connectable: true,
        selectable: true,
      })
      const subscribingBots = (s.subscribers ?? []).filter((bid) => botAbs.has(bid))
      for (const bid of subscribingBots) {
        // type 'closest' draws between the nearest sides of the two
        // cards; handles are omitted so the edge path is free of the
        // fixed right→left bias.
        edges.push({
          id: `sub:${bid}->${s.id}`,
          source: `bot:${bid}`,
          target: `topic:${s.id}`,
          type: 'closest',
          animated: false,
          data: { kind: 'sub', botId: bid, topicId: s.id },
          style: {
            stroke: isLight ? 'rgba(180,100,0,0.7)' : 'rgba(255,180,80,0.7)',
            strokeWidth: 1.25,
            strokeDasharray: '6 4',
          },
        })
      }
    }
  }

  // ---- Processors -------------------------------------------------------
  // A processor sits just right of the topic column. It draws an input
  // edge from its input Topic, and one edge per output BRANCH from that
  // branch's labelled port to each Bot subscribed to the branch's
  // (collapsed) output topic. Wiring a branch to a Bot is a drag from the
  // branch port → the Bot.
  if (processors.length > 0) {
    const topicNodeIds = new Set<string>()
    const topicPosById = new Map<string, { x: number; y: number }>()
    for (const n of nodes) {
      if (n.id.startsWith('topic:')) {
        const tid = n.id.slice('topic:'.length)
        topicNodeIds.add(tid)
        topicPosById.set(tid, n.position as { x: number; y: number })
      }
    }
    // Subscribers per topic (incl. the collapsed output topics) so we can
    // draw branch → Bot edges.
    const botSet = new Set<string>()
    for (const b of flat) botSet.add(b.id)
    const subsByTopic = new Map<string, string[]>()
    for (const tp of topics) subsByTopic.set(tp.id, (tp.subscribers ?? []).filter((b) => botSet.has(b)))

    const PROC_COL_X = maxRight + 120 + STREAM_W + 80
    const procStroke = isLight ? 'rgba(90,60,170,0.7)' : 'rgba(180,150,255,0.7)'

    // Vertical collision avoidance, accounting for each node's height
    // (which grows with the branch count).
    const used: { y: number; h: number }[] = []
    const placeY = (preferred: number, h: number): number => {
      let y = preferred
      for (let guard = 0; guard < 200; guard++) {
        const clash = used.find((u) => y < u.y + u.h + 24 && y + h + 24 > u.y)
        if (clash === undefined) break
        y = clash.y + clash.h + 24
      }
      used.push({ y, h })
      return y
    }

    for (const p of processors) {
      const inPos = p.inputTopicId ? topicPosById.get(p.inputTopicId) : undefined
      const h = procNodeHeight(p.outputs.length)
      const saved = savedPositions[chartPositionKey('processor', p.id)]
      let pos: { x: number; y: number }
      if (saved) {
        // Free-placed: keep the saved coords as-is. Reserve the vertical
        // band without clash-shifting (placeY would move a free-placed
        // node if a sibling already occupied that y).
        pos = saved
        used.push({ y: saved.y, h })
      } else {
        pos = { x: PROC_COL_X, y: placeY(inPos ? inPos.y : minTop, h) }
      }
      nodes.push({
        id: `processor:${p.id}`,
        type: 'processor',
        position: pos,
        data: {
          processorId: p.id,
          name: p.name,
          kind: p.kind,
          outputs: p.outputs.map((o) => ({ topicId: o.topicId, label: o.label, match: o.match })),
          onSelectProcessor: handlers.onSelectProcessor,
          onDeleteProcessor: handlers.onDeleteProcessor,
          onInspectBranch: handlers.onSelectTopic,
        } as ProcessorNodeData,
        draggable: true,
        connectable: true,
        selectable: true,
      })

      if (p.inputTopicId && topicNodeIds.has(p.inputTopicId)) {
        edges.push({
          id: `procin:${p.inputTopicId}->${p.id}`,
          source: `topic:${p.inputTopicId}`,
          sourceHandle: 'src',
          target: `processor:${p.id}`,
          type: 'deletable',
          data: { kind: 'proc_in', processorId: p.id },
          style: { stroke: procStroke, strokeWidth: 1.5 },
        })
      } else if (p.inputTopicId && branchOwner.has(p.inputTopicId) && branchOwner.get(p.inputTopicId) !== p.id) {
        // Chained: this processor reads an upstream processor's output
        // branch — draw the edge from that branch port to this IN port.
        const upstream = branchOwner.get(p.inputTopicId)!
        edges.push({
          id: `procchain:${upstream}:${p.inputTopicId}->${p.id}`,
          source: `processor:${upstream}`,
          sourceHandle: p.inputTopicId,
          target: `processor:${p.id}`,
          type: 'deletable',
          data: { kind: 'proc_in', processorId: p.id },
          style: { stroke: procStroke, strokeWidth: 1.5 },
        })
      }
      // Each branch port → every Bot subscribed to that branch's output
      // topic. The edge leaves the branch's own handle (sourceHandle = the
      // branch topic id) and lands on the Bot's right-side DATA handle
      // (id "topic") — the same side a Bot uses to subscribe to topics.
      for (const o of p.outputs) {
        if (!o.topicId) continue
        for (const bid of subsByTopic.get(o.topicId) ?? []) {
          edges.push({
            id: `procout:${p.id}:${o.topicId}->${bid}`,
            source: `processor:${p.id}`,
            sourceHandle: o.topicId,
            target: `bot:${bid}`,
            // Closest-side path between branch and bot; sourceHandle still
            // names the branch port so the edge leaves the right port.
            type: 'closest',
            data: { kind: 'proc_out', processorId: p.id, topicId: o.topicId, botId: bid },
            style: { stroke: procStroke, strokeWidth: 1.25, strokeDasharray: '6 4' },
          })
        }
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

// ---- Custom edges ------------------------------------------------------
//
// DeletableEdge: path between the RF-supplied handle endpoints, with a
// hover × that routes through deleteElements → onEdgesDelete. Used for
// processor input wires (fixed ports).
//
// ClosestSideEdge: same chrome, but endpoints are recomputed from the
// live node rects as the nearest side-midpoint pair. Used for bot↔bot
// reporting lines, bot↔topic subscriptions, and branch→bot wires so
// free-placed cards don't force a fixed-side cable that crosses cards.

const EdgeDeleteButton: FC<{
  id: string
  labelX: number
  labelY: number
  ariaLabel: string
  show: boolean
  onHover: (v: boolean) => void
}> = ({ id, labelX, labelY, ariaLabel, show, onHover }) => {
  const { deleteElements } = useReactFlow()
  if (!show) return null
  return (
    <EdgeLabelRenderer>
      <button
        type="button"
        aria-label={ariaLabel}
        title={ariaLabel}
        onMouseEnter={() => onHover(true)}
        onMouseLeave={() => onHover(false)}
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
          zIndex: 1,
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
  )
}

// Mid-edge caption (e.g. "reports to"). Hidden while the delete control
// is shown so the two don't stack on the same point.
const EdgeCaption: FC<{
  labelX: number
  labelY: number
  text: string
  show: boolean
}> = ({ labelX, labelY, text, show }) => {
  const lightTheme = useLightTheme()
  if (!show || !text) return null
  const isLight = lightTheme.isLight
  return (
    <EdgeLabelRenderer>
      <div
        style={{
          position: 'absolute',
          transform: `translate(-50%, -50%) translate(${labelX}px, ${labelY}px)`,
          pointerEvents: 'none',
          fontSize: 11,
          fontWeight: 600,
          letterSpacing: '0.01em',
          color: isLight ? 'rgba(0,0,0,0.65)' : 'rgba(255,255,255,0.8)',
          background: isLight ? 'rgba(255,255,255,0.92)' : 'rgba(30,30,30,0.92)',
          border: isLight ? '1px solid rgba(0,0,0,0.08)' : '1px solid rgba(255,255,255,0.12)',
          borderRadius: 4,
          padding: '1px 6px',
          whiteSpace: 'nowrap',
          boxShadow: isLight ? '0 1px 2px rgba(0,0,0,0.06)' : 'none',
          lineHeight: 1.4,
        }}
        className="nodrag nopan"
      >
        {text}
      </div>
    </EdgeLabelRenderer>
  )
}

const edgeAriaLabel = (kind?: string) =>
  kind === 'sub' || kind === 'proc_out' ? 'Remove subscription'
    : kind === 'proc_in' ? 'Disconnect input'
      : 'Remove reporting line'

const DeletableEdge: FC<EdgeProps> = ({
  id,
  sourceX,
  sourceY,
  targetX,
  targetY,
  sourcePosition,
  targetPosition,
  style,
  markerEnd,
  data,
  selected,
}) => {
  const [hover, setHover] = useState(false)
  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX,
    sourceY,
    targetX,
    targetY,
    sourcePosition,
    targetPosition,
  })
  const kind = (data as { kind?: string } | undefined)?.kind
  const show = hover || selected
  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} interactionWidth={20} />
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
      <EdgeDeleteButton
        id={id}
        labelX={labelX}
        labelY={labelY}
        ariaLabel={edgeAriaLabel(kind)}
        show={show}
        onHover={setHover}
      />
    </>
  )
}

// Subscription-style edge: attach to the closest sides of the two cards.
// Re-reads node positions from the store so the path updates live while
// either end is dragged (handle-based endpoints would stick to a fixed side).
// Uses a bezier so the cable leaves/enters perpendicular to each side.
const ClosestSideEdge: FC<EdgeProps> = ({
  id,
  source,
  target,
  sourceX,
  sourceY,
  sourcePosition,
  style,
  markerEnd,
  data,
  selected,
  sourceHandleId,
}) => {
  const [hover, setHover] = useState(false)
  const { getNode } = useReactFlow()
  const sourceNode = getNode(source)
  const targetNode = getNode(target)
  const edgeData = data as { kind?: string; label?: string } | undefined
  const kind = edgeData?.kind
  const caption = edgeData?.label

  let sx = sourceX
  let sy = sourceY
  let tx = sourceX
  let ty = sourceY
  let sPos = sourcePosition ?? RFPosition.Right
  let tPos = RFPosition.Left

  if (sourceNode && targetNode) {
    const sf = fallbackSizeForNode(sourceNode)
    const tf = fallbackSizeForNode(targetNode)
    const sRect = nodeCardRect(sourceNode, sf.w, sf.h)
    const tRect = nodeCardRect(targetNode, tf.w, tf.h)

    // Processor branch ports: leave the edge at the source handle RF
    // already resolved (the labelled branch port), only free the target
    // side to the closest bot side. Pure bot↔bot / bot↔topic edges free
    // both ends.
    //
    // When the edge has an arrow (reporting lines), park the target end
    // slightly outside the card so the marker isn't clipped under the
    // node layer.
    const hasArrow = Boolean(markerEnd)
    if (kind === 'proc_out' && sourceHandleId) {
      const { to } = closestSidePair(sRect, tRect)
      const p2 = hasArrow ? sideOutward(tRect, to, ARROW_CLEARANCE_PX) : sideMidpoint(tRect, to)
      sx = sourceX
      sy = sourceY
      sPos = sourcePosition ?? RFPosition.Right
      tx = p2.x
      ty = p2.y
      tPos = sideToPosition(to)
    } else {
      const { from, to } = closestSidePair(sRect, tRect)
      const p1 = sideMidpoint(sRect, from)
      const p2 = hasArrow ? sideOutward(tRect, to, ARROW_CLEARANCE_PX) : sideMidpoint(tRect, to)
      sx = p1.x
      sy = p1.y
      sPos = sideToPosition(from)
      tx = p2.x
      ty = p2.y
      tPos = sideToPosition(to)
    }
  }

  const [edgePath, labelX, labelY] = getBezierPath({
    sourceX: sx,
    sourceY: sy,
    targetX: tx,
    targetY: ty,
    sourcePosition: sPos,
    targetPosition: tPos,
  })
  const showDelete = hover || selected
  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} interactionWidth={20} />
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
      <EdgeCaption
        labelX={labelX}
        labelY={labelY}
        text={caption ?? ''}
        show={!showDelete}
      />
      <EdgeDeleteButton
        id={id}
        labelX={labelX}
        labelY={labelY}
        ariaLabel={edgeAriaLabel(kind)}
        show={showDelete}
        onHover={setHover}
      />
    </>
  )
}

const edgeTypes = { deletable: DeletableEdge, closest: ClosestSideEdge }

// ---- ReactFlow canvas --------------------------------------------------

const ChartCanvas: FC<{
  flat: FlatBot[]
  handlers: {
    onSelectBot: (botId: string) => void
    onOpenBotDetails: (botId: string) => void
    onNewBot: (parentBotId: string) => void
    onDeleteBot: (botId: string) => void
    onStartBot: (botId: string) => void
    onStopBot: (botId: string) => void
    onRestartBot: (botId: string) => void
    onSelectTopic: (topicId: string) => void
    onDeleteTopic: (topicId: string) => void
    onSelectProcessor: (processorId: string) => void
    onDeleteProcessor: (processorId: string) => void
  }
  // onAddParent fires when the user wires manager → subordinate (an
  // onConnect); onRemoveParent fires when they delete a reporting edge,
  // and carries the specific manager since a Bot may have several.
  onAddParent: (childBotId: string, newParentBotId: string) => void
  onRemoveParent: (childBotId: string, parentBotId: string) => void
  // onSubscribeBot fires when the user wires a Bot node → a topic
  // pseudo-node; onUnsubscribeBot fires when they delete that edge.
  onSubscribeBot: (botId: string, topicId: string) => void
  onUnsubscribeBot: (botId: string, topicId: string) => void
  // onSetProcessorInput fires when the user wires a Topic (or another
  // processor's output branch) into a processor's IN port.
  onSetProcessorInput: (processorId: string, topicId: string) => void
  // onLayoutSnapshot fires after the user finishes dragging a node with
  // the FULL set of node positions currently on the canvas. Saving only
  // the dragged node lets unpinned topics re-auto-layout and "follow"
  // the bot; pinning everything freezes the layout.
  onLayoutSnapshot: (positions: { kind: string; id: string; x: number; y: number }[]) => void
  topics: TopicSummary[]
  messageCounts: Record<string, number>
  processors: ProcessorSummary[]
  savedPositions: ChartPositionMap
}> = ({ flat, handlers, onAddParent, onRemoveParent, onSubscribeBot, onUnsubscribeBot, onSetProcessorInput, onLayoutSnapshot, topics, messageCounts, processors, savedPositions }) => {
  const lightTheme = useLightTheme()
  const account = useAccount()
  const userId = account.user?.id ?? ''
  // Canonical org id (not the URL slug) so a rename doesn't lose the camera.
  const orgId = account.organizationTools.organization?.id ?? ''
  const { fitView, setViewport } = useReactFlow()
  // Apply camera once after the first graph build: restore this user's
  // saved pan/zoom for the org, or fitView when nothing is stored yet.
  // Node-drag persistence must not re-run this (would yank the camera).
  const didInitViewportRef = useRef(false)
  // Track which user+org the init applied to so a mid-session org switch
  // re-loads that org's camera.
  const viewportScopeRef = useRef('')

  const { nodes: computedNodes, edges: computedEdges } = useMemo(
    () => buildGraph(flat, handlers, lightTheme.isLight, topics, messageCounts, processors, savedPositions),
    [flat, handlers, lightTheme.isLight, topics, messageCounts, processors, savedPositions],
  )
  const [nodes, setNodes, onNodesChange] = useNodesState(computedNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(computedEdges)

  useEffect(() => {
    setNodes(computedNodes)
    setEdges(computedEdges)
    const scope = userId && orgId ? `${userId}:${orgId}` : ''
    if (scope && scope !== viewportScopeRef.current) {
      viewportScopeRef.current = scope
      didInitViewportRef.current = false
    }
    if (didInitViewportRef.current || computedNodes.length === 0 || !userId || !orgId) return
    didInitViewportRef.current = true
    const saved = loadChartViewport(userId, orgId)
    requestAnimationFrame(() => {
      if (saved) {
        setViewport(saved, { duration: 0 })
      } else {
        fitView({ padding: 0.2, duration: 250 })
      }
    })
  }, [computedNodes, computedEdges, fitView, setViewport, setNodes, setEdges, userId, orgId])

  // Personal camera: pan/zoom only — node layout is server-side shared.
  const onMoveEnd = useCallback(
    (_event: MouseEvent | TouchEvent | null, viewport: Viewport) => {
      if (!userId || !orgId) return
      saveChartViewport(userId, orgId, viewport)
    },
    [userId, orgId],
  )

  const onNodeDragStop = useCallback(
    (_: React.MouseEvent, dragged: Node, allNodes?: Node[]) => {
      // Prefer the nodes array ReactFlow passes (includes the final
      // drag position); fall back to local state with the dragged node
      // patched in.
      const source = allNodes && allNodes.length > 0
        ? allNodes
        : nodes.map((n) => (n.id === dragged.id ? { ...n, position: dragged.position } : n))
      const positions: { kind: string; id: string; x: number; y: number }[] = []
      for (const n of source) {
        const colon = n.id.indexOf(':')
        if (colon <= 0) continue
        const kind = n.id.slice(0, colon)
        const id = n.id.slice(colon + 1)
        if (!id || (kind !== 'bot' && kind !== 'topic' && kind !== 'processor')) continue
        positions.push({ kind, id, x: n.position.x, y: n.position.y })
      }
      if (positions.length === 0) return
      onLayoutSnapshot(positions)
    },
    [nodes, onLayoutSnapshot],
  )

  // onConnect handles wire shapes:
  //   - bot→bot:     drag manager → report creates a "reports to" line
  //                  (stored as subordinate reports_to manager; drawn
  //                  subordinate → manager with arrow + label)
  //   - bot→topic OR topic→bot: subscribe (either direction)
  //   - topic→processor / processor-branch→…: processor wiring
  const onConnect = useCallback(
    ({ source, sourceHandle, target }: { source: string | null; sourceHandle?: string | null; target: string | null }) => {
      if (!source || !target) return

      // Processor OUT branch → (Bot | Processor). The branch handle id IS
      // the branch's output topic id (see buildGraph), so the wire carries
      // which branch was dragged.
      if (source.startsWith('processor:') && sourceHandle && sourceHandle.startsWith('s-')) {
        const branchTopicId = sourceHandle
        if (target.startsWith('bot:')) {
          const botId = target.replace(/^bot:/, '')
          if (botId) onSubscribeBot(botId, branchTopicId)
        } else if (target.startsWith('processor:')) {
          // Chain: the downstream processor reads this branch's output.
          const procId = target.replace(/^processor:/, '')
          if (procId) onSetProcessorInput(procId, branchTopicId)
        }
        return
      }

      // Topic → Processor IN: that processor now reads this topic.
      if (source.startsWith('topic:') && target.startsWith('processor:')) {
        const topicId = source.replace(/^topic:/, '')
        const procId = target.replace(/^processor:/, '')
        if (topicId && procId) onSetProcessorInput(procId, topicId)
        return
      }

      // Topic → Bot: subscribe (drag from a topic onto a bot).
      if (source.startsWith('topic:') && target.startsWith('bot:')) {
        const topicId = source.replace(/^topic:/, '')
        const botId = target.replace(/^bot:/, '')
        if (topicId && botId) onSubscribeBot(botId, topicId)
        return
      }

      // Bot → Topic: subscribe.
      if (source.startsWith('bot:') && target.startsWith('topic:')) {
        const botId = source.replace(/^bot:/, '')
        const topicId = target.replace(/^topic:/, '')
        if (botId && topicId) onSubscribeBot(botId, topicId)
        return
      }

      // Bot → Bot: reporting line (manager → subordinate).
      if (source.startsWith('bot:') && target.startsWith('bot:')) {
        const managerId = source.replace(/^bot:/, '')
        const reportId = target.replace(/^bot:/, '')
        if (!managerId || !reportId || managerId === reportId) return
        onAddParent(reportId, managerId)
      }
    },
    [onAddParent, onSubscribeBot, onSetProcessorInput],
  )

  // onEdgesDelete severs whatever the edge represented: a reporting edge
  // drops that one (manager → report) line; a subscription edge drops the
  // (bot, topic) row.
  const onEdgesDelete = useCallback(
    (deleted: Edge[]) => {
      for (const e of deleted) {
        const d = e.data as { kind?: string; childBotId?: string; parentBotId?: string; botId?: string; topicId?: string; processorId?: string } | undefined
        // Deleting a processor's input edge disconnects it: clear the
        // input topic, leaving the processor inert until it's re-wired.
        if (d?.kind === 'proc_in' && d.processorId) {
          onSetProcessorInput(d.processorId, '')
          continue
        }
        // A branch → Bot edge IS a subscription to the branch's output
        // topic; deleting it unsubscribes the Bot from that branch.
        if (d?.kind === 'proc_out' && d.botId && d.topicId) {
          onUnsubscribeBot(d.botId, d.topicId)
          continue
        }
        if (d?.kind === 'sub' && d.botId && d.topicId) {
          onUnsubscribeBot(d.botId, d.topicId)
          continue
        }
        // Reporting edge (subordinate → manager, "reports to"). Prefer
        // data; fall back to id "report:<parent>-><child>".
        let childId = d?.childBotId
        let parentId = d?.parentBotId
        if ((!childId || !parentId) && typeof e.id === 'string' && e.id.startsWith('report:')) {
          const rest = e.id.slice('report:'.length)
          const arrow = rest.indexOf('->')
          if (arrow > 0) {
            parentId = parentId || rest.slice(0, arrow)
            childId = childId || rest.slice(arrow + 2)
          }
        }
        if (childId && parentId) onRemoveParent(childId, parentId)
      }
    },
    [onRemoveParent, onUnsubscribeBot, onSetProcessorInput],
  )

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      onNodesChange={onNodesChange}
      onEdgesChange={onEdgesChange}
      onConnect={onConnect}
      onEdgesDelete={onEdgesDelete}
      onNodeDragStop={onNodeDragStop}
      onMoveEnd={onMoveEnd}
      nodeTypes={nodeTypes}
      edgeTypes={edgeTypes}
      // Snap a dropped connection to the nearest handle within this radius,
      // so wiring into a bot / processor port doesn't require pixel-perfect
      // aim.
      connectionRadius={55}
      // Loose mode lets a connection END on any handle regardless of
      // source/target type. Needed because a Bot's only target handle is on
      // top, but a processor's output approaches from the right (a source
      // handle) — in strict mode that drop is rejected and the wire
      // silently fails. onConnect validates which combos are real.
      connectionMode={ConnectionMode.Loose}
      // Match persisted edges: curved while the user is still dragging a wire.
      connectionLineType={ConnectionLineType.Bezier}
      // Camera is restored from localStorage (or fitView) in the init effect —
      // do not fitView on every mount prop, or it fights the saved viewport.
      fitViewOptions={{ padding: 0.2 }}
      proOptions={{ hideAttribution: true }}
      colorMode={lightTheme.isLight ? 'light' : 'dark'}
      nodesDraggable
      nodesConnectable
      elementsSelectable
      // @xyflow/react v12's deleteKeyCode defaults to Backspace only, so
      // Linux/Windows users hitting Delete on a selected edge get nothing.
      // Accept both.
      deleteKeyCode={['Backspace', 'Delete']}
      panOnDrag
      zoomOnScroll
    >
      <Background gap={20} size={1} />
      <Controls showInteractive={false} position="top-left" />
    </ReactFlow>
  )
}

// ---- Page --------------------------------------------------------------

type Selection =
  | { kind: 'none' }
  | { kind: 'newBot'; parentBotId: string }

// PeoplePanel is the docked list of the org's people, pinned to the bottom-
// right of the chart canvas. People (kind=human) are NOT graph nodes — the
// chart is for agents — but they're shown here alongside the agents with the
// contact channels the org reaches them on plus their responsibility. Click
// a person to open their profile.
const PeoplePanel: FC<{ people: BotDTO[]; onSelect: (botId: string) => void }> = ({ people, onSelect }) => {
  const lightTheme = useLightTheme()
  if (people.length === 0) return null
  const bg = lightTheme.isLight ? 'rgba(255,255,255,0.96)' : 'rgba(28,28,32,0.96)'
  const border = lightTheme.isLight ? 'rgba(0,0,0,0.12)' : 'rgba(255,255,255,0.14)'
  const hover = lightTheme.isLight ? 'rgba(0,0,0,0.04)' : 'rgba(255,255,255,0.06)'
  return (
    <Paper
      elevation={0}
      sx={{
        position: 'absolute', bottom: 12, right: 12, zIndex: 5,
        width: 300, maxHeight: '48%', display: 'flex', flexDirection: 'column',
        border: `1px solid ${border}`, borderRadius: 1.5, backgroundColor: bg,
        backdropFilter: 'blur(4px)',
      }}
    >
      <Box sx={{ px: 1.5, py: 1, borderBottom: `1px solid ${border}` }}>
        <Stack direction="row" alignItems="center" spacing={0.75}>
          <PersonOutlineIcon sx={{ fontSize: 16, color: 'rgba(60,140,210,0.9)' }} />
          <Typography variant="caption" sx={{ fontWeight: 700, textTransform: 'uppercase', letterSpacing: '0.05em' }}>
            People
          </Typography>
        </Stack>
      </Box>
      <Box sx={{ p: 0.5, overflowY: 'auto' }}>
        {people.map((p) => {
          const channels = Object.entries(p.identity ?? {}).filter(([, v]) => !!v)
          const responsibility = (p.content || '').split('\n').find((l) => l.trim() !== '')?.trim()
          return (
            <Box
              key={p.id}
              className="nodrag nopan"
              onClick={() => onSelect(p.id ?? '')}
              sx={{ px: 1, py: 0.75, borderRadius: 1, cursor: 'pointer', '&:hover': { backgroundColor: hover } }}
            >
              <Typography variant="body2" sx={{ fontWeight: 600 }}>{p.name || p.id}</Typography>
              {channels.length > 0 && (
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', fontFamily: 'monospace', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {channels.map(([k, v]) => `${k}: ${v}`).join('  ·  ')}
                </Typography>
              )}
              {responsibility && (
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', whiteSpace: 'nowrap', overflow: 'hidden', textOverflow: 'ellipsis' }}>
                  {responsibility}
                </Typography>
              )}
            </Box>
          )
        })}
      </Box>
    </Paper>
  )
}

const HelixOrgChart: FC = () => {
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const router = useRouter()
  // Poll bots so agent_status (green/grey sandbox dots) stays fresh while
  // the chart is open — desktops start/stop without other chart mutations.
  const { data: botsData, isLoading } = useListHelixOrgBots({ refetchInterval: 5000 })
  const { data: streamsData } = useListHelixOrgTopics()
  const { data: processorsData } = useListHelixOrgProcessors()
  const { data: savedPositions = {} } = useListChartPositions()
  const upsertPositions = useUpsertChartPositions()
  const clearPositions = useClearChartPositions()
  const deleteBot = useDeleteBot()
  const deleteTopic = useDeleteHelixOrgTopic()
  const deleteProcessor = useDeleteHelixOrgProcessor()
  const updateProcessor = useUpdateHelixOrgProcessor()
  const addParent = useAddBotParent()
  const removeParent = useRemoveBotParent()
  const subscribe = useSubscribeBotAtChart()
  const unsubscribe = useUnsubscribeBotAtChart()
  const activateBot = useActivateBot()
  const stopBot = useStopBotAgent()
  const restartBot = useRestartBotAgent()

  const flat = useMemo<FlatBot[]>(
    () => (botsData ?? [])
      // People (kind=human) are managed in the People tab, not on the agent
      // chart — the chart is for agent relationships (reporting, subscriptions).
      .filter((b: BotDTO) => b.kind !== 'human')
      .map((b: BotDTO) => ({
        id: b.id ?? '',
        name: b.name ?? '',
        parentIds: b.parent_ids ?? [],
        agentStatus: b.agent_status === 'running' ? 'running' as const : 'stopped' as const,
      })),
    [botsData],
  )

  // People (kind=human) — shown in the docked PeoplePanel on the chart, not
  // as graph nodes.
  const people = useMemo<BotDTO[]>(
    () => (botsData ?? []).filter((b: BotDTO) => b.kind === 'human'),
    [botsData],
  )

  // Map each processor-owned output topic id → owning processor id, so
  // those topics render as managed (no independent delete).
  const ownedOutputTopics = useMemo(() => {
    const m = new Map<string, string>()
    for (const p of processorsData ?? []) {
      for (const o of p.outputs ?? []) {
        if (o.owned && o.topic_id) m.set(o.topic_id, p.id)
      }
    }
    return m
  }, [processorsData])

  const topics = useMemo<TopicSummary[]>(
    () => (streamsData?.topics ?? []).map((s) => ({
      id: s.id ?? '',
      name: s.name ?? '',
      kind: s.kind ?? '',
      created_by: s.created_by,
      subscribers: s.subscribers,
      ownedByProcessor: ownedOutputTopics.get(s.id ?? ''),
    })),
    [streamsData, ownedOutputTopics],
  )

  // Per-topic waiting-message counts for the topic cards. One cached query
  // per topic id (shared with the detail page's count hook), so each
  // card's number refreshes independently. topicIds is memoized so the
  // fan-out only re-subscribes when the set of topics changes, not on
  // every render.
  const topicIds = useMemo(() => topics.map((s) => s.id), [topics])
  const messageCounts = useTopicMessageCounts(topicIds)

  const processorSummaries = useMemo<ProcessorSummary[]>(
    () => (processorsData ?? []).map((p: ProcessorDTO) => ({
      id: p.id,
      name: p.name ?? p.id,
      kind: p.kind ?? '',
      inputTopicId: p.input_topic_id ?? '',
      outputs: (p.outputs ?? []).map((o) => ({
        topicId: o.topic_id ?? '',
        label: o.label ?? '',
        match: o.match ?? '',
        owned: !!o.owned,
      })),
    })),
    [processorsData],
  )

  const [selection, setSelection] = useState<Selection>({ kind: 'none' })
  const [botDialogOpen, setBotDialogOpen] = useState(false)
  // Processor drawer: { open, processor } — processor null = create mode.
  const [processorDrawer, setProcessorDrawer] = useState<{ open: boolean; processor: ProcessorDTO | null }>({ open: false, processor: null })
  const [confirmDelete, setConfirmDelete] = useState<
    | { kind: 'bot'; id: string }
    | { kind: 'topic'; id: string }
    | { kind: 'processor'; id: string }
    | null
  >(null)

  const titleColor = lightTheme.isLight ? 'rgba(0,0,0,0.87)' : 'rgba(255,255,255,0.95)'
  const subtitleColor = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const canvasBorder = lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'
  const canvasBg = lightTheme.isLight ? '#fafafa' : 'rgba(255,255,255,0.02)'

  const orgSlug = (router.params.org_id as string | undefined) ?? ''
  // Card body click → focus left chat rail on this bot (stay on chart).
  const onSelectBot = useCallback(
    (botId: string) => {
      if (!orgSlug) return
      focusChatBot(orgSlug, botId)
    },
    [orgSlug],
  )
  // ⋮ → Details → bot detail page.
  const onOpenBotDetails = useCallback(
    (botId: string) => {
      if (!orgSlug) return
      router.navigate('helix_org_bot_detail', { org_id: orgSlug, bot_id: botId })
    },
    [router, orgSlug],
  )
  const onSelectPerson = useCallback(
    (botId: string) => {
      if (!orgSlug) return
      router.navigate('helix_org_human_detail', { org_id: orgSlug, bot_id: botId })
    },
    [router, orgSlug],
  )
  const onNewBot = useCallback((parentBotId: string) => setSelection({ kind: 'newBot', parentBotId }), [])
  const onDeleteBot = useCallback((botId: string) => setConfirmDelete({ kind: 'bot', id: botId }), [])
  const onStartBot = useCallback(async (botId: string) => {
    try {
      await activateBot.mutateAsync(botId)
      snackbar.success(`Starting ${botId}…`)
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'start failed')
    }
  }, [activateBot, snackbar])
  const onStopBot = useCallback(async (botId: string) => {
    try {
      await stopBot.mutateAsync(botId)
      snackbar.success(`Stopped ${botId}`)
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'stop failed')
    }
  }, [stopBot, snackbar])
  const onRestartBot = useCallback(async (botId: string) => {
    try {
      await restartBot.mutateAsync(botId)
      snackbar.success(`Restarting ${botId}…`)
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'restart failed')
    }
  }, [restartBot, snackbar])
  const onSelectTopic = useCallback(
    (topicId: string) => {
      if (!orgSlug) return
      router.navigate('helix_org_topic_detail', { org_id: orgSlug, topic_id: topicId })
    },
    [router, orgSlug],
  )
  const onDeleteTopic = useCallback((topicId: string) => setConfirmDelete({ kind: 'topic', id: topicId }), [])
  const onSelectProcessor = useCallback(
    (processorId: string) => {
      const p = (processorsData ?? []).find((x) => x.id === processorId) ?? null
      setProcessorDrawer({ open: true, processor: p })
    },
    [processorsData],
  )
  const onDeleteProcessor = useCallback((processorId: string) => setConfirmDelete({ kind: 'processor', id: processorId }), [])
  const handlers = useMemo(
    () => ({
      onSelectBot, onOpenBotDetails, onNewBot, onDeleteBot, onStartBot, onStopBot, onRestartBot,
      onSelectTopic, onDeleteTopic, onSelectProcessor, onDeleteProcessor,
    }),
    [onSelectBot, onOpenBotDetails, onNewBot, onDeleteBot, onStartBot, onStopBot, onRestartBot, onSelectTopic, onDeleteTopic, onSelectProcessor, onDeleteProcessor],
  )

  const onAddParent = useCallback(
    async (childBotId: string, newParentBotId: string) => {
      try {
        await addParent.mutateAsync({ botID: childBotId, parentID: newParentBotId })
        snackbar.success(`${childBotId} now reports to ${newParentBotId}`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'add reporting line failed')
      }
    },
    [addParent, snackbar],
  )

  const onRemoveParent = useCallback(
    async (childBotId: string, parentBotId: string) => {
      try {
        await removeParent.mutateAsync({ botID: childBotId, parentID: parentBotId })
        snackbar.success(`${childBotId} no longer reports to ${parentBotId}`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'remove reporting line failed')
      }
    },
    [removeParent, snackbar],
  )

  const onSubscribeBot = useCallback(
    async (botId: string, topicId: string) => {
      try {
        await subscribe.mutateAsync({ botID: botId, topicID: topicId })
        snackbar.success(`${botId} now consumes ${topicId}`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'subscribe failed')
      }
    },
    [subscribe, snackbar],
  )

  const onUnsubscribeBot = useCallback(
    async (botId: string, topicId: string) => {
      try {
        await unsubscribe.mutateAsync({ botID: botId, topicID: topicId })
        snackbar.success(`${botId} no longer consumes ${topicId}`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.error ?? err?.message ?? 'unsubscribe failed')
      }
    },
    [unsubscribe, snackbar],
  )

  // onSetProcessorInput re-points a processor at a new input topic (from
  // wiring a Topic — or another processor's output branch — into its IN
  // port). Preserves the processor's name/kind/config; only the input
  // changes. The cycle check runs server-side.
  const onSetProcessorInput = useCallback(
    async (processorId: string, topicId: string) => {
      const p = (processorsData ?? []).find((x) => x.id === processorId)
      if (!p) return
      try {
        await updateProcessor.mutateAsync({
          id: processorId,
          attrs: { name: p.name ?? processorId, kind: p.kind ?? 'template', config: p.config, input_topic_id: topicId },
        })
        snackbar.success(topicId ? `${processorId} now reads ${topicId}` : `${processorId} disconnected from its input`)
      } catch (err: any) {
        snackbar.error(err?.response?.data?.errors?.[0]?.detail ?? err?.response?.data?.error ?? err?.message ?? 'wire input failed')
      }
    },
    [processorsData, updateProcessor, snackbar],
  )

  // Persist the full canvas after a drag. Saving only the moved node
  // left topics/processors on auto-layout, which re-anchored them to
  // the bot and made them "follow" the drag. Snapshot freezes everyone.
  const onLayoutSnapshot = useCallback(
    (positions: { kind: string; id: string; x: number; y: number }[]) => {
      upsertPositions.mutate(positions, {
        onError: (err: any) => {
          snackbar.error(err?.response?.data?.error ?? err?.message ?? 'save position failed')
        },
      })
    },
    [upsertPositions, snackbar],
  )

  const onResetLayout = useCallback(async () => {
    try {
      await clearPositions.mutateAsync()
      snackbar.success('layout reset to auto')
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'reset layout failed')
    }
  }, [clearPositions, snackbar])

  const handleConfirmDelete = async () => {
    if (!confirmDelete) return
    try {
      if (confirmDelete.kind === 'bot') {
        await deleteBot.mutateAsync(confirmDelete.id)
        snackbar.success(`deleted bot ${confirmDelete.id}`)
      } else if (confirmDelete.kind === 'topic') {
        await deleteTopic.mutateAsync(confirmDelete.id)
        snackbar.success(`deleted topic ${confirmDelete.id}`)
      } else {
        await deleteProcessor.mutateAsync(confirmDelete.id)
        snackbar.success(`deleted processor ${confirmDelete.id}`)
      }
      setConfirmDelete(null)
    } catch (err: any) {
      const status = err?.response?.status
      // Processor endpoints emit JSON:API errors[]; the others emit
      // {error}. Read both shapes.
      const msg = err?.response?.data?.error ?? err?.response?.data?.errors?.[0]?.detail ?? err?.message ?? 'delete failed'
      if (status === 409) {
        snackbar.error(`${confirmDelete.kind} is protected and cannot be deleted`)
      } else {
        snackbar.error(msg)
      }
    }
  }

  const confirmBody = useMemo(() => {
    if (!confirmDelete) return ''
    if (confirmDelete.kind === 'topic') {
      const s = (streamsData?.topics ?? []).find((x) => x.id === confirmDelete.id)
      const subs = s?.subscribers ?? []
      return [
        `Deleting topic ${confirmDelete.id}:`,
        `  • removes the Topic row`,
        `  • drops ${subs.length} subscription${subs.length === 1 ? '' : 's'}${subs.length > 0 ? ' (' + subs.join(', ') + ')' : ''}`,
        `  • events on this topic survive as an audit trail`,
        '',
        'This is irreversible.',
      ].join('\n')
    }
    if (confirmDelete.kind === 'processor') {
      const p = (processorsData ?? []).find((x) => x.id === confirmDelete.id)
      const owned = (p?.outputs ?? []).filter((o) => o.owned).map((o) => o.topic_id)
      return [
        `Deleting processor ${confirmDelete.id}:`,
        `  • removes the Processor`,
        `  • deletes ${owned.length} auto-created output topic${owned.length === 1 ? '' : 's'}${owned.length > 0 ? ' (' + owned.join(', ') + ')' : ''} and their subscriptions`,
        '',
        'This is irreversible.',
      ].join('\n')
    }
    const reports = flat.filter((b) => b.parentIds.includes(confirmDelete.id)).map((b) => b.id)
    return [
      `Deleting bot ${confirmDelete.id} will cascade:`,
      `  • stops sessions, deletes its project + agent app, drops its subscriptions`,
      reports.length > 0
        ? `  • ${reports.length} direct report${reports.length === 1 ? '' : 's'} (${reports.join(', ')}) lose their manager`
        : `  • no direct reports`,
      '',
      'This is irreversible.',
    ].join('\n')
  }, [confirmDelete, flat, streamsData, processorsData])

  return (
    <HelixOrgShell>
      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
        <Box
          sx={{
            flex: 1,
            minHeight: 0,
            m: 1.5,
            position: 'relative',
            border: `1px solid ${canvasBorder}`,
            borderRadius: 1,
            backgroundColor: canvasBg,
            overflow: 'hidden',
          }}
        >
          <Stack direction="row" spacing={1} sx={{ position: 'absolute', top: 12, right: 12, zIndex: 5 }}>
            <Button
              size="small"
              variant="outlined"
              onClick={onResetLayout}
              disabled={clearPositions.isPending || Object.keys(savedPositions).length === 0}
            >
              Reset layout
            </Button>
            <Button
              size="small"
              variant="outlined"
              startIcon={<TransformIcon />}
              onClick={() => setProcessorDrawer({ open: true, processor: null })}
            >
              Processor
            </Button>
            <Button
              size="small"
              variant="contained"
              color="secondary"
              startIcon={<AddIcon />}
              onClick={() => setBotDialogOpen(true)}
            >
              New bot
            </Button>
          </Stack>

          {isLoading ? (
            <Box sx={{ p: 4 }}><LoadingSpinner /></Box>
          ) : flat.length === 0 ? (
            <Box sx={{ p: 4 }}>
              <Typography variant="body1" sx={{ color: subtitleColor }}>
                No bots yet. Click <strong>New bot</strong> to get started.
              </Typography>
            </Box>
          ) : (
            <ReactFlowProvider>
              <ChartCanvas
                flat={flat}
                handlers={handlers}
                onAddParent={onAddParent}
                onRemoveParent={onRemoveParent}
                onSubscribeBot={onSubscribeBot}
                onUnsubscribeBot={onUnsubscribeBot}
                onSetProcessorInput={onSetProcessorInput}
                onLayoutSnapshot={onLayoutSnapshot}
                topics={topics}
                messageCounts={messageCounts}
                processors={processorSummaries}
                savedPositions={savedPositions}
              />
            </ReactFlowProvider>
          )}
          <PeoplePanel people={people} onSelect={onSelectPerson} />
        </Box>
      </Box>

      <NewBotDialog
        open={botDialogOpen || selection.kind === 'newBot'}
        onClose={() => { setBotDialogOpen(false); setSelection({ kind: 'none' }) }}
        presetParentId={selection.kind === 'newBot' ? selection.parentBotId : undefined}
      />
      <ConfirmDeleteDialog
        open={confirmDelete !== null}
        title={
          confirmDelete?.kind === 'topic' ? 'Delete topic?' :
          confirmDelete?.kind === 'processor' ? 'Delete processor?' :
          'Delete bot?'
        }
        body={confirmBody}
        onConfirm={handleConfirmDelete}
        onClose={() => setConfirmDelete(null)}
        pending={deleteBot.isPending || deleteTopic.isPending || deleteProcessor.isPending}
      />

      <ProcessorConfigDrawer
        open={processorDrawer.open}
        processor={processorDrawer.processor}
        onClose={() => setProcessorDrawer({ open: false, processor: null })}
      />
    </HelixOrgShell>
  )
}

export default HelixOrgChart
