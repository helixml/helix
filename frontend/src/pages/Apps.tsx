import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import Page from '../components/system/Page'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import AppsTable from '../components/apps/AppsTable'

import useApps from '../hooks/useApps'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useCreateBlankAgent from '../hooks/useCreateBlankAgent'
import useSubscriptionGate from '../hooks/useSubscriptionGate'
import Paywall from '../components/subscription/Paywall'
import HelixOrgTopNav from '../components/helix-org/HelixOrgTopNav'

import {
  IApp,
} from '../types'

const Apps: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const createBlankAgent = useCreateBlankAgent()
  const { paywallActive, navigateToBilling } = useSubscriptionGate()

  const {
    params,
  } = useRouter()

  const [ deletingApp, setDeletingApp ] = useState<IApp>()

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

  const onNewAgent = async () => {
    if(!checkLoginStatus()) return
    await createBlankAgent()
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
      organizationId={account.organizationTools.organization?.id}
      topbarContent={(
        <>
          <HelixOrgTopNav />
        </>
      )}
    >
      <Container
        maxWidth="xl"
        sx={{
          mb: 4,
          pt: 3,
        }}
      >
        <Stack spacing={2}>
          <Stack direction="row" justifyContent="space-between" alignItems="center" spacing={2}>
            <Typography variant="h5">Agents</Typography>
            <Button
              id="new-app-button"
              variant="contained"
              color="secondary"
              startIcon={<AddIcon />}
              onClick={onNewAgent}
            >
              New Agent
            </Button>
          </Stack>
          <Typography variant="body2" color="text.secondary">
            Agents in this org. Click an agent to edit instructions, tools and subscriptions.
          </Typography>
          <Paywall active={paywallActive} onBillingClick={navigateToBilling}>
            <AppsTable
              authenticated={ !!account.user }
              data={ apps.apps }
              onEdit={ onEditApp }
              onDelete={ setDeletingApp }
              orgId={ account.organizationTools.organization?.id || '' }
            />
          </Paywall>
        </Stack>
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
    </Page>
  )
}

export default Apps
