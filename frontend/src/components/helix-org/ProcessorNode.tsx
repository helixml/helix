// ProcessorNode is the React Flow node for an org Processor (a
// transform / filter / router). Extracted from HelixOrgChart so the
// node's layout — input port on the left, one labelled output port per
// branch on the right — lives in one place.
//
// Port geometry: handles are pinned to the node's border (React Flow's
// default Left/Right positioning), each aligned to a fixed-height row so
// the dot sits exactly on the edge next to its label — never floating
// off the node. Clear "in" / "out" cues mark direction.

import { FC } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import TransformIcon from '@mui/icons-material/Transform'
import { Handle, Node, NodeProps, Position } from '@xyflow/react'

import useLightTheme from '../../hooks/useLightTheme'

// React Flow swallows pointer events on elements with these classes, so
// dragging the node body doesn't fire on icon buttons / scroll.
const NO_DRAG_NO_PAN = 'nodrag nopan'

export type ProcessorBranch = {
  topicId: string
  label: string
  match: string
}

export type ProcessorNodeData = {
  processorId: string
  name: string
  kind: string
  // One entry per output branch. Each renders a labelled source handle
  // on the node's right edge; dragging from it to a Worker subscribes
  // that Worker to the branch's (hidden) output topic.
  outputs: ProcessorBranch[]
  onSelectProcessor: (processorId: string) => void
  onDeleteProcessor: (processorId: string) => void
  onInspectBranch: (topicId: string) => void
}

// Fixed geometry so handle positions and row positions line up exactly.
export const PROC_W = 220
const HEADER_H = 50
const ROW_H = 28
const PORT = 12 // handle diameter
export const procNodeHeight = (outputCount: number) => HEADER_H + Math.max(1, outputCount) * ROW_H

const ProcessorNode: FC<NodeProps<Node<ProcessorNodeData>>> = ({ data }) => {
  const lightTheme = useLightTheme()
  const accent = lightTheme.isLight ? 'rgba(90,60,170,0.95)' : 'rgba(180,150,255,0.95)'
  const inAccent = lightTheme.isLight ? 'rgba(180,100,0,0.9)' : 'rgba(255,180,80,0.9)'
  const bg = lightTheme.isLight ? 'rgba(140,110,230,0.06)' : 'rgba(140,110,230,0.10)'
  const cardBg = lightTheme.isLight ? '#fff' : 'rgba(255,255,255,0.04)'
  const muted = lightTheme.isLight ? 'rgba(0,0,0,0.5)' : 'rgba(255,255,255,0.5)'
  const border = lightTheme.isLight ? 'rgba(90,60,170,0.4)' : 'rgba(180,150,255,0.4)'

  const outputs = data.outputs.length > 0 ? data.outputs : [{ topicId: '', label: 'out', match: '' }]
  const height = procNodeHeight(outputs.length)

  const handleBase = { width: PORT, height: PORT, borderRadius: '50%', border: '2px solid #fff' }

  return (
    <Box
      sx={{
        width: PROC_W,
        height,
        position: 'relative',
        borderRadius: 2,
        border: `1px solid ${border}`,
        backgroundColor: bg,
        boxShadow: lightTheme.isLight ? '0 1px 3px rgba(0,0,0,0.08)' : 'none',
        cursor: 'grab',
        '&:active': { cursor: 'grabbing' },
      }}
    >
      {/* INPUT port — pinned to the left border, level with the header. */}
      <Handle
        type="target"
        position={Position.Left}
        style={{ ...handleBase, background: inAccent, top: HEADER_H / 2 }}
      />
      <Box
        sx={{
          position: 'absolute', left: 10, top: HEADER_H / 2, transform: 'translateY(-50%)',
          px: 0.5, borderRadius: 0.5, backgroundColor: inAccent, lineHeight: 1,
        }}
      >
        <Typography sx={{ fontSize: '0.55rem', fontWeight: 700, color: '#fff', letterSpacing: '0.5px' }}>IN</Typography>
      </Box>

      {/* Header — clicking it opens the edit drawer. */}
      <Box
        onClick={(e) => { e.stopPropagation(); data.onSelectProcessor(data.processorId) }}
        sx={{
          height: HEADER_H, boxSizing: 'border-box',
          pl: 4, pr: 1, py: 0.75,
          borderTopLeftRadius: 8, borderTopRightRadius: 8,
          backgroundColor: cardBg, borderBottom: `1px solid ${border}`,
          cursor: 'pointer', display: 'flex', flexDirection: 'column', justifyContent: 'center',
          '&:hover': { backgroundColor: lightTheme.isLight ? 'rgba(140,110,230,0.06)' : 'rgba(255,255,255,0.06)' },
        }}
      >
        <Tooltip title="Delete processor (and its output topics)">
          <IconButton
            className={NO_DRAG_NO_PAN}
            size="small"
            onClick={(e) => { e.stopPropagation(); data.onDeleteProcessor(data.processorId) }}
            sx={{ position: 'absolute', top: 4, right: 4, p: 0.25, color: muted }}
          >
            <DeleteOutlineIcon sx={{ fontSize: 15 }} />
          </IconButton>
        </Tooltip>
        <Stack direction="row" alignItems="center" spacing={0.5} sx={{ pr: 2 }}>
          <TransformIcon sx={{ fontSize: 15, color: accent }} />
          <Typography sx={{ fontSize: '0.8rem', fontWeight: 700, color: accent, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {data.name}
          </Typography>
        </Stack>
        <Typography sx={{ fontSize: '0.6rem', color: muted, fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {data.kind} · {data.processorId}
        </Typography>
      </Box>

      {/* OUTPUT ports — one labelled row per branch, dot pinned to the
          right border at the row's centre. */}
      {outputs.map((o, i) => {
        const rowTop = HEADER_H + i * ROW_H
        return (
          <Box
            key={o.topicId || i}
            onClick={(e) => { e.stopPropagation(); if (o.topicId) data.onInspectBranch(o.topicId) }}
            sx={{
              position: 'absolute', top: rowTop, left: 0, right: 0, height: ROW_H,
              display: 'flex', alignItems: 'center', justifyContent: 'flex-end', gap: 0.5, pr: 1.5,
              borderBottom: i < outputs.length - 1 ? `1px dashed ${border}` : 'none',
              cursor: o.topicId ? 'pointer' : 'default',
              '&:hover': { backgroundColor: lightTheme.isLight ? 'rgba(140,110,230,0.07)' : 'rgba(255,255,255,0.05)' },
            }}
          >
            <Typography sx={{ fontSize: '0.5rem', fontWeight: 700, color: accent, opacity: 0.7, letterSpacing: '0.5px' }}>OUT</Typography>
            <Typography sx={{ fontSize: '0.68rem', color: muted, fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', maxWidth: 120 }}>
              {o.label || (o.match ? 'match' : 'default')}
            </Typography>
            {/* The handle is a child of this absolutely-positioned row,
                so React Flow's default top:50% centres it vertically
                within the row — keeping the dot pinned to the right
                border next to its label (no manual node-space math). */}
            <Handle
              id={o.topicId || `out-${i}`}
              type="source"
              position={Position.Right}
              isConnectable
              style={{ ...handleBase, background: accent }}
            />
          </Box>
        )
      })}
    </Box>
  )
}

export default ProcessorNode
