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
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import PersonAddOutlinedIcon from '@mui/icons-material/PersonAddOutlined'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'
import TransformIcon from '@mui/icons-material/Transform'

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
  ConnectionMode,
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
import NewBotDialog from '../components/helix-org/NewBotDialog'
import ProcessorConfigDrawer from '../components/helix-org/ProcessorConfigDrawer'
import ProcessorNode, { ProcessorNodeData, procNodeHeight } from '../components/helix-org/ProcessorNode'
import useLightTheme from '../hooks/useLightTheme'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  BotDTO,
  ProcessorDTO,
  useDeleteBot,
  useDeleteHelixOrgTopic,
  useListHelixOrgBots,
  useListHelixOrgTopics,
  useTopicMessageCounts,
  useListHelixOrgProcessors,
  useDeleteHelixOrgProcessor,
  useUpdateHelixOrgProcessor,
  useAddBotParent,
  useRemoveBotParent,
  useSubscribeBotAtChart,
  useUnsubscribeBotAtChart,
} from '../services/helixOrgService'

// The chart visualises the org as a ReactFlow graph. Bots are plain
// nodes wired by reporting edges:
//
//   [b-owner]
//      │ (bot-to-bot reporting edge, from a reporting line)
//      ↓
//   [b-alice]  [b-bob]  [b-carol]
//
// Reporting is a many-to-many relation: each (manager → report) reporting
// line becomes a Bot → Bot edge (a Bot may have several incoming edges).
// Topics hang off the right of the tree; an edge from a Bot to a Topic is
// a subscription.
//
// Layout: dagre runs over the bot graph (edges = reporting lines) to get
// global (x, y) for each Bot node.

const BOT_W = 220
const BOT_H = 96
const BOT_GAP_X = 32
const BOT_GAP_Y = 90

// ---- Flatten -----------------------------------------------------------

type FlatBot = {
  id: string
  // Human-readable display label; empty falls back to the id.
  name: string
  // Reporting is many-to-many: a Bot may report to several managers.
  parentIds: string[]
  // "" for an agent Bot (the default) or "human" for a person placeholder.
  kind: string
}

// ---- Node renderers ----------------------------------------------------

type BotNodeData = {
  botId: string
  botName: string
  // "" for an agent Bot or "human" for a person placeholder — the node
  // renders with a person icon and "Human" label when human.
  kind: string
  onSelectBot: (botId: string) => void
  onNewBot: (parentBotId: string) => void
  onDeleteBot: (botId: string) => void
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

const BotNode: FC<NodeProps<Node<BotNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const isHuman = data.kind === 'human'
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const border = isHuman
    ? (lightTheme.isLight ? 'rgba(30,110,180,0.45)' : 'rgba(90,160,230,0.5)')
    : (lightTheme.isLight ? 'rgba(0,0,0,0.14)' : 'rgba(255,255,255,0.18)')
  const bg = lightTheme.isLight ? '#fff' : 'rgba(255,255,255,0.05)'
  const hoverBg = lightTheme.isLight ? 'rgba(0,0,0,0.02)' : 'rgba(255,255,255,0.08)'
  const handleColor = lightTheme.isLight ? 'rgba(0,0,0,0.35)' : 'rgba(255,255,255,0.35)'

  return (
    <Box
      className={NO_DRAG_NO_PAN}
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
        cursor: 'pointer',
        '&:hover': { backgroundColor: hoverBg },
      }}
    >
      {/* Target handle = where a manager's edge LANDS, marking this bot as
          the subordinate. Source handle = where the user drags FROM when
          this bot becomes the manager. */}
      <Handle
        type="target"
        position={RFPosition.Top}
        style={{ background: handleColor, width: 12, height: 12 }}
      />
      <Stack direction="row" justifyContent="space-between" alignItems="flex-start">
        <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0 }}>
          {isHuman
            ? <PersonOutlineIcon sx={{ fontSize: 18, color: 'rgba(60,140,210,0.9)' }} />
            : <SmartToyOutlinedIcon sx={{ fontSize: 18, color: muted }} />}
          <Typography
            variant="body2"
            sx={{ fontSize: '0.85rem', fontWeight: 600, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
          >
            {data.botName || data.botId}
          </Typography>
        </Stack>
        <Stack direction="row" spacing={0.25}>
          <Tooltip title="New bot reporting to this one">
            <IconButton
              className={NO_DRAG_NO_PAN}
              size="small"
              onClick={(e) => { e.stopPropagation(); data.onNewBot(data.botId) }}
              sx={{ p: 0.25, color: muted }}
            >
              <PersonAddOutlinedIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
          <Tooltip title="Delete bot">
            <IconButton
              className={NO_DRAG_NO_PAN}
              size="small"
              onClick={(e) => { e.stopPropagation(); data.onDeleteBot(data.botId) }}
              sx={{ p: 0.25, color: muted }}
            >
              <DeleteOutlineIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Tooltip>
        </Stack>
      </Stack>
      <Typography variant="caption" sx={{ color: muted, fontSize: '0.65rem', mt: 'auto' }}>
        {isHuman ? 'Human' : 'Bot'}
      </Typography>
      <Handle
        type="source"
        position={RFPosition.Bottom}
        style={{ background: handleColor, width: 12, height: 12 }}
      />
      {/* Dedicated source handle for topic/subscription edges, anchored on
          the right side of the card. Decoupling topic edges from the
          bottom-center reporting handle means a subscription edge and a
          manager → subordinate edge can never share the same geometry.
          id="topic" is what buildGraph passes as sourceHandle when
          emitting subscription edges. */}
      <Handle
        id="topic"
        type="source"
        position={RFPosition.Right}
        isConnectable
        style={{ background: 'rgba(180,100,0,0.85)', border: 'none', width: 14, height: 14, zIndex: 5 }}
      />
    </Box>
  )
}

// TopicNode is a small pseudo-node — narrower than a Bot card — rendered
// beside the org tree to anchor subscription edges. Clicking the body
// navigates to the per-topic detail page; the trash icon deletes the
// Topic row (irreversible).
const STREAM_W = 180
const STREAM_H = 80
const TopicNode: FC<NodeProps<Node<TopicNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const accent = lightTheme.isLight ? 'rgba(180,100,0,0.85)' : 'rgba(255,180,80,0.85)'
  const bg = 'rgba(255,180,80,0.06)'
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.55)' : 'rgba(255,255,255,0.55)'
  const handleColor = lightTheme.isLight ? 'rgba(180,100,0,0.55)' : 'rgba(255,180,80,0.55)'
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
        cursor: 'pointer',
        position: 'relative',
        '&:hover': { backgroundColor: 'rgba(255,180,80,0.12)' },
      }}
    >
      <Handle type="target" position={RFPosition.Left} style={{ background: handleColor, width: 8, height: 8 }} />
      {/* Source handle on the right — drag from a Topic into a Processor's
          IN port to make that Processor read this Topic. */}
      <Handle id="src" type="source" position={RFPosition.Right} isConnectable style={{ background: accent, width: 10, height: 10 }} />
      {data.ownedByProcessor ? (
        <Tooltip title={`Output of processor ${data.ownedByProcessor} — delete the processor to remove this topic`}>
          <Box sx={{ position: 'absolute', top: 2, right: 4, fontSize: '0.6rem', color: muted, fontFamily: 'monospace' }}>
            ⟜ {data.ownedByProcessor}
          </Box>
        </Tooltip>
      ) : (
        <Tooltip title="Delete topic">
          <IconButton
            className={NO_DRAG_NO_PAN}
            size="small"
            onClick={(e) => { e.stopPropagation(); data.onDeleteTopic(data.topicId) }}
            sx={{ position: 'absolute', top: 2, right: 2, p: 0.25, color: muted }}
          >
            <DeleteOutlineIcon sx={{ fontSize: 14 }} />
          </IconButton>
        </Tooltip>
      )}
      <Typography variant="caption" sx={{ fontFamily: 'monospace', fontSize: '0.7rem', color: muted, pr: 2 }}>
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
    onNewBot: (parentBotId: string) => void
    onDeleteBot: (botId: string) => void
    onSelectTopic: (topicId: string) => void
    onDeleteTopic: (topicId: string) => void
    onSelectProcessor: (processorId: string) => void
    onDeleteProcessor: (processorId: string) => void
  },
  isLight: boolean,
  topics: TopicSummary[],
  messageCounts: Record<string, number>,
  processors: ProcessorSummary[],
): { nodes: Node[]; edges: Edge[] } => {
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
  const nodes: Node[] = []
  const botAbs = new Map<string, { x: number; y: number }>()
  for (const b of flat) {
    const ln = g.node(`bot:${b.id}`)
    if (!ln) continue
    const x = ln.x - BOT_W / 2
    const y = ln.y - BOT_H / 2
    botAbs.set(b.id, { x, y })
    nodes.push({
      id: `bot:${b.id}`,
      type: 'bot',
      position: { x, y },
      data: {
        botId: b.id,
        botName: b.name,
        kind: b.kind,
        onSelectBot: handlers.onSelectBot,
        onNewBot: handlers.onNewBot,
        onDeleteBot: handlers.onDeleteBot,
      } as BotNodeData,
      draggable: false,
      connectable: true,
    })
  }

  // 3. Reporting edges: manager → subordinate, one per reporting line (a
  //    Bot may report to several). Bezier (the default) gives every pair
  //    its own arc so multiple reports from one manager never overlap.
  const edges: Edge[] = []
  for (const b of flat) {
    for (const parentId of b.parentIds) {
      if (!parentId || !flatByID.has(parentId)) continue
      edges.push({
        id: `report:${parentId}->${b.id}`,
        source: `bot:${parentId}`,
        target: `bot:${b.id}`,
        type: 'deletable',
        animated: false,
        data: { kind: 'report', childBotId: b.id, parentBotId: parentId },
        style: {
          stroke: isLight ? 'rgba(0,0,0,0.3)' : 'rgba(255,255,255,0.35)',
          strokeWidth: 1.5,
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

  let maxRight = -Infinity
  let minTop = Infinity, maxBottom = -Infinity, minLeft = Infinity
  for (const pos of botAbs.values()) {
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
      const onChart = subjectBot && botAbs.has(subjectBot) ? subjectBot : null
      resolved.push({ topic: s, subjectBot: onChart })
    }

    // Anchored topics: lay them out beside their subject Bot.
    const anchored = resolved.filter((r) => r.subjectBot)
    const placed = layoutTopicColumns(
      anchored.map((r) => ({ topic: r.topic, anchorY: botAbs.get(r.subjectBot!)!.y })),
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
      const pos = topicPos.get(s.id)!
      const { x, y } = pos
      nodes.push({
        id: `topic:${s.id}`,
        type: 'topic',
        position: { x, y },
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
        draggable: false,
        connectable: true,
        selectable: true,
      })
      const subscribingBots = (s.subscribers ?? []).filter((bid) => botAbs.has(bid))
      for (const bid of subscribingBots) {
        edges.push({
          id: `sub:${bid}->${s.id}`,
          source: `bot:${bid}`,
          sourceHandle: 'topic',
          target: `topic:${s.id}`,
          type: 'deletable',
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
      const py = placeY(inPos ? inPos.y : minTop, h)
      nodes.push({
        id: `processor:${p.id}`,
        type: 'processor',
        position: { x: PROC_COL_X, y: py },
        data: {
          processorId: p.id,
          name: p.name,
          kind: p.kind,
          outputs: p.outputs.map((o) => ({ topicId: o.topicId, label: o.label, match: o.match })),
          onSelectProcessor: handlers.onSelectProcessor,
          onDeleteProcessor: handlers.onDeleteProcessor,
          onInspectBranch: handlers.onSelectTopic,
        } as ProcessorNodeData,
        draggable: false,
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
            targetHandle: 'topic',
            type: 'deletable',
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

// ---- Custom edge: deletable on hover -----------------------------------
//
// Wraps the default straight edge with a hover affordance: a small ×
// button appears at the edge midpoint while the pointer is over the edge
// (or the button), and clicking it routes through ReactFlow's
// deleteElements API so the existing onEdgesDelete dispatch fires
// unchanged. A transparent wider stroke overlay widens the hover hit-area.
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
  const ariaLabel =
    kind === 'sub' || kind === 'proc_out' ? 'Remove subscription'
      : kind === 'proc_in' ? 'Disconnect input'
        : 'Remove reporting line'
  const show = hover || selected
  return (
    <>
      <BaseEdge id={id} path={edgePath} style={style} markerEnd={markerEnd} interactionWidth={20} />
      {/* invisible wider hit-area; must NOT inherit strokeDasharray or
          hover becomes spotty between dashes on subscription edges */}
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
  flat: FlatBot[]
  handlers: {
    onSelectBot: (botId: string) => void
    onNewBot: (parentBotId: string) => void
    onDeleteBot: (botId: string) => void
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
  topics: TopicSummary[]
  messageCounts: Record<string, number>
  processors: ProcessorSummary[]
}> = ({ flat, handlers, onAddParent, onRemoveParent, onSubscribeBot, onUnsubscribeBot, onSetProcessorInput, topics, messageCounts, processors }) => {
  const lightTheme = useLightTheme()
  const { fitView } = useReactFlow()

  const { nodes: computedNodes, edges: computedEdges } = useMemo(
    () => buildGraph(flat, handlers, lightTheme.isLight, topics, messageCounts, processors),
    [flat, handlers, lightTheme.isLight, topics, messageCounts, processors],
  )
  const [nodes, setNodes, onNodesChange] = useNodesState(computedNodes)
  const [edges, setEdges, onEdgesChange] = useEdgesState(computedEdges)

  useEffect(() => {
    setNodes(computedNodes)
    setEdges(computedEdges)
    requestAnimationFrame(() => fitView({ padding: 0.2, duration: 250 }))
  }, [computedNodes, computedEdges, fitView, setNodes, setEdges])

  // onConnect handles both wire shapes:
  //   - bot→bot:   manager wires their report. Source = manager, target =
  //     subordinate. Persists by adding a reporting line.
  //   - bot→topic: the bot consumes a topic. Persists by POSTing a (bot,
  //     topic) subscription.
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

      // Bot → Topic (subscribe) | Bot → Bot (reporting).
      if (!source.startsWith('bot:')) return
      const sourceId = source.replace(/^bot:/, '')
      if (!sourceId) return
      if (target.startsWith('topic:')) {
        const topicId = target.replace(/^topic:/, '')
        if (!topicId) return
        onSubscribeBot(sourceId, topicId)
        return
      }
      if (target.startsWith('bot:')) {
        const targetId = target.replace(/^bot:/, '')
        if (!targetId || sourceId === targetId) return
        onAddParent(targetId, sourceId)
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
        // Reporting edge: remove the specific manager line. Fall back to
        // parsing "report:<parent>-><child>" from the edge id when data is
        // missing (e.g. an edge synthesised by ReactFlow).
        const childId = d?.childBotId ?? (e.target ?? '').replace(/^bot:/, '')
        const parentId = d?.parentBotId ?? (e.source ?? '').replace(/^bot:/, '')
        if (childId && parentId && (e.target ?? '').startsWith('bot:')) onRemoveParent(childId, parentId)
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
      fitView
      fitViewOptions={{ padding: 0.2 }}
      proOptions={{ hideAttribution: true }}
      colorMode={lightTheme.isLight ? 'light' : 'dark'}
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
      {/* Both overlays anchored bottom-left so they never sit on top of the
          topic / processor column on the right (whose ports must stay
          grabbable). */}
      <Controls showInteractive={false} position="top-left" />
      <MiniMap pannable zoomable position="bottom-left" maskColor={lightTheme.isLight ? 'rgba(0,0,0,0.06)' : 'rgba(0,0,0,0.6)'} />
    </ReactFlow>
  )
}

// ---- Page --------------------------------------------------------------

type Selection =
  | { kind: 'none' }
  | { kind: 'newBot'; parentBotId: string }

const HelixOrgChart: FC = () => {
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  const router = useRouter()
  const { data: botsData, isLoading } = useListHelixOrgBots()
  const { data: streamsData } = useListHelixOrgTopics()
  const { data: processorsData } = useListHelixOrgProcessors()
  const deleteBot = useDeleteBot()
  const deleteTopic = useDeleteHelixOrgTopic()
  const deleteProcessor = useDeleteHelixOrgProcessor()
  const updateProcessor = useUpdateHelixOrgProcessor()
  const addParent = useAddBotParent()
  const removeParent = useRemoveBotParent()
  const subscribe = useSubscribeBotAtChart()
  const unsubscribe = useUnsubscribeBotAtChart()

  const flat = useMemo<FlatBot[]>(
    () => (botsData ?? []).map((b: BotDTO) => ({
      kind: b.kind ?? '',
      id: b.id ?? '',
      name: b.name ?? '',
      parentIds: b.parent_ids ?? [],
    })),
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
  const onSelectBot = useCallback(
    (botId: string) => {
      if (!orgSlug) return
      router.navigate('helix_org_bot_detail', { org_id: orgSlug, bot_id: botId })
    },
    [router, orgSlug],
  )
  const onNewBot = useCallback((parentBotId: string) => setSelection({ kind: 'newBot', parentBotId }), [])
  const onDeleteBot = useCallback((botId: string) => setConfirmDelete({ kind: 'bot', id: botId }), [])
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
    () => ({ onSelectBot, onNewBot, onDeleteBot, onSelectTopic, onDeleteTopic, onSelectProcessor, onDeleteProcessor }),
    [onSelectBot, onNewBot, onDeleteBot, onSelectTopic, onDeleteTopic, onSelectProcessor, onDeleteProcessor],
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
              Bots are wired by reporting lines. Create bots, then drag from a manager's
              bottom handle to a subordinate to set who reports to whom, or from a bot's
              right handle to a Topic to subscribe.
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
          {/* New bot lives on the canvas (floating top-right) rather than
              in the page header — it reads as a canvas action, and keeps
              the header to title + description. zIndex sits above the
              ReactFlow surface / controls. */}
          <Stack direction="row" spacing={1} sx={{ position: 'absolute', top: 12, right: 12, zIndex: 5 }}>
            <Button
              variant="outlined"
              startIcon={<TransformIcon />}
              onClick={() => setProcessorDrawer({ open: true, processor: null })}
            >
              Processor
            </Button>
            <Button
              variant="contained"
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
                topics={topics}
                messageCounts={messageCounts}
                processors={processorSummaries}
              />
            </ReactFlowProvider>
          )}
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
    </Page>
  )
}

export default HelixOrgChart
