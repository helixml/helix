import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'

import Page from '../components/system/Page'
import CreateAppWindow from '../components/apps/CreateAppWindow'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import AppsTable from '../components/apps/AppsTable'

import useApps from '../hooks/useApps'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'

import {
  IApp,
} from '../types'

const Apps: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const {
    params,
    setParams,
    removeParams,
    navigate,
  } = useRouter()

  const [ deletingApp, setDeletingApp ] = useState<IApp>()

  const onConnectRepo = useCallback(async (repo: string) => {
    const newApp = await apps.createGithubApp(repo)
    if(!newApp) return false
    removeParams(['add_app'])
    snackbar.success('app created')
    apps.loadData()
    navigate('app', {
      app_id: newApp.id,
    })
    return true
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
    if(!account.user) return
    if(!params.add_app) return
    apps.loadGithubStatus(`${window.location.href}?add_app=true`)
  }, [
    account.user,
    params.add_app,
  ])

  useEffect(() => {
    if(!apps.githubStatus) return
    apps.loadGithubRepos()
  }, [
    apps.githubStatus,
  ])

  useEffect(() => {
    if(!params.snackbar_message) return
    snackbar.success(params.snackbar_message)
  }, [
    params.snackbar_message,
  ])

  return (
    <Page
      topbarTitle="Apps"
      topbarContent={(
        <div>
          <Button
              variant="contained"
              color="secondary"
              endIcon={<AddIcon />}
              onClick={ () => {
                if(!account.user) {
                  account.setShowLoginWindow(true)
                  return false
                }
                setParams({add_app: 'true'})
              }}
            >
              New App
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
          data={ apps.data }
          onEdit={ onEditApp }
          onDelete={ setDeletingApp }
        />
      </Container>
      {
        params.add_app && apps.githubStatus && (
          <CreateAppWindow
            githubStatus={ apps.githubStatus }
            githubRepos={ apps.githubRepos}
            githubReposLoading={ apps.githubReposLoading }
            onConnectRepo={ onConnectRepo }
            onCancel={ () => removeParams(['add_app']) }
            onLoadRepos={ apps.loadGithubRepos }
            connectLoading= { apps.connectLoading }
            connectError= { apps.connectError }
          />
        )
      }
      {
        deletingApp && (
          <DeleteConfirmWindow
            title="this app"
            onCancel={ () => setDeletingApp(undefined) }
            onSubmit={ onDeleteApp }
          />
        )
      }
    </Page>
  )
}

export default Apps