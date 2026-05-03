import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import Typography from '@mui/material/Typography'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'
import DeleteIcon from '@mui/icons-material/DeleteOutline'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import SandboxOverviewTab from '../components/sandboxes/SandboxOverviewTab'
import SandboxCommandsTab from '../components/sandboxes/SandboxCommandsTab'
import SandboxFilesTab from '../components/sandboxes/SandboxFilesTab'
import SandboxTerminal from '../components/sandboxes/SandboxTerminal'
import SandboxStatusBadge from '../components/sandboxes/SandboxStatusBadge'

import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useSandbox, useDeleteSandbox } from '../services/sandboxesService'

const SandboxDetail: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const orgId = router.params.org_id as string | undefined
  const sandboxId = router.params.sandbox_id as string | undefined

  const [tab, setTab] = useState<'overview' | 'terminal' | 'commands' | 'files'>('overview')
  const [deleteOpen, setDeleteOpen] = useState(false)

  const { data: sandbox, isLoading } = useSandbox(orgId, sandboxId)
  const deleteMutation = useDeleteSandbox(orgId ?? '')

  const handleDelete = async () => {
    if (!sandboxId) return
    try {
      await deleteMutation.mutateAsync(sandboxId)
      snackbar.success('Sandbox deleted')
      router.navigate('org_sandboxes', { org_id: orgId })
    } catch {
      snackbar.error('Failed to delete sandbox')
    } finally {
      setDeleteOpen(false)
    }
  }

  const sandboxBreadcrumbs = [
    {
      title: 'Sandboxes',
      routeName: 'org_sandboxes',
      params: { org_id: orgId },
    },
  ]

  if (!sandboxId) {
    return (
      <Page
        breadcrumbs={sandboxBreadcrumbs}
        breadcrumbTitle="Sandbox"
        orgBreadcrumbs={true}
      >
        <Typography>Missing sandbox id</Typography>
      </Page>
    )
  }
  if (isLoading || !sandbox) {
    return (
      <Page
        breadcrumbs={sandboxBreadcrumbs}
        breadcrumbTitle="Sandbox"
        orgBreadcrumbs={true}
      >
        <LoadingSpinner />
      </Page>
    )
  }

  const running = sandbox.status === 'running'

  return (
    <Page
      breadcrumbs={sandboxBreadcrumbs}
      breadcrumbTitle={sandbox.name || sandbox.id}
      orgBreadcrumbs={true}
      topbarContent={(
        <Stack direction="row" spacing={1}>
          <Button
            variant="outlined"
            startIcon={<ArrowBackIcon />}
            onClick={() => router.navigate('org_sandboxes', { org_id: orgId })}
          >
            Back
          </Button>
          <Button
            variant="outlined"
            color="error"
            startIcon={<DeleteIcon />}
            onClick={() => setDeleteOpen(true)}
          >
            Delete
          </Button>
        </Stack>
      )}
    >
      <Container maxWidth="lg" sx={{ py: 3 }}>
        <Stack spacing={2}>
          <Box display="flex" alignItems="center" gap={2}>
            <Typography variant="h5" sx={{ fontFamily: 'monospace' }}>
              {sandbox.name || sandbox.id}
            </Typography>
            <SandboxStatusBadge status={sandbox.status} message={sandbox.status_message} />
          </Box>

          <Tabs value={tab} onChange={(_, v) => setTab(v)}>
            <Tab value="overview" label="Overview" />
            <Tab value="terminal" label="Terminal" />
            <Tab value="commands" label="Commands" />
            <Tab value="files" label="Files" />
          </Tabs>

          {tab === 'overview' && <SandboxOverviewTab sandbox={sandbox} />}
          {tab === 'terminal' && <SandboxTerminal orgId={orgId!} sandboxId={sandbox.id!} running={running} />}
          {tab === 'commands' && <SandboxCommandsTab orgId={orgId!} sandboxId={sandbox.id!} running={running} />}
          {tab === 'files' && <SandboxFilesTab orgId={orgId!} sandboxId={sandbox.id!} running={running} />}
        </Stack>
      </Container>

      {deleteOpen && (
        <DeleteConfirmWindow
          title={`Delete sandbox ${sandbox.name || sandbox.id}?`}
          onCancel={() => setDeleteOpen(false)}
          onSubmit={handleDelete}
        />
      )}
    </Page>
  )
}

export default SandboxDetail
