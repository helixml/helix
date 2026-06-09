// HelixOrgWorkers lists every worker defined in the current org.
// Clicking a row navigates to /helix-org/workers/:worker_id, which
// shows the worker's identity, the role it holds, and a
// "Chat with this worker (new session)" button. The chart's worker
// chips also navigate to the same detail page.

import { FC, MouseEvent, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import useTheme from '@mui/material/styles/useTheme'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  WorkerDTO,
  useFireHelixOrgWorker,
  useListHelixOrgWorkers,
} from '../services/helixOrgService'

const OWNER_WORKER = 'w-owner'

const HelixOrgWorkers: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const snackbar = useSnackbar()
  const theme = useTheme()
  const orgSlug = router.params.org_id as string | undefined

  const { data, isLoading } = useListHelixOrgWorkers()
  const fire = useFireHelixOrgWorker()

  const workers = data ?? []
  const [firing, setFiring] = useState<WorkerDTO | undefined>()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentWorker, setCurrentWorker] = useState<WorkerDTO | null>(null)

  const openWorker = (workerId: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_worker_detail', { org_id: orgSlug, worker_id: workerId })
  }

  const handleMenuOpen = (e: MouseEvent<HTMLElement>, w: WorkerDTO) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
    setCurrentWorker(w)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentWorker(null)
  }

  const handleFire = async () => {
    if (!firing) return
    try {
      await fire.mutateAsync(firing.id)
      snackbar.success(`fired ${firing.id}`)
    } catch (e: any) {
      const status = e?.response?.status
      if (status === 409) {
        snackbar.error('owner worker is protected and cannot be fired')
      } else {
        snackbar.error(e?.response?.data?.error ?? e?.message ?? 'fire failed')
      }
    } finally {
      setFiring(undefined)
    }
  }

  const tableData = useMemo(() => workers.map((w) => ({
    id: w.id,
    _data: w,
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
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); openWorker(w.id) }}
        >
          {w.id}
        </a>
      </Typography>
    ),
    kind: (
      <Stack direction="row" alignItems="center" spacing={0.5}>
        {w.kind === 'ai' ? (
          <SmartToyOutlinedIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
        ) : (
          <PersonOutlineIcon sx={{ fontSize: 16, color: 'text.secondary' }} />
        )}
        <Typography variant="body2" color="text.secondary">{w.kind}</Typography>
      </Stack>
    ),
    role: (
      <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
        {w.role_id || '—'}
      </Typography>
    ),
    reportsTo: (
      <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
        {(w.parent_ids ?? []).join(', ') || '—'}
      </Typography>
    ),
    identityPreview: (
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          maxWidth: 360,
        }}
      >
        {(w.identity_content || '').split('\n').find((l) => l.trim() !== '')?.replace(/^#+\s*/, '').slice(0, 80) || '—'}
      </Typography>
    ),
    tools: (
      <Typography variant="body2" color="text.secondary">
        {w.tools?.length ?? 0}
      </Typography>
    ),
  })), [workers, theme])

  const getActions = (row: any) => {
    const w = row._data as WorkerDTO
    return (
      <IconButton size="small" onClick={(e) => handleMenuOpen(e, w)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <Page
      breadcrumbTitle="Workers"
      orgBreadcrumbs={true}
      organizationId={account.organizationTools.organization?.id}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Workers</Typography>
            <Typography variant="body2" color="text.secondary">
              Workers are the people and AI agents in the org. Each holds a Role
              (the source of their MCP capabilities) and reports to another
              Worker. Click a worker to open its detail page — chat to it in a
              fresh session, inspect its identity, manage its subscriptions.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : workers.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary" gutterBottom>
                No workers yet. Hire one from the chart.
              </Typography>
            </Box>
          ) : (
            <SimpleTable
              authenticated={true}
              fields={[
                { name: 'name', title: 'ID' },
                { name: 'kind', title: 'Kind' },
                { name: 'role', title: 'Role' },
                { name: 'reportsTo', title: 'Reports to' },
                { name: 'identityPreview', title: 'Identity' },
                { name: 'tools', title: 'Tools' },
              ]}
              data={tableData}
              getActions={getActions}
            />
          )}
        </Stack>
      </Container>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentWorker) openWorker(currentWorker.id)
          }}
        >
          <OpenInNewIcon sx={{ mr: 1, fontSize: 20 }} />
          Open
        </MenuItem>
        <MenuItem
          disabled={currentWorker?.id === OWNER_WORKER}
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentWorker) setFiring(currentWorker)
          }}
        >
          <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
          {currentWorker?.id === OWNER_WORKER ? 'Owner — protected' : 'Fire'}
        </MenuItem>
      </Menu>

      {firing && (
        <DeleteConfirmWindow
          title="worker"
          submitTitle="Fire"
          onSubmit={handleFire}
          onCancel={() => setFiring(undefined)}
        >
          <Typography variant="body1">
            Firing worker <b style={{ fontFamily: 'monospace' }}>{firing.id}</b> tears down its
            per-worker Helix project + agent app and clears its runtime state. This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}
    </Page>
  )
}

export default HelixOrgWorkers
