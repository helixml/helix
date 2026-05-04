import { FC, MouseEvent, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Breadcrumbs from '@mui/material/Breadcrumbs'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import Link from '@mui/material/Link'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import FolderIcon from '@mui/icons-material/Folder'
import DescriptionIcon from '@mui/icons-material/Description'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'
import RefreshIcon from '@mui/icons-material/Refresh'
import DownloadIcon from '@mui/icons-material/Download'
import UploadIcon from '@mui/icons-material/Upload'
import DeleteIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'

import SimpleTable from '../widgets/SimpleTable'
import { useSandboxFiles, sandboxFileUrl } from '../../services/sandboxesService'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'

interface Props {
  orgId: string
  sandboxId: string
  running: boolean
  // When true, the files browser opens at /home/retro/work — the bind-mounted
  // persistent workspace — instead of /root, since that's where any data the
  // user actually wants to keep is going to live.
  persistent?: boolean
}

// Path inside the container where the persistent volume is mounted (must
// match controller.go buildMounts). Anything written outside this path is
// ephemeral.
const PERSISTENT_WORKSPACE_PATH = '/home/retro/work'

interface FileEntry {
  name: string
  path: string
  is_dir: boolean
  size: number
  mode: string
  mod_time: string
}

// Walk up one level — works for both abs paths and slashes-only.
function parentOf(path: string): string {
  if (!path || path === '/') return '/'
  const trimmed = path.replace(/\/+$/, '')
  const idx = trimmed.lastIndexOf('/')
  return idx <= 0 ? '/' : trimmed.slice(0, idx)
}

const SandboxFilesTab: FC<Props> = ({ orgId, sandboxId, running, persistent }) => {
  const api = useApi()
  const apiClient = api.getApiClient()
  const snackbar = useSnackbar()
  const [path, setPath] = useState(persistent ? PERSISTENT_WORKSPACE_PATH : '/root')
  const fileInput = useRef<HTMLInputElement | null>(null)
  const { data, isLoading, refetch } = useSandboxFiles(orgId, sandboxId, path, { enabled: running })

  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentEntry, setCurrentEntry] = useState<FileEntry | null>(null)

  const handleMenuOpen = (e: MouseEvent<HTMLElement>, entry: FileEntry) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
    setCurrentEntry(entry)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentEntry(null)
  }

  const handleDownload = async (filePath: string) => {
    const url = sandboxFileUrl(orgId, sandboxId, filePath)
    const a = document.createElement('a')
    a.href = url
    a.download = filePath.split('/').pop() ?? 'download'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
  }

  const handleUpload = async (file: File) => {
    const target = `${path === '/' ? '' : path}/${file.name}`
    try {
      const buf = await file.arrayBuffer()
      await apiClient.request<void>({
        path: `/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/files`,
        method: 'PUT',
        query: { path: target },
        body: buf,
        secure: true,
      })
      snackbar.success(`Uploaded ${file.name}`)
      refetch()
    } catch (e: any) {
      snackbar.error(`Upload failed: ${e?.message ?? 'unknown error'}`)
    }
  }

  const handleDelete = async (filePath: string, isDir: boolean) => {
    await apiClient.v1OrganizationsSandboxesFilesDelete(orgId, sandboxId, {
      path: filePath,
      recursive: isDir ? '1' : undefined,
    })
    snackbar.success('Deleted')
    refetch()
  }

  const entries: FileEntry[] = data?.entries ?? []

  const tableData = useMemo(() => entries.map((entry) => ({
    id: entry.path,
    _data: entry,
    name: (
      <Box display="flex" alignItems="center" gap={1}>
        {entry.is_dir ? <FolderIcon fontSize="small" sx={{ color: 'text.secondary' }} /> : <DescriptionIcon fontSize="small" sx={{ color: 'text.secondary' }} />}
        <Link
          component="button"
          onClick={() => entry.is_dir && setPath(entry.path)}
          disabled={!entry.is_dir}
          sx={{
            fontFamily: 'monospace',
            textDecoration: 'none',
            fontWeight: entry.is_dir ? 600 : 400,
            color: entry.is_dir ? 'primary.main' : 'text.primary',
          }}
        >
          {entry.name}
        </Link>
      </Box>
    ),
    size: (
      <Typography variant="body2" color="text.secondary">
        {entry.is_dir ? '-' : entry.size}
      </Typography>
    ),
    mode: (
      <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
        {entry.mode}
      </Typography>
    ),
    modified: (
      <Typography variant="body2" color="text.secondary">
        {entry.mod_time}
      </Typography>
    ),
  })), [entries])

  const getActions = (row: any) => {
    const entry = row._data as FileEntry
    return (
      <IconButton size="small" onClick={(e) => handleMenuOpen(e, entry)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <Stack spacing={2}>
      <Stack direction="row" spacing={1} alignItems="center">
        <IconButton size="small" onClick={() => setPath(parentOf(path))} disabled={path === '/'}>
          <ArrowUpwardIcon fontSize="small" />
        </IconButton>
        <Breadcrumbs sx={{ flex: 1 }}>
          <Link component="button" onClick={() => setPath('/')}>
            /
          </Link>
          {path
            .split('/')
            .filter(Boolean)
            .map((seg, idx, arr) => {
              const segPath = '/' + arr.slice(0, idx + 1).join('/')
              return (
                <Link key={segPath} component="button" onClick={() => setPath(segPath)}>
                  {seg}
                </Link>
              )
            })}
        </Breadcrumbs>
        <IconButton size="small" onClick={() => refetch()}>
          <RefreshIcon fontSize="small" />
        </IconButton>
        <Button
          size="small"
          startIcon={<UploadIcon />}
          onClick={() => fileInput.current?.click()}
          disabled={!running}
        >
          Upload
        </Button>
        <input
          ref={fileInput}
          type="file"
          hidden
          onChange={(e) => {
            const file = e.target.files?.[0]
            if (file) handleUpload(file)
            e.target.value = ''
          }}
        />
      </Stack>

      {!running ? (
        <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
          Sandbox is not running yet.
        </Typography>
      ) : isLoading ? (
        <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
          Loading…
        </Typography>
      ) : entries.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ py: 4, textAlign: 'center' }}>
          Empty directory.
        </Typography>
      ) : (
        <SimpleTable
          authenticated={true}
          fields={[
            { name: 'name', title: 'Name' },
            { name: 'size', title: 'Size' },
            { name: 'mode', title: 'Mode' },
            { name: 'modified', title: 'Modified' },
          ]}
          data={tableData}
          getActions={getActions}
        />
      )}

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        {currentEntry && !currentEntry.is_dir && (
          <MenuItem
            onClick={(e) => {
              e.stopPropagation()
              handleMenuClose()
              if (currentEntry) handleDownload(currentEntry.path)
            }}
          >
            <DownloadIcon sx={{ mr: 1, fontSize: 20 }} />
            Download
          </MenuItem>
        )}
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentEntry) handleDelete(currentEntry.path, currentEntry.is_dir)
          }}
        >
          <DeleteIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>
    </Stack>
  )
}

export default SandboxFilesTab
