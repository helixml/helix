import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import ViewModeToggle from '../components/widgets/ViewModeToggle'
import CreateSandboxDialog from '../components/sandboxes/CreateSandboxDialog'
import SandboxesView from '../components/sandboxes/SandboxesView'

import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useViewMode from '../hooks/useViewMode'
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
  const [viewMode, setViewMode] = useViewMode('sandboxes-view-mode', 'table')

  const { data, isLoading } = useListSandboxes(orgId)
  const deleteMutation = useDeleteSandbox(orgId ?? '')

  const sandboxes = data?.sandboxes ?? []

  const handleOpen = (sb: TypesSandbox) => {
    if (!sb.id) return
    router.navigate('org_sandbox_detail', { org_id: orgId, sandbox_id: sb.id })
  }

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
      orgBreadcrumbs={true}
      topbarContent={(
        <Button
          variant="contained"
          color="secondary"
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
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary" gutterBottom>
                You don't have any sandboxes yet.
              </Typography>
              <Button
                variant="contained"
                color="secondary"
                startIcon={<AddIcon />}
                onClick={() => setCreateOpen(true)}
                sx={{ mt: 1 }}
              >
                Create your first sandbox
              </Button>
            </Box>
          ) : (
            <>
              <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <ViewModeToggle mode={viewMode} onChange={setViewMode} />
              </Box>
              <SandboxesView
                mode={viewMode}
                sandboxes={sandboxes}
                onOpen={handleOpen}
                onDelete={setDeleting}
              />
            </>
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
