import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import IconButton from '@mui/material/IconButton'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Table from '@mui/material/Table'
import TableBody from '@mui/material/TableBody'
import TableCell from '@mui/material/TableCell'
import TableContainer from '@mui/material/TableContainer'
import TableHead from '@mui/material/TableHead'
import TableRow from '@mui/material/TableRow'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/DeleteOutline'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import SandboxStatusBadge from '../components/sandboxes/SandboxStatusBadge'
import CreateSandboxDialog from '../components/sandboxes/CreateSandboxDialog'

import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  useListSandboxes,
  useDeleteSandbox,
} from '../services/sandboxesService'
import { TypesSandbox } from '../api/api'

// Sandboxes lists every sandbox in the current organization.
const Sandboxes: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const orgId = router.params.org_id as string | undefined

  const [createOpen, setCreateOpen] = useState(false)
  const [deleting, setDeleting] = useState<TypesSandbox | undefined>()

  const { data, isLoading } = useListSandboxes(orgId)
  const deleteMutation = useDeleteSandbox(orgId ?? '')

  const sandboxes = data?.sandboxes ?? []

  const handleDelete = async () => {
    if (!deleting?.id) return
    try {
      await deleteMutation.mutateAsync(deleting.id)
      snackbar.success(`Sandbox ${deleting.name || deleting.id} deleted`)
    } catch (e) {
      snackbar.error('Failed to delete sandbox')
    } finally {
      setDeleting(undefined)
    }
  }

  return (
    <Page
      breadcrumbTitle="Sandboxes"
      topbarContent={(
        <Button
          variant="contained"
          color="primary"
          startIcon={<AddIcon />}
          onClick={() => setCreateOpen(true)}
        >
          New Sandbox
        </Button>
      )}
    >
      <Container maxWidth="lg" sx={{ py: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Sandboxes</Typography>
            <Typography variant="body2" color="text.secondary">
              Ephemeral containers you can SSH into, run commands inside, and read/write files.
              Pinned at 1 vCPU / 2GB RAM. Nothing is persisted after deletion.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : sandboxes.length === 0 ? (
            <Paper sx={{ p: 4, textAlign: 'center' }}>
              <Typography variant="body1" color="text.secondary" gutterBottom>
                You don't have any sandboxes yet.
              </Typography>
              <Button
                variant="contained"
                color="primary"
                startIcon={<AddIcon />}
                onClick={() => setCreateOpen(true)}
              >
                Create your first sandbox
              </Button>
            </Paper>
          ) : (
            <TableContainer component={Paper}>
              <Table size="small">
                <TableHead>
                  <TableRow>
                    <TableCell>Name</TableCell>
                    <TableCell>Runtime</TableCell>
                    <TableCell>Status</TableCell>
                    <TableCell>Created</TableCell>
                    <TableCell>Expires</TableCell>
                    <TableCell align="right">Actions</TableCell>
                  </TableRow>
                </TableHead>
                <TableBody>
                  {sandboxes.map((sb) => (
                    <TableRow key={sb.id} hover>
                      <TableCell>
                        <Typography
                          variant="body2"
                          sx={{ cursor: 'pointer', fontFamily: 'monospace' }}
                          onClick={() => router.navigate('org_sandbox_detail', { org_id: orgId, sandbox_id: sb.id! })}
                        >
                          {sb.name || sb.id}
                        </Typography>
                      </TableCell>
                      <TableCell>{sb.runtime || 'ubuntu-desktop'}</TableCell>
                      <TableCell>
                        <SandboxStatusBadge status={sb.status} message={sb.status_message} />
                      </TableCell>
                      <TableCell>{sb.created_at ? new Date(sb.created_at).toLocaleString() : '-'}</TableCell>
                      <TableCell>{sb.expires_at ? new Date(sb.expires_at).toLocaleString() : '-'}</TableCell>
                      <TableCell align="right">
                        <Tooltip title="Open">
                          <IconButton
                            size="small"
                            onClick={() => router.navigate('org_sandbox_detail', { org_id: orgId, sandbox_id: sb.id! })}
                          >
                            <OpenInNewIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                        <Tooltip title="Delete">
                          <IconButton size="small" onClick={() => setDeleting(sb)}>
                            <DeleteIcon fontSize="small" />
                          </IconButton>
                        </Tooltip>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </TableContainer>
          )}
        </Stack>
      </Container>

      <CreateSandboxDialog
        open={createOpen}
        orgId={orgId ?? ''}
        onClose={() => setCreateOpen(false)}
        onCreated={(sandbox) => {
          setCreateOpen(false)
          snackbar.success(`Sandbox ${sandbox.name || sandbox.id} provisioning…`)
          router.navigate('org_sandbox_detail', { org_id: orgId, sandbox_id: sandbox.id! })
        }}
      />

      {deleting && (
        <DeleteConfirmWindow
          title={`Delete sandbox ${deleting.name || deleting.id}?`}
          onCancel={() => setDeleting(undefined)}
          onSubmit={handleDelete}
        />
      )}
    </Page>
  )
}

export default Sandboxes
