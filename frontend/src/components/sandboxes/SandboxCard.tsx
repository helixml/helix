import { FC, MouseEvent, useState } from 'react'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import DeleteIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'

import SandboxStatusBadge from './SandboxStatusBadge'
import { TypesSandbox } from '../../api/api'

interface SandboxCardProps {
  sandbox: TypesSandbox
  onOpen: (sandbox: TypesSandbox) => void
  onDelete: (sandbox: TypesSandbox) => void
}

const StatRow: FC<{ label: string; value: string | number }> = ({ label, value }) => (
  <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.25, minWidth: 0 }}>
    <Typography variant="caption" sx={{
      color: 'text.secondary',
      fontSize: '0.65rem',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
    }}>
      {label}
    </Typography>
    <Typography variant="body2" sx={{
      fontWeight: 600,
      color: 'text.primary',
      fontSize: '0.8rem',
      fontFamily: 'monospace',
      whiteSpace: 'nowrap',
      overflow: 'hidden',
      textOverflow: 'ellipsis',
    }}>
      {value}
    </Typography>
  </Box>
)

const formatTimestamp = (ts?: string): string => {
  if (!ts) return '-'
  return new Date(ts).toLocaleString()
}

const formatExpiry = (ts?: string): string => {
  if (!ts) return 'Never'
  return new Date(ts).toLocaleString()
}

const formatDisplay = (sandbox: TypesSandbox): string => {
  if (!sandbox.display_width || !sandbox.display_height) return '-'
  const fps = sandbox.display_fps ? `@${sandbox.display_fps}` : ''
  return `${sandbox.display_width}×${sandbox.display_height}${fps}`
}

const isHeadless = (sandbox: TypesSandbox): boolean =>
  (sandbox.runtime || '').includes('headless')

const formatResources = (sandbox: TypesSandbox): string => {
  const cpu = sandbox.vcpus ? `${sandbox.vcpus} vCPU` : '-'
  const mem = sandbox.memory_mb ? `${(sandbox.memory_mb / 1024).toFixed(1)}GB` : '-'
  return `${cpu} / ${mem}`
}

const SandboxCard: FC<SandboxCardProps> = ({ sandbox, onOpen, onDelete }) => {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)

  const handleMenuOpen = (e: MouseEvent<HTMLElement>) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
  }
  const handleMenuClose = () => setAnchorEl(null)

  return (
    <Card
      sx={{
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        backgroundColor: 'background.paper',
        border: '1px solid rgba(0, 0, 0, 0.08)',
        borderRadius: 1,
        boxShadow: 'none',
        transition: 'all 0.15s ease-in-out',
        '&:hover': {
          borderColor: 'rgba(0, 0, 0, 0.12)',
          backgroundColor: 'rgba(0, 0, 0, 0.01)',
        },
      }}
    >
      <CardContent
        sx={{
          flexGrow: 1,
          cursor: 'pointer',
          p: 2,
          '&:last-child': { pb: 2 },
          display: 'flex',
          flexDirection: 'column',
        }}
        onClick={() => onOpen(sandbox)}
      >
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 1.5, gap: 1 }}>
          <Box sx={{ flex: 1, minWidth: 0 }}>
            <Typography
              variant="body2"
              sx={{
                fontWeight: 600,
                fontFamily: 'monospace',
                lineHeight: 1.4,
                color: 'text.primary',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {sandbox.name || sandbox.id}
            </Typography>
            <Typography
              variant="caption"
              sx={{
                color: 'text.secondary',
                fontSize: '0.7rem',
                display: 'block',
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              }}
            >
              {sandbox.runtime || 'ubuntu-desktop'}
            </Typography>
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }}>
            <SandboxStatusBadge status={sandbox.status} message={sandbox.status_message} />
            <IconButton size="small" onClick={handleMenuOpen}>
              <MoreVertIcon sx={{ fontSize: 16 }} />
            </IconButton>
          </Box>
        </Box>

        <Box sx={{
          background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)',
          borderRadius: 2,
          border: '1px solid rgba(255,255,255,0.06)',
          p: 1.5,
          mt: 'auto',
          display: 'grid',
          gridTemplateColumns: 'repeat(2, 1fr)',
          gap: 1,
        }}>
          <StatRow label="Resources" value={formatResources(sandbox)} />
          {!isHeadless(sandbox) && (
            <StatRow label="Display" value={formatDisplay(sandbox)} />
          )}
          <StatRow label="Created" value={formatTimestamp(sandbox.created_at)} />
          <StatRow label="Expires" value={formatExpiry(sandbox.expires_at)} />
        </Box>
      </CardContent>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            onOpen(sandbox)
          }}
        >
          <OpenInNewIcon sx={{ mr: 1, fontSize: 20 }} />
          Open
        </MenuItem>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            onDelete(sandbox)
          }}
        >
          <DeleteIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>
    </Card>
  )
}

export default SandboxCard
