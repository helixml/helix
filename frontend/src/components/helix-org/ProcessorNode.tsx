// ProcessorNode is the React Flow node for an org Processor (a
// transform / filter / router). Extracted from HelixOrgChart so the
// node's layout — input port on the left, one labelled output port per
// branch on the right — lives in one place.
//
// Port geometry: handles are pinned to the node's border (React Flow's
// default Left/Right positioning), each aligned to a fixed-height row so
// the dot sits exactly on the edge next to its label — never floating
// off the node. Clear "in" / "out" cues mark direction.

import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'
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
  const [menuEl, setMenuEl] = useState<null | HTMLElement>(null)
  const closeMenu = () => setMenuEl(null)

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
        <Box
          className={NO_DRAG_NO_PAN}
          sx={{ position: 'absolute', top: 2, right: 2, zIndex: 2 }}
          onClick={(e) => e.stopPropagation()}
        >
          <IconButton
            className={NO_DRAG_NO_PAN}
            size="small"
            aria-label="Processor actions"
            onClick={(e) => {
              e.stopPropagation()
              setMenuEl(e.currentTarget)
            }}
            sx={{ p: 0.25, color: muted }}
          >
            <MoreVertIcon sx={{ fontSize: 15 }} />
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
                data.onDeleteProcessor(data.processorId)
              }}
            >
              <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
              Delete processor
            </MenuItem>
          </Menu>
        </Box>
        <Stack direction="row" alignItems="center" spacing={0.5} sx={{ pr: 2.5 }}>
          <TransformIcon sx={{ fontSize: 15, color: accent }} />
          <Typography sx={{ fontSize: '0.8rem', fontWeight: 700, color: accent, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
            {data.name}
          </Typography>
        </Stack>
        <Typography sx={{ fontSize: '0.6rem', color: muted, fontFamily: 'monospace', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', pr: 2.5 }}>
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
