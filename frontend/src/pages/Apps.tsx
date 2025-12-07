import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'
import LockIcon from '@mui/icons-material/Lock'
import Box from '@mui/material/Box'

import Page from '../components/system/Page'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import AppsTable from '../components/apps/AppsTable'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'

import useApps from '../hooks/useApps'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useCreateBlankAgent from '../hooks/useCreateBlankAgent'

import {
  IApp,
} from '../types'

const Apps: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const createBlankAgent = useCreateBlankAgent()

  const {
    params,
    navigate,
  } = useRouter()

  const [ deletingApp, setDeletingApp ] = useState<IApp>()

  const onEditApp = (app: IApp) => {
    account.orgNavigate('app', {
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

  const onNewSecret = () => {
    if(!checkLoginStatus()) return

    account.orgNavigate('secrets')
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
      topbarContent={(
        <div>
          <Button
            id="secrets-button"
            variant="contained"
            color="secondary"
            endIcon={<LockIcon />}
            onClick={onNewSecret}
            sx={{ mr: 2 }}
          >
            Secrets
          </Button>

          <Button
            id="new-app-button"
            variant="contained"
            color="secondary"
            endIcon={<AddIcon />}
            onClick={onNewAgent}
            sx={{ mr: 2 }}
          >
            New Agent
          </Button>
        </div>
      )}
    >
      <Container
        maxWidth="xl"
        sx={{
          mb: 4,
        }}
      >
        <AppsTable
          authenticated={ !!account.user }
          data={ apps.apps }
          onEdit={ onEditApp }
          onDelete={ setDeletingApp }
          orgId={ account.organizationTools.organization?.id || '' }
        />
        
        {/* Find Agents CTA */}
        <Box
          sx={{
            display: 'flex',
            justifyContent: 'flex-start',
            mt: 6,
            mb: 2,
          }}
        >
          <LaunchpadCTAButton />
        </Box>
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
