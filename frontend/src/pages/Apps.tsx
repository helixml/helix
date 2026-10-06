import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import { Plus } from 'lucide-react'

import Page from '../components/system/Page'
import PageSectionHeader from '../components/system/PageSectionHeader'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import AppsTable from '../components/apps/AppsTable'
import NewAgentDialog from '../components/agent/NewAgentDialog'

import useApps from '../hooks/useApps'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useSubscriptionGate from '../hooks/useSubscriptionGate'
import Paywall from '../components/subscription/Paywall'
import HelixOrgTopNav from '../components/helix-org/HelixOrgTopNav'

import {
  AGENT_KIND_HELIX,
  IApp,
} from '../types'

// The kind a "New Agent" starts on. Agents are no longer split into per-kind
// tabs — the harness column tells them apart — so this is only a dialog seed.
export const DEFAULT_AGENT_KIND = AGENT_KIND_HELIX

const Apps: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const { paywallActive, navigateToBilling } = useSubscriptionGate()

  const {
    params,
  } = useRouter()

  const [ deletingApp, setDeletingApp ] = useState<IApp>()
  const [newAgentOpen, setNewAgentOpen] = useState(false)

  const onEditApp = (app: IApp) => {
    account.orgNavigate('agent', {
      app_id: app.id,
    })
  }

  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }

  const onNewAgent = () => {
    if(!checkLoginStatus()) return
    setNewAgentOpen(true)
  }

  const onAgentCreated = (kind: string, id: string) => {
    setNewAgentOpen(false)
    snackbar.success('Agent created')
    if (kind === AGENT_KIND_HELIX) {
      account.orgNavigate('agent', { app_id: id })
      return
    }
    apps.loadApps()
  }

  const onDeleteApp = useCallback(async () => {
    if(!deletingApp) return
    const result = await apps.deleteApp(deletingApp.id)
    if(!result) return
    setDeletingApp(undefined)
    apps.loadApps()
    snackbar.success('Agent deleted')
  }, [
    deletingApp,
    apps.deleteApp,
  ])  

  useEffect(() => {
    if(!params.snackbar_message) return
    snackbar.success(params.snackbar_message)
  }, [
    params.snackbar_message,
  ])

  useEffect(() => {
    if(account.user) {
      apps.loadApps()
    }
  }, [
    account, apps.loadApps,
  ])
  
  return (
    <Page
      breadcrumbTitle="Agents"
      orgBreadcrumbs={ true }
      globalSearch={true}
      notifications={true}
      organizationId={account.organizationTools.organization?.id}
      topbarContent={(
        <>
          <HelixOrgTopNav />
        </>
      )}
    >
      <Container
        maxWidth="lg"
        sx={{
          mb: 4,
          mt: 4,
        }}
      >
        <PageSectionHeader
          title="Agents"
          description="Agents in this org. Click an agent to edit instructions, tools and subscriptions."
          action={
            <Button
              id="new-app-button"
              variant="contained"
              color="secondary"
              size="small"
              startIcon={<Plus size={18} />}
              onClick={onNewAgent}
            >
              New Agent
            </Button>
          }
        />
        <Paywall active={paywallActive} onBillingClick={navigateToBilling}>
          <AppsTable
            authenticated={ !!account.user }
            data={ apps.apps }
            onEdit={ onEditApp }
            onDelete={ setDeletingApp }
            orgId={ account.organizationTools.organization?.id || '' }
          />
        </Paywall>
      </Container>
      {
        deletingApp && (
          <DeleteConfirmWindow
            title="this agent"
            onCancel={ () => setDeletingApp(undefined) }
            onSubmit={ onDeleteApp }
          />
        )
      }
      <NewAgentDialog
        open={newAgentOpen}
        initialKind={DEFAULT_AGENT_KIND}
        onClose={() => setNewAgentOpen(false)}
        onCreated={onAgentCreated}
      />
    </Page>
  )
}

export default Apps
