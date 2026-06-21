// HelixOrgRoles lists every role defined in the current org and is
// the entry point for the standalone "Roles" page in the helix-org
// middle-nav. Clicking a row navigates to /helix-org/roles/:role_id,
// which shows the full role markdown + per-role actions.
//
// The chart page (/helix-org/chart) still exists in parallel; the two
// surfaces both read the same backend `GET /roles` / `GET /roles/{id}`
// endpoints, so they stay in sync without extra plumbing.

import { FC, MouseEvent, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import useTheme from '@mui/material/styles/useTheme'
import AddIcon from '@mui/icons-material/Add'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'

import Page from '../components/system/Page'
import NewRoleDialog from '../components/helix-org/NewRoleDialog'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  RoleDTO,
  useDeleteHelixOrgRole,
  useListHelixOrgRoles,
  useListHelixOrgWorkers,
} from '../services/helixOrgService'

const HelixOrgRoles: FC = () => {
  const router = useRouter()
  const account = useAccount()
  const breadcrumbs = useHelixOrgBreadcrumbs()
  const snackbar = useSnackbar()
  const theme = useTheme()
  const orgSlug = router.params.org_id as string | undefined

  const { data, isLoading } = useListHelixOrgRoles()
  const { data: workersData } = useListHelixOrgWorkers()
  const deleteRole = useDeleteHelixOrgRole()

  const roles = data ?? []
  const [deleting, setDeleting] = useState<RoleDTO | undefined>()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentRole, setCurrentRole] = useState<RoleDTO | null>(null)
  const [newRoleOpen, setNewRoleOpen] = useState(false)

  const openRole = (roleId: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_role_detail', { org_id: orgSlug, role_id: roleId })
  }

  const handleMenuOpen = (e: MouseEvent<HTMLElement>, role: RoleDTO) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
    setCurrentRole(role)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentRole(null)
  }

  const handleDelete = async () => {
    if (!deleting) return
    try {
      await deleteRole.mutateAsync(deleting.id)
      snackbar.success(`deleted role ${deleting.id}`)
    } catch (e: any) {
      const status = e?.response?.status
      if (status === 409) {
        snackbar.error('owner role is protected and cannot be deleted')
      } else {
        snackbar.error(e?.response?.data?.error ?? e?.message ?? 'delete failed')
      }
    } finally {
      setDeleting(undefined)
    }
  }

  const tableData = useMemo(() => roles.map((r) => ({
    id: r.id,
    _data: r,
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
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); openRole(r.id) }}
        >
          {r.id}
        </a>
      </Typography>
    ),
    contentPreview: (
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
        {(r.content || '').split('\n')[0].slice(0, 80) || '—'}
      </Typography>
    ),
    tools: (
      <Typography variant="body2" color="text.secondary">
        {r.tools?.length ?? 0}
      </Typography>
    ),
    streams: (
      <Typography variant="body2" color="text.secondary">
        {r.streams?.length ?? 0}
      </Typography>
    ),
    updated: (
      <Typography variant="body2" color="text.secondary">
        {r.updated_at ? new Date(r.updated_at).toLocaleString() : '—'}
      </Typography>
    ),
  })), [roles, theme])

  const getActions = (row: any) => {
    const role = row._data as RoleDTO
    return (
      <IconButton size="small" onClick={(e) => handleMenuOpen(e, role)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <Page
      breadcrumbTitle="Roles"
      breadcrumbs={breadcrumbs}
      organizationId={account.organizationTools.organization?.id}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
            <Box sx={{ flex: 1 }}>
              <Typography variant="h5" sx={{ mb: 1 }}>Roles</Typography>
              <Typography variant="body2" color="text.secondary">
                A Role defines a job description: the markdown content tells a Worker what they're for, the
                tools list is the Worker's MCP tool surface, and the streams list flags which inbound events the Role's
                prompt expects. Workers hold a Role directly — tools and prompt come from here.
              </Typography>
            </Box>
            <Button
              variant="contained"
              startIcon={<AddIcon />}
              onClick={() => setNewRoleOpen(true)}
              sx={{ flexShrink: 0, mt: 0.5 }}
            >
              New Role
            </Button>
          </Stack>

          {isLoading ? (
            <LoadingSpinner />
          ) : roles.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary" gutterBottom>
                No roles defined yet.
              </Typography>
              <Button
                variant="contained"
                startIcon={<AddIcon />}
                onClick={() => setNewRoleOpen(true)}
                sx={{ mt: 1 }}
              >
                New Role
              </Button>
            </Box>
          ) : (
            <SimpleTable
              authenticated={true}
              fields={[
                { name: 'name', title: 'ID' },
                { name: 'contentPreview', title: 'Content' },
                { name: 'tools', title: 'Tools' },
                { name: 'streams', title: 'Streams' },
                { name: 'updated', title: 'Updated' },
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
            if (currentRole) openRole(currentRole.id)
          }}
        >
          <OpenInNewIcon sx={{ mr: 1, fontSize: 20 }} />
          Open
        </MenuItem>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentRole) setDeleting(currentRole)
          }}
        >
          <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>

      {deleting && (() => {
        const affected = (workersData ?? []).filter((w) => w.role_id === deleting.id)
        return (
          <DeleteConfirmWindow
            title="role"
            submitTitle="Delete"
            onSubmit={handleDelete}
            onCancel={() => setDeleting(undefined)}
          >
            <Typography variant="body1" gutterBottom>
              Deleting role <b style={{ fontFamily: 'monospace' }}>{deleting.id}</b> will fire{' '}
              {affected.length === 0
                ? 'no workers (role is unoccupied)'
                : affected.length === 1
                ? 'the following worker:'
                : `the following ${affected.length} workers:`}
            </Typography>
            {affected.length > 0 && (
              <Typography variant="body2" sx={{ fontFamily: 'monospace', pl: 1 }}>
                {affected.map((w) => w.id).join(', ')}
              </Typography>
            )}
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
              This is irreversible.
            </Typography>
          </DeleteConfirmWindow>
        )
      })()}

      <NewRoleDialog open={newRoleOpen} onClose={() => setNewRoleOpen(false)} />
    </Page>
  )
}

export default HelixOrgRoles
