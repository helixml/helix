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
import PageLoader from '../components/widgets/PageLoader'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import SandboxDesktopTab from '../components/sandboxes/SandboxDesktopTab'
import SandboxOverviewTab from '../components/sandboxes/SandboxOverviewTab'
import SandboxCommandsTab from '../components/sandboxes/SandboxCommandsTab'
import SandboxFilesTab from '../components/sandboxes/SandboxFilesTab'
import SandboxTerminal from '../components/sandboxes/SandboxTerminal'
import SandboxStatusBadge from '../components/sandboxes/SandboxStatusBadge'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useUrlTab from '../hooks/useUrlTab'
import { useSandbox, useDeleteSandbox } from '../services/sandboxesService'
import { TypesSandbox } from '../api/api'

// Tab ordering: terminal first for headless runtimes (it's the only useful
// interactive view), desktop first for desktop runtimes. Both come before the
// overview/commands/files supporting tabs.
const ALL_SANDBOX_TABS = ['desktop', 'terminal', 'overview', 'commands', 'files'] as const
type SandboxTab = (typeof ALL_SANDBOX_TABS)[number]

const hasDesktop = (runtime?: string): boolean =>
  !!runtime && !runtime.includes('headless')

interface LoadedProps {
  orgId: string | undefined
  orgSlug: string | undefined
  sandbox: TypesSandbox
  onDelete: () => void
  onBack: () => void
}

// SandboxDetailLoaded renders the page once the sandbox payload is available.
// Splitting it out lets useUrlTab pick the right default tab (desktop vs
// overview) on first render — initializers can't depend on data still loading.
// orgId is the actual org id (used for API calls); orgSlug is the URL-facing
// slug (used for breadcrumbs/navigation back to the list page).
const SandboxDetailLoaded: FC<LoadedProps> = ({ orgId, orgSlug, sandbox, onDelete, onBack }) => {
  const desktopAvailable = hasDesktop(sandbox.runtime)
  const validTabs = (desktopAvailable ? ALL_SANDBOX_TABS : ALL_SANDBOX_TABS.filter((t) => t !== 'desktop')) as readonly SandboxTab[]
  const [tab, setTab] = useUrlTab<SandboxTab>(
    'tab',
    validTabs,
    desktopAvailable ? 'desktop' : 'terminal',
  )

  const running = sandbox.status === 'running'

  const sandboxBreadcrumbs = [
    {
      title: 'Sandboxes',
      routeName: 'org_sandboxes',
      params: { org_id: orgSlug },
    },
  ]

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
            onClick={onBack}
          >
            Back
          </Button>
          <Button
            variant="outlined"
            color="error"
            startIcon={<DeleteIcon />}
            onClick={onDelete}
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

          <Tabs value={tab} onChange={(_, v) => setTab(v as SandboxTab)}>
            {desktopAvailable && <Tab value="desktop" label="Desktop" />}
            <Tab value="terminal" label="Terminal" />
            <Tab value="overview" label="Overview" />
            <Tab value="commands" label="Commands" />
            <Tab value="files" label="Files" />
          </Tabs>

          {tab === 'desktop' && desktopAvailable && <SandboxDesktopTab sandbox={sandbox} />}
          {tab === 'overview' && <SandboxOverviewTab orgId={orgId!} sandbox={sandbox} />}
          {tab === 'terminal' && <SandboxTerminal orgId={orgId!} sandboxId={sandbox.id!} running={running} />}
          {tab === 'commands' && <SandboxCommandsTab orgId={orgId!} sandboxId={sandbox.id!} running={running} />}
          {tab === 'files' && <SandboxFilesTab orgId={orgId!} sandboxId={sandbox.id!} running={running} persistent={sandbox.persistent} />}
        </Stack>
      </Container>
    </Page>
  )
}

const SandboxDetail: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const account = useAccount()
  // The URL carries the org slug; the API and the breadcrumb need different
  // values. orgSlug is for navigation (URLs), orgId is the actual organization
  // id used in API path params and stored on the sandbox row. Mixing these up
  // caused sandbox.OrganizationID to be set to the slug, which broke wallet
  // lookups (GetWalletByOrg("koala-bunny-corp") → not found) when billing or
  // delete tried to charge a final partial minute.
  const orgSlug = router.params.org_id as string | undefined
  const orgId = account.organizationTools.organization?.id
  const sandboxId = router.params.sandbox_id as string | undefined

  const [deleteOpen, setDeleteOpen] = useState(false)

  const { data: sandbox, isLoading } = useSandbox(orgId, sandboxId)
  const deleteMutation = useDeleteSandbox(orgId ?? '')

  const handleDelete = async () => {
    if (!sandboxId) return
    try {
      await deleteMutation.mutateAsync(sandboxId)
      snackbar.success('Sandbox deleted')
      router.navigate('org_sandboxes', { org_id: orgSlug })
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
      params: { org_id: orgSlug },
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
        <PageLoader message="Loading sandbox…" />
      </Page>
    )
  }

  return (
    <>
      <SandboxDetailLoaded
        orgId={orgId}
        orgSlug={orgSlug}
        sandbox={sandbox}
        onDelete={() => setDeleteOpen(true)}
        onBack={() => router.navigate('org_sandboxes', { org_id: orgSlug })}
      />
      {deleteOpen && (
        <DeleteConfirmWindow
          title={`Delete sandbox ${sandbox.name || sandbox.id}?`}
          onCancel={() => setDeleteOpen(false)}
          onSubmit={handleDelete}
        />
      )}
    </>
  )
}

export default SandboxDetail
