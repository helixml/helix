import { FC, MouseEvent, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import IconButton from '@mui/material/IconButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Download, EllipsisVertical, ExternalLink, Pencil, Trash2 } from 'lucide-react'

import { TypesArtifact, TypesArtifactKind } from '../../api/api'
import useRouter from '../../hooks/useRouter'
import { ViewMode } from '../widgets/ViewModeToggle'
import ArtifactVisibilityBadge from './ArtifactVisibilityBadge'
import CardGrid from '../widgets/CardGrid'
import SimpleTable, { TableSortDirection } from '../widgets/SimpleTable'

type Props = {
  artifacts: TypesArtifact[]
  mode: ViewMode
  // Shown in place of the table/grid when the caller's filters match nothing.
  // Rendered here rather than by the caller so the view stays mounted and keeps
  // its sort state.
  emptyMessage?: string
  onEdit: (artifact: TypesArtifact) => void
  onDelete: (artifact: TypesArtifact) => void
}

const artifactURL = (artifact: TypesArtifact) => artifact.url || ''

export const artifactKindLabel = (artifact: TypesArtifact) => {
  switch (artifact.kind) {
    case TypesArtifactKind.ArtifactKindSPA: return 'Static SPA'
    case TypesArtifactKind.ArtifactKindPDF: return 'PDF document'
    case TypesArtifactKind.ArtifactKindImage: return 'Image'
    case TypesArtifactKind.ArtifactKindMarkdown: return 'Markdown document'
    default: return 'HTML page'
  }
}

// Sortable value per column. Strings compare case-insensitively, numbers and
// timestamps compare numerically.
const sortValue = (artifact: TypesArtifact, field: string): string | number => {
  switch (field) {
    case 'kind': return artifactKindLabel(artifact).toLowerCase()
    case 'visibility': return (artifact.visibility || '').toLowerCase()
    case 'version': return artifact.active_version?.version ?? 1
    case 'updated': return artifact.updated_at ? new Date(artifact.updated_at).getTime() : 0
    default: return (artifact.name || '').toLowerCase()
  }
}

const compareArtifacts = (a: TypesArtifact, b: TypesArtifact, field: string, direction: TableSortDirection) => {
  const left = sortValue(a, field)
  const right = sortValue(b, field)
  let result = typeof left === 'number' && typeof right === 'number'
    ? left - right
    : String(left).localeCompare(String(right))
  // Tiebreak so equal values keep a predictable order.
  if (result === 0) result = (a.name || '').localeCompare(b.name || '')
  return direction === 'asc' ? result : -result
}

const ArtifactsView: FC<Props> = ({ artifacts, mode, emptyMessage, onEdit, onDelete }) => {
  const router = useRouter()
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null)
  const [currentArtifact, setCurrentArtifact] = useState<TypesArtifact>()
  // Matches the server's default ordering (updated_at DESC).
  const [sortField, setSortField] = useState('updated')
  const [sortDirection, setSortDirection] = useState<TableSortDirection>('desc')
  const artifactKey = artifacts.map((artifact) => `${artifact.id}:${artifact.updated_at}`).join('|')

  const toggleSort = (field: string) => {
    if (field === sortField) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc')
      return
    }
    setSortField(field)
    // Dates start newest-first, everything else A-Z / smallest-first.
    setSortDirection(field === 'updated' ? 'desc' : 'asc')
  }

  const sortedArtifacts = useMemo(
    () => [...artifacts].sort((a, b) => compareArtifacts(a, b, sortField, sortDirection)),
    [artifactKey, sortField, sortDirection],
  )

  const openArtifact = (artifact: TypesArtifact) => {
    if (artifact.id) router.navigate('artifact_viewer', { artifact_id: artifact.id })
  }

  // Same-origin navigation so the session cookie authenticates the download and
  // the server's Content-Disposition names the file.
  const downloadArtifact = (artifact: TypesArtifact) => {
    if (!artifact.id) return
    const link = document.createElement('a')
    link.href = `/api/v1/artifacts/${artifact.id}/download`
    link.download = ''
    document.body.appendChild(link)
    link.click()
    link.remove()
  }

  const openMenu = (event: MouseEvent<HTMLElement>, artifact: TypesArtifact) => {
    event.preventDefault()
    event.stopPropagation()
    setCurrentArtifact(artifact)
    setMenuAnchor(event.currentTarget)
  }

  const closeMenu = () => {
    setMenuAnchor(null)
    setCurrentArtifact(undefined)
  }

  const actionButton = (artifact: TypesArtifact) => (
    <Tooltip title="Artifact actions">
      <IconButton aria-label={`Actions for ${artifact.name}`} onClick={(event) => openMenu(event, artifact)}>
        <EllipsisVertical size={18} />
      </IconButton>
    </Tooltip>
  )

  const tableData = useMemo(() => sortedArtifacts.map((artifact) => ({
    id: artifact.id,
    _data: artifact,
    name: (
      <Typography variant="body2" sx={{ fontWeight: 600 }}>
        <a
          href={artifactURL(artifact)}
          onClick={(event) => {
            event.preventDefault()
            event.stopPropagation()
            openArtifact(artifact)
          }}
          style={{ color: 'inherit', textDecoration: 'none' }}
        >
          {artifact.name}
        </a>
      </Typography>
    ),
    kind: <Typography variant="body2" color="text.secondary">{artifactKindLabel(artifact)}</Typography>,
    visibility: <ArtifactVisibilityBadge visibility={artifact.visibility} />,
    version: <Typography variant="body2" color="text.secondary">v{artifact.active_version?.version ?? 1}</Typography>,
    updated: <Typography variant="body2" color="text.secondary">{artifact.updated_at ? new Date(artifact.updated_at).toLocaleString() : '—'}</Typography>,
  })), [sortedArtifacts])

  const menu = (
    <Menu
      anchorEl={menuAnchor}
      open={!!menuAnchor}
      onClose={closeMenu}
      anchorOrigin={{ vertical: 'bottom', horizontal: 'right' }}
      transformOrigin={{ vertical: 'top', horizontal: 'right' }}
      MenuListProps={{ dense: true, sx: { p: 0 } }}
      PaperProps={{ sx: { minWidth: 164 } }}
    >
      <MenuItem dense onClick={(event) => {
        event.stopPropagation()
        if (currentArtifact) openArtifact(currentArtifact)
        closeMenu()
      }}>
        <ListItemIcon><ExternalLink size={16} /></ListItemIcon>
        <ListItemText primary="Open" />
      </MenuItem>
      <MenuItem dense onClick={(event) => {
        event.stopPropagation()
        if (currentArtifact) downloadArtifact(currentArtifact)
        closeMenu()
      }}>
        <ListItemIcon><Download size={16} /></ListItemIcon>
        <ListItemText primary="Download" />
      </MenuItem>
      <MenuItem dense onClick={(event) => {
        event.stopPropagation()
        if (currentArtifact) onEdit(currentArtifact)
        closeMenu()
      }}>
        <ListItemIcon><Pencil size={16} /></ListItemIcon>
        <ListItemText primary="Edit / publish" />
      </MenuItem>
      <MenuItem dense onClick={(event) => {
        event.stopPropagation()
        if (currentArtifact) onDelete(currentArtifact)
        closeMenu()
      }}>
        <ListItemIcon><Trash2 size={16} /></ListItemIcon>
        <ListItemText primary="Delete" />
      </MenuItem>
    </Menu>
  )

  if (artifacts.length === 0) {
    return (
      <Box sx={{ textAlign: 'center', py: 6 }}>
        <Typography variant="body2" color="text.secondary">{emptyMessage || 'No artifacts to display.'}</Typography>
      </Box>
    )
  }

  if (mode === 'table') {
    return (
      <>
        <SimpleTable
          authenticated
          fields={[
            { name: 'name', title: 'Name', sortable: true },
            { name: 'kind', title: 'Type', sortable: true },
            { name: 'visibility', title: 'Visibility', sortable: true },
            { name: 'version', title: 'Version', sortable: true },
            { name: 'updated', title: 'Updated', sortable: true },
          ]}
          data={tableData}
          sortField={sortField}
          sortDirection={sortDirection}
          onSortChange={toggleSort}
          onRowClick={(row) => openArtifact(row._data as TypesArtifact)}
          getActions={(row) => actionButton(row._data as TypesArtifact)}
        />
        {menu}
      </>
    )
  }

  return (
    <>
      <CardGrid
        items={sortedArtifacts}
        getKey={(artifact) => artifact.id ?? ''}
        renderCard={(artifact) => (
          <Card
            sx={{
              border: '1px solid rgba(0, 0, 0, 0.08)', borderRadius: 1, boxShadow: 'none',
              height: '100%', display: 'flex', flexDirection: 'column',
              '&:hover': { borderColor: 'rgba(0,0,0,0.12)', backgroundColor: 'rgba(0,0,0,0.01)' },
            }}
          >
            <CardContent
              onClick={() => openArtifact(artifact)}
              sx={{ p: 2, '&:last-child': { pb: 2 }, cursor: 'pointer', flex: 1 }}
            >
              <Stack direction="row" alignItems="flex-start" justifyContent="space-between" spacing={1}>
                <Box sx={{ minWidth: 0 }}>
                  <Typography variant="subtitle1" sx={{ fontWeight: 600 }} noWrap>{artifact.name}</Typography>
                  <Typography variant="body2" color="text.secondary">{artifactKindLabel(artifact)}</Typography>
                </Box>
                <Stack direction="row" spacing={0.25} alignItems="center">
                  <ArtifactVisibilityBadge visibility={artifact.visibility} />
                  {actionButton(artifact)}
                </Stack>
              </Stack>
              {artifact.description && (
                <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>{artifact.description}</Typography>
              )}
              <Box sx={{ mt: 2, background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)', border: '1px solid rgba(255,255,255,0.06)', borderRadius: 2, p: 1.5 }}>
                <Stack direction="row" spacing={3}>
                  <Box><Typography variant="caption" color="text.secondary">VERSION</Typography><Typography variant="body2" sx={{ fontFamily: 'var(--helix-font-mono)', fontWeight: 600 }}>v{artifact.active_version?.version ?? 1}</Typography></Box>
                  <Box><Typography variant="caption" color="text.secondary">FILES</Typography><Typography variant="body2" sx={{ fontFamily: 'var(--helix-font-mono)', fontWeight: 600 }}>{artifact.active_version?.file_count ?? 1}</Typography></Box>
                </Stack>
              </Box>
            </CardContent>
          </Card>
        )}
      />
      {menu}
    </>
  )
}

export default ArtifactsView
