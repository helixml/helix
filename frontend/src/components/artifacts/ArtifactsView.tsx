import { FC, MouseEvent, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Chip from '@mui/material/Chip'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { EllipsisVertical, ExternalLink, Pencil, Trash2 } from 'lucide-react'

import { TypesArtifact } from '../../api/api'
import { ViewMode } from '../widgets/ViewModeToggle'
import CardGrid from '../widgets/CardGrid'
import SimpleTable from '../widgets/SimpleTable'

type Props = {
  artifacts: TypesArtifact[]
  mode: ViewMode
  onEdit: (artifact: TypesArtifact) => void
  onDelete: (artifact: TypesArtifact) => void
}

const ArtifactVisibilityBadge: FC<{ artifact: TypesArtifact }> = ({ artifact }) => (
  <Chip
    size="small"
    label={artifact.visibility === 'public' ? 'Public' : 'Project'}
    color={artifact.visibility === 'public' ? 'success' : 'default'}
    variant="outlined"
  />
)

const artifactURL = (artifact: TypesArtifact) => artifact.url || ''

const ArtifactsView: FC<Props> = ({ artifacts, mode, onEdit, onDelete }) => {
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null)
  const [currentArtifact, setCurrentArtifact] = useState<TypesArtifact>()
  const artifactKey = artifacts.map((artifact) => `${artifact.id}:${artifact.updated_at}`).join('|')

  const openArtifact = (artifact: TypesArtifact) => {
    const url = artifactURL(artifact)
    if (url) window.open(url, '_blank', 'noopener,noreferrer')
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

  const tableData = useMemo(() => artifacts.map((artifact) => ({
    id: artifact.id,
    _data: artifact,
    name: (
      <Typography variant="body2" sx={{ fontWeight: 600 }}>
        <a
          href={artifactURL(artifact)}
          target="_blank"
          rel="noreferrer"
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
    kind: <Typography variant="body2" color="text.secondary">{artifact.kind === 'spa' ? 'Static SPA' : 'HTML page'}</Typography>,
    visibility: <ArtifactVisibilityBadge artifact={artifact} />,
    version: <Typography variant="body2" color="text.secondary">v{artifact.active_version?.version ?? 1}</Typography>,
    updated: <Typography variant="body2" color="text.secondary">{artifact.updated_at ? new Date(artifact.updated_at).toLocaleString() : '—'}</Typography>,
  })), [artifactKey])

  const menu = (
    <Menu anchorEl={menuAnchor} open={!!menuAnchor} onClose={closeMenu}>
      <MenuItem onClick={(event) => {
        event.stopPropagation()
        if (currentArtifact) openArtifact(currentArtifact)
        closeMenu()
      }}>
        <ExternalLink size={20} />
        <Typography sx={{ ml: 1 }}>Open</Typography>
      </MenuItem>
      <MenuItem onClick={(event) => {
        event.stopPropagation()
        if (currentArtifact) onEdit(currentArtifact)
        closeMenu()
      }}>
        <Pencil size={20} />
        <Typography sx={{ ml: 1 }}>Edit or publish version</Typography>
      </MenuItem>
      <MenuItem onClick={(event) => {
        event.stopPropagation()
        if (currentArtifact) onDelete(currentArtifact)
        closeMenu()
      }}>
        <Trash2 size={20} />
        <Typography sx={{ ml: 1 }}>Delete</Typography>
      </MenuItem>
    </Menu>
  )

  if (mode === 'table') {
    return (
      <>
        <SimpleTable
          authenticated
          fields={[
            { name: 'name', title: 'Name' },
            { name: 'kind', title: 'Type' },
            { name: 'visibility', title: 'Visibility' },
            { name: 'version', title: 'Version' },
            { name: 'updated', title: 'Updated' },
          ]}
          data={tableData}
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
        items={artifacts}
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
                  <Typography variant="body2" color="text.secondary">{artifact.kind === 'spa' ? 'Static SPA' : 'HTML page'}</Typography>
                </Box>
                <Stack direction="row" spacing={0.25} alignItems="center">
                  <ArtifactVisibilityBadge artifact={artifact} />
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
