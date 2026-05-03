import { FC, useState, useRef } from 'react'
import Box from '@mui/material/Box'
import Breadcrumbs from '@mui/material/Breadcrumbs'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import Link from '@mui/material/Link'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import Typography from '@mui/material/Typography'
import FolderIcon from '@mui/icons-material/Folder'
import DescriptionIcon from '@mui/icons-material/Description'
import ArrowUpwardIcon from '@mui/icons-material/ArrowUpward'
import RefreshIcon from '@mui/icons-material/Refresh'
import DownloadIcon from '@mui/icons-material/Download'
import UploadIcon from '@mui/icons-material/Upload'
import DeleteIcon from '@mui/icons-material/DeleteOutline'

import { useSandboxFiles, sandboxFileUrl } from '../../services/sandboxesService'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'

interface Props {
  orgId: string
  sandboxId: string
  running: boolean
}

// Walk up one level — works for both abs paths and slashes-only.
function parentOf(path: string): string {
  if (!path || path === '/') return '/'
  const trimmed = path.replace(/\/+$/, '')
  const idx = trimmed.lastIndexOf('/')
  return idx <= 0 ? '/' : trimmed.slice(0, idx)
}

const SandboxFilesTab: FC<Props> = ({ orgId, sandboxId, running }) => {
  const api = useApi()
  const snackbar = useSnackbar()
  const [path, setPath] = useState('/root')
  const fileInput = useRef<HTMLInputElement | null>(null)
  const { data, isLoading, refetch } = useSandboxFiles(orgId, sandboxId, path, { enabled: running })

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
      const res = await fetch(`/api/v1/organizations/${orgId}/sandboxes/${sandboxId}/files?path=${encodeURIComponent(target)}`, {
        method: 'PUT',
        body: buf,
      })
      if (!res.ok) {
        snackbar.error(`Upload failed: ${res.statusText}`)
        return
      }
      snackbar.success(`Uploaded ${file.name}`)
      refetch()
    } catch (e: any) {
      snackbar.error(`Upload failed: ${e?.message ?? 'unknown error'}`)
    }
  }

  const handleDelete = async (filePath: string, isDir: boolean) => {
    const url = `/organizations/${orgId}/sandboxes/${sandboxId}/files?path=${encodeURIComponent(filePath)}${isDir ? '&recursive=1' : ''}`
    const res = await api.delete(url)
    if (res !== null) {
      snackbar.success('Deleted')
      refetch()
    }
  }

  return (
    <Stack spacing={2}>
      <Paper sx={{ p: 2 }}>
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
      </Paper>

      <TableContainer component={Paper}>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Size</TableCell>
              <TableCell>Mode</TableCell>
              <TableCell>Modified</TableCell>
              <TableCell />
            </TableRow>
          </TableHead>
          <TableBody>
            {!running && (
              <TableRow>
                <TableCell colSpan={5}>
                  <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
                    Sandbox is not running yet.
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {running && isLoading && (
              <TableRow>
                <TableCell colSpan={5}>
                  <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
                    Loading…
                  </Typography>
                </TableCell>
              </TableRow>
            )}
            {running &&
              !isLoading &&
              (data?.entries ?? []).map((entry) => (
                <TableRow key={entry.path} hover>
                  <TableCell>
                    <Box display="flex" alignItems="center" gap={1}>
                      {entry.is_dir ? <FolderIcon fontSize="small" /> : <DescriptionIcon fontSize="small" />}
                      <Link
                        component="button"
                        onClick={() => entry.is_dir && setPath(entry.path)}
                        disabled={!entry.is_dir}
                        sx={{ fontFamily: 'monospace' }}
                      >
                        {entry.name}
                      </Link>
                    </Box>
                  </TableCell>
                  <TableCell>{entry.is_dir ? '-' : entry.size}</TableCell>
                  <TableCell sx={{ fontFamily: 'monospace' }}>{entry.mode}</TableCell>
                  <TableCell>{entry.mod_time}</TableCell>
                  <TableCell align="right">
                    {!entry.is_dir && (
                      <IconButton size="small" onClick={() => handleDownload(entry.path)}>
                        <DownloadIcon fontSize="small" />
                      </IconButton>
                    )}
                    <IconButton size="small" onClick={() => handleDelete(entry.path, entry.is_dir)}>
                      <DeleteIcon fontSize="small" />
                    </IconButton>
                  </TableCell>
                </TableRow>
              ))}
            {running && !isLoading && (data?.entries ?? []).length === 0 && (
              <TableRow>
                <TableCell colSpan={5}>
                  <Typography variant="body2" color="text.secondary" sx={{ p: 2, textAlign: 'center' }}>
                    Empty directory.
                  </Typography>
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Stack>
  )
}

export default SandboxFilesTab
