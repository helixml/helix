import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'
import LockIcon from '@mui/icons-material/Lock'

import Page from '../components/system/Page'
import CreateAppWindow from '../components/apps/CreateAppWindow'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import AppsTable from '../components/apps/AppsTable'

import useApps from '../hooks/useApps'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import useApi from '../hooks/useApi'

import {
  IApp,
  SESSION_TYPE_TEXT,
} from '../types'

const Apps: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const api = useApi()
  const {
    params,
    setParams,
    removeParams,
    navigate,
  } = useRouter()

  const [ deletingApp, setDeletingApp ] = useState<IApp>()

  useEffect(() => {
    if (
      params.create === 'true' && 
      params.template && 
      params.provider_id && 
      params.oauth === 'true'
    ) {
      createOAuthApp(params.template, params.provider_id)
    }
  }, [params.create, params.template, params.provider_id, params.oauth])

  const createOAuthApp = async (templateId: string, providerId: string) => {
    try {
      const provider = await api.get(`/api/v1/oauth/providers/${providerId}`)
      if (!provider) {
        snackbar.error('Could not load OAuth provider')
        return
      }
      
      const defaultModel = account.models && account.models.length > 0 ? account.models[0].id : ''
      
      const appName = templateId.includes('github') ? 'GitHub Repository Analyzer' : 
                      templateId.includes('jira') ? 'Jira Project Manager' :
                      templateId.includes('slack') ? 'Slack Channel Assistant' :
                      templateId.includes('google') ? 'Google Drive Navigator' :
                      `${provider.name} Assistant`
                      
      const appDescription = templateId.includes('github') ? 'Analyze GitHub repositories, issues, and PRs' : 
                            templateId.includes('jira') ? 'Manage and analyze Jira projects and issues' :
                            templateId.includes('slack') ? 'Answer questions and perform tasks in Slack channels' :
                            templateId.includes('google') ? 'Search and summarize documents in Google Drive' :
                            `AI assistant that connects to your ${provider.name} account`
      
      const toolName = `${provider.name} API`
      const tool = {
        name: toolName,
        description: `Access ${provider.name} data and functionality`,
        tool_type: 'api' as const,
        config: {
          api: {
            url: provider.api_url || '',
            schema: '',
            oauth_provider: providerId,
            oauth_scopes: provider.default_scopes || []
          }
        }
      }
      
      const newApp = await apps.createApp('helix', {
        helix: {
          external_url: '',
          name: appName,
          description: appDescription,
          avatar: '',
          image: '',
          assistants: [{
            name: 'Default Assistant',
            description: appDescription,
            avatar: '',
            image: '',
            model: defaultModel,
            type: SESSION_TYPE_TEXT,
            system_prompt: `You are an AI assistant that connects to ${provider.name}. You can help users access their data and perform actions.`,
            apis: [{
              name: toolName,
              description: `Access ${provider.name} data and functionality`,
              url: provider.api_url || '',
              schema: '',
            }],
            gptscripts: [],
            tools: [],
            rag_source_id: '',
            lora_id: '',
            is_actionable_template: '',
          }],
        },
        secrets: {},
        allowed_domains: [],
      })
      
      if (!newApp) {
        snackbar.error('Failed to create app')
        return
      }
      
      removeParams(['create', 'template', 'provider_id', 'oauth'])
      navigate('app', { app_id: newApp.id })
      snackbar.success(`Created new ${provider.name} app`)
    } catch (err) {
      console.error('Error creating OAuth app:', err)
      snackbar.error('Failed to create app with OAuth integration')
    }
  }

  const onConnectRepo = useCallback(async (repo: string) => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    const newApp = await apps.createGithubApp(repo)
    if(!newApp) return false
    removeParams(['add_app'])
    snackbar.success('app created')
    apps.loadApps()
    navigate('app', {
      app_id: newApp.id,
    })
    return true
  }, [
    apps.createApp,
  ])

  const onEditApp = (app: IApp) => {
    account.orgNavigate('app', {
      app_id: app.id,
    })
  }

  const onDeleteApp = useCallback(async () => {
    if(!deletingApp) return
    const result = await apps.deleteApp(deletingApp.id)
    if(!result) return
    setDeletingApp(undefined)
    apps.loadApps()
    snackbar.success('app deleted')
  }, [
    deletingApp,
    apps.deleteApp,
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

  useEffect(() => {
    apps.loadApps()
  }, [
    apps.loadApps,
  ])

  return (
    <Page
      breadcrumbTitle="Apps"
      orgBreadcrumbs={ true }
      topbarContent={(
        <div>
          <Button
            id="secrets-button"
            variant="contained"
            color="secondary"
            endIcon={<LockIcon />}
            onClick={() => navigate('secrets')}
            sx={{ mr: 2 }}
          >
            Secrets
          </Button>

          <Button
            id="new-app-button"
            variant="contained"
            color="secondary"
            endIcon={<AddIcon />}
            onClick={apps.createOrgApp}
            sx={{ mr: 2 }}
          >
            New App
          </Button>
          <Button
            id="connect-repo-button"
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
            Connect Repo
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
          data={ apps.apps }
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
