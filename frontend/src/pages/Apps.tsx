import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'

import CreateToolWindow from '../components/tools/CreateToolWindow'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import AppsTable from '../components/apps/AppsTable'
import useLayout from '../hooks/useLayout'
import useApps from '../hooks/useApps'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'

import {
  IAppType,
  IApp,
  APP_TYPE_GITHUB,
} from '../types'

const Apps: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const layout = useLayout()
  const snackbar = useSnackbar()
  const {
    navigate,
  } = useRouter()

  const [ addingApp, setAddingApp ] = useState(false)
  const [ deletingApp, setDeletingApp ] = useState<IApp>()

  const onCreateApp = useCallback(async () => {
    const newApp = await apps.createApp('', '', APP_TYPE_GITHUB, {
      
    })
    if(!newApp) return
    setAddingApp(false)
    snackbar.success('app created')
    navigate('app', {
      app_id: newApp.id,
    })
  }, [
    apps.createApp,
  ])

  const onEditApp = useCallback((app: IApp) => {
    navigate('app', {
      app_id: app.id,
    })
  }, [])

  const onDeleteApp = useCallback(async () => {
    if(!deletingApp) return
    const result = await apps.deleteApp(deletingApp.id)
    if(!result) return
    setDeletingApp(undefined)
    snackbar.success('app deleted')
  }, [
    deletingApp,
    apps.deleteApp,
  ])

  useEffect(() => {
    if(!account.user) return
    apps.loadData()
  }, [
    account.user,
  ])

  useEffect(() => {
    layout.setToolbarRenderer(() => () => {
      return (
        <div>
          <Button
              variant="contained"
              color="secondary"
              endIcon={<AddIcon />}
              onClick={ () => {
                // setAddingApiTool(true)
              }}
            >
              New App
          </Button>
        </div>
        
      )
    })

    return () => layout.setToolbarRenderer(undefined)
  }, [])

  if(!account.user) return null

  return (
    <>
      <Container
        maxWidth="xl"
        sx={{
          mt: 12,
          height: 'calc(100% - 100px)',
        }}
      >
        <AppsTable
          data={ apps.data }
          onEdit={ onEditApp }
          onDelete={ setDeletingApp }
        />
      </Container>
      {
        addingApp && (
          <CreateToolWindow
            onCreate={ onCreateApp }
            onCancel={ () => setAddingApp(false) }
          />
        )
      }
      {
        deletingApp && (
          <DeleteConfirmWindow
            title="this tool"
            onCancel={ () => setDeletingApp(undefined) }
            onSubmit={ onDeleteApp }
          />
        )
      }
    </>
  )
}

export default Apps