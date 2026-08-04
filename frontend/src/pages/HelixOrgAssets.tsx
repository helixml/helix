import { FC, MouseEvent, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import BlockOutlinedIcon from '@mui/icons-material/BlockOutlined'
import CheckCircleOutlineIcon from '@mui/icons-material/CheckCircleOutline'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import InfoOutlinedIcon from '@mui/icons-material/InfoOutlined'
import MoreVertIcon from '@mui/icons-material/MoreVert'

import AssetConfigDrawer from '../components/helix-org/AssetConfigDrawer'
import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import useSnackbar from '../hooks/useSnackbar'
import {
  AssetDTO,
  AssetHealthDTO,
  useAssetHealth,
  useDeleteAsset,
  useListAssets,
  useListHelixOrgBots,
  useUpdateAsset,
} from '../services/helixOrgService'

const AssetStatusBadge: FC<{ enabled: boolean; health?: AssetHealthDTO }> = ({ enabled, health }) => {
  if (!enabled) return <Chip size="small" label="Disabled" color="default" />
  if (!health) return <Chip size="small" label="Checking" color="default" />
  if (health.tcp_reachable && health.ssh_reachable) return <Chip size="small" label="Connected" color="success" />
  return <Chip size="small" label="Connection degraded" color="warning" />
}

const HelixOrgAssets: FC = () => {
  const snackbar = useSnackbar()
  const breadcrumbs = useHelixOrgBreadcrumbs()
  const { data: assets = [], isLoading } = useListAssets()
  const { data: agents = [] } = useListHelixOrgBots()
  const assetIDs = useMemo(() => assets.filter((asset) => asset.enabled !== false).map((asset) => asset.id ?? '').filter(Boolean), [assets])
  const health = useAssetHealth(assetIDs, { refetchInterval: 15000 })
  const deleteAsset = useDeleteAsset()
  const updateAsset = useUpdateAsset()

  const [drawerOpen, setDrawerOpen] = useState(false)
  const [editing, setEditing] = useState<AssetDTO>()
  const [deleting, setDeleting] = useState<AssetDTO>()
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const [anchorPosition, setAnchorPosition] = useState<{ top: number; left: number }>()
  const [currentAsset, setCurrentAsset] = useState<AssetDTO>()

  const openCreate = () => {
    setEditing(undefined)
    setDrawerOpen(true)
  }

  const openEdit = (asset: AssetDTO) => {
    setEditing(asset)
    setDrawerOpen(true)
  }

  const openMenu = (event: MouseEvent<HTMLElement>, asset: AssetDTO) => {
    event.stopPropagation()
    setAnchorPosition(undefined)
    setAnchorEl(event.currentTarget)
    setCurrentAsset(asset)
  }

  const openContextMenu = (event: MouseEvent<HTMLTableRowElement>, asset: AssetDTO) => {
    event.preventDefault()
    event.stopPropagation()
    setAnchorEl(null)
    setAnchorPosition({ top: event.clientY, left: event.clientX })
    setCurrentAsset(asset)
  }

  const closeMenu = () => {
    setAnchorEl(null)
    setAnchorPosition(undefined)
    setCurrentAsset(undefined)
  }

  const toggleAssetAccess = async (asset: AssetDTO) => {
    if (!asset.id) return
    const enabled = asset.enabled === false
    closeMenu()
    try {
      await updateAsset.mutateAsync({ id: asset.id, enabled })
      snackbar.success(`${asset.name ?? asset.id} ${enabled ? 'enabled' : 'disabled'}`)
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'Could not update asset access')
    }
  }

  const confirmDelete = async () => {
    if (!deleting?.id) return
    try {
      await deleteAsset.mutateAsync(deleting.id)
      snackbar.success(`deleted asset ${deleting.name ?? deleting.id}`)
      if (editing?.id === deleting.id) {
        setDrawerOpen(false)
        setEditing(undefined)
      }
    } catch (err: any) {
      snackbar.error(err?.response?.data?.error ?? err?.message ?? 'delete asset failed')
    } finally {
      setDeleting(undefined)
    }
  }

  const tableData = useMemo(() => assets.map((asset) => ({
    id: asset.id,
    _data: asset,
    name: (
      <Stack>
        <a
          href="#"
          onClick={(event) => {
            event.preventDefault()
            event.stopPropagation()
            openEdit(asset)
          }}
          style={{ fontWeight: 'bold', color: 'inherit', textDecoration: 'none' }}
        >
          {asset.name ?? asset.id}
        </a>
        {asset.description && (
          <Typography variant="caption" color="text.secondary">{asset.description}</Typography>
        )}
      </Stack>
    ),
    kind: <Typography variant="body2" color="text.secondary">Server</Typography>,
    endpoint: (
      <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
        {asset.server ? `${asset.server.user}@${asset.server.address}:${asset.server.port ?? 22}` : '—'}
      </Typography>
    ),
    status: <AssetStatusBadge enabled={asset.enabled !== false} health={asset.id ? health[asset.id] : undefined} />,
    agents: <Typography variant="body2" color="text.secondary">{asset.agent_ids?.length ?? 0}</Typography>,
    updated: (
      <Typography variant="body2" color="text.secondary">
        {asset.updated_at ? new Date(asset.updated_at).toLocaleString() : '—'}
      </Typography>
    ),
  })), [assets, health])

  const getActions = (row: Record<string, any>) => {
    const asset = row._data as AssetDTO
    return (
      <IconButton size="small" onClick={(event) => openMenu(event, asset)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <HelixOrgShell showChat={false} breadcrumbs={breadcrumbs} breadcrumbTitle="Assets">
      <Box sx={{ height: '100%', overflow: 'auto' }}>
        <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
          <Stack spacing={2}>
            <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
              <Typography variant="body2" color="text.secondary">
                Let AI agents troubleshoot and manage your servers at your command.
              </Typography>
              <Button variant="contained" color="secondary" startIcon={<AddIcon />} onClick={openCreate} sx={{ flexShrink: 0 }}>
                New asset
              </Button>
            </Stack>

            {isLoading ? (
              <LoadingSpinner />
            ) : assets.length === 0 ? (
              <Box sx={{ textAlign: 'center', py: 8 }}>
                <Typography variant="body1" color="text.secondary" gutterBottom>No assets yet.</Typography>
                <Button variant="contained" color="secondary" startIcon={<AddIcon />} onClick={openCreate} sx={{ mt: 1 }}>
                  New asset
                </Button>
              </Box>
            ) : (
              <SimpleTable
                authenticated
                fields={[
                  { name: 'name', title: 'Asset' },
                  { name: 'kind', title: 'Type' },
                  { name: 'endpoint', title: 'Server' },
                  { name: 'status', title: 'Status' },
                  { name: 'agents', title: 'Allowed agents' },
                  { name: 'updated', title: 'Updated' },
                ]}
                data={tableData}
                getActions={getActions}
                onRowContextMenu={(event, row) => openContextMenu(event, row._data as AssetDTO)}
              />
            )}
          </Stack>
        </Container>

        <Menu
          anchorEl={anchorEl}
          anchorReference={anchorPosition ? 'anchorPosition' : 'anchorEl'}
          anchorPosition={anchorPosition}
          open={Boolean(anchorEl || anchorPosition)}
          onClose={closeMenu}
        >
          <MenuItem
            onClick={(event) => {
              event.stopPropagation()
              const asset = currentAsset
              closeMenu()
              if (asset) openEdit(asset)
            }}
          >
            <InfoOutlinedIcon sx={{ mr: 1, fontSize: 20 }} />
            Details
          </MenuItem>
          <MenuItem
            disabled={updateAsset.isPending}
            onClick={(event) => {
              event.stopPropagation()
              const asset = currentAsset
              if (asset) void toggleAssetAccess(asset)
            }}
          >
            {currentAsset?.enabled === false
              ? <CheckCircleOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
              : <BlockOutlinedIcon sx={{ mr: 1, fontSize: 20 }} />}
            {currentAsset?.enabled === false ? 'Enable access' : 'Disable access'}
          </MenuItem>
          <MenuItem
            onClick={(event) => {
              event.stopPropagation()
              const asset = currentAsset
              closeMenu()
              if (asset) setDeleting(asset)
            }}
          >
            <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
            Delete
          </MenuItem>
        </Menu>

        {deleting && (
          <DeleteConfirmWindow title="asset" submitTitle="Delete" onSubmit={confirmDelete} onCancel={() => setDeleting(undefined)}>
            <Typography variant="body1">
              Deleting <b>{deleting.name ?? deleting.id}</b> revokes every agent link and removes its stored credentials.
            </Typography>
          </DeleteConfirmWindow>
        )}

        <AssetConfigDrawer
          open={drawerOpen}
          asset={editing}
          health={editing?.id ? health[editing.id] : undefined}
          agents={agents.filter((agent) => agent.kind !== 'human')}
          onClose={() => { setDrawerOpen(false); setEditing(undefined) }}
          onDelete={(id) => {
            const asset = assets.find((candidate) => candidate.id === id)
            if (asset) setDeleting(asset)
          }}
        />
      </Box>
    </HelixOrgShell>
  )
}

export default HelixOrgAssets
