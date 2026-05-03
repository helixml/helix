import { FC, MouseEvent, useMemo, useState } from 'react'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import useTheme from '@mui/material/styles/useTheme'
import DeleteIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'

import SimpleTable from '../widgets/SimpleTable'
import SandboxStatusBadge from './SandboxStatusBadge'
import { TypesSandbox } from '../../api/api'

interface SandboxesTableProps {
  sandboxes: TypesSandbox[]
  onOpen: (sandbox: TypesSandbox) => void
  onDelete: (sandbox: TypesSandbox) => void
}

const SandboxesTable: FC<SandboxesTableProps> = ({ sandboxes, onOpen, onDelete }) => {
  const theme = useTheme()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentSandbox, setCurrentSandbox] = useState<TypesSandbox | null>(null)

  const handleMenuOpen = (e: MouseEvent<HTMLElement>, sb: TypesSandbox) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
    setCurrentSandbox(sb)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentSandbox(null)
  }

  const tableData = useMemo(() => sandboxes.map((sb) => ({
    id: sb.id,
    _data: sb,
    name: (
      <Typography variant="body1">
        <a
          style={{
            textDecoration: 'none',
            fontWeight: 'bold',
            color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
            fontFamily: 'monospace',
          }}
          href="#"
          onClick={(e) => {
            e.preventDefault()
            e.stopPropagation()
            onOpen(sb)
          }}
        >
          {sb.name || sb.id}
        </a>
      </Typography>
    ),
    runtime: (
      <Typography variant="body2" color="text.secondary">
        {sb.runtime || 'ubuntu-desktop'}
      </Typography>
    ),
    status: <SandboxStatusBadge status={sb.status} message={sb.status_message} />,
    created: (
      <Typography variant="body2" color="text.secondary">
        {sb.created_at ? new Date(sb.created_at).toLocaleString() : '-'}
      </Typography>
    ),
    expires: (
      <Typography variant="body2" color="text.secondary">
        {sb.expires_at ? new Date(sb.expires_at).toLocaleString() : '-'}
      </Typography>
    ),
  })), [sandboxes, theme, onOpen])

  const getActions = (row: any) => {
    const sb = row._data as TypesSandbox
    return (
      <IconButton size="small" onClick={(e) => handleMenuOpen(e, sb)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <>
      <SimpleTable
        authenticated={true}
        fields={[
          { name: 'name', title: 'Name' },
          { name: 'runtime', title: 'Runtime' },
          { name: 'status', title: 'Status' },
          { name: 'created', title: 'Created' },
          { name: 'expires', title: 'Expires' },
        ]}
        data={tableData}
        getActions={getActions}
      />
      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentSandbox) onOpen(currentSandbox)
          }}
        >
          <OpenInNewIcon sx={{ mr: 1, fontSize: 20 }} />
          Open
        </MenuItem>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentSandbox) onDelete(currentSandbox)
          }}
        >
          <DeleteIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>
    </>
  )
}

export default SandboxesTable
