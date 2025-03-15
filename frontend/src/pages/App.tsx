import React, { FC, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import ApiIntegrations from '../components/app/ApiIntegrations'
import APIKeysSection from '../components/app/APIKeysSection'
import AppSettings from '../components/app/AppSettings'
import CodeExamples from '../components/app/CodeExamples'
import DevelopersSection from '../components/app/DevelopersSection'
import GPTScriptsSection from '../components/app/GPTScriptsSection'
import KnowledgeEditor from '../components/app/KnowledgeEditor'
import PreviewPanel from '../components/app/PreviewPanel'
import ZapierIntegrations from '../components/app/ZapierIntegrations'
import Page from '../components/system/Page'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import Window from '../components/widgets/Window'
import SavingToast from '../components/widgets/SavingToast'
import { useStreaming } from '../contexts/streaming'
import { useEndpointProviders } from '../hooks/useEndpointProviders'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useApps from '../hooks/useApps'
import useApp from '../hooks/useApp'
import useRouter from '../hooks/useRouter'
import useSession from '../hooks/useSession'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useWebsocket from '../hooks/useWebsocket'
import useFilestore from '../hooks/useFilestore';
import AppLogsTable from '../components/app/AppLogsTable'

import {
  APP_SOURCE_GITHUB,
  APP_SOURCE_HELIX,
  IApp,
  IAppFlatState,
  IAppUpdate,
  IAssistantApi,
  IAssistantGPTScript,
  IAssistantZapier,
  IFileStoreItem,
  IAssistantConfig,
  IKnowledgeSearchResult,
  IKnowledgeSource,
  ISession,
  SESSION_TYPE_TEXT,
  WEBSOCKET_EVENT_TYPE_SESSION_UPDATE,
  IOwnerType,
} from '../types'

const App: FC = () => {
  const account = useAccount()
  const endpointProviders = useEndpointProviders()
  const api = useApi()
  const snackbar = useSnackbar()
  const filestore = useFilestore()
  const themeConfig = useThemeConfig()
  const {
    params,
    navigate,
  } = useRouter()

  const appTools = useApp(params.app_id)

  const [ schema, setSchema ] = useState('')
  const [ showBigSchema, setShowBigSchema ] = useState(false)
  const [ deletingAPIKey, setDeletingAPIKey ] = useState('')

  const [searchParams, setSearchParams] = useState(() => new URLSearchParams(window.location.search));
  const [isSearchMode, setIsSearchMode] = useState(() => searchParams.get('isSearchMode') === 'true');
  const [tabValue, setTabValue] = useState(() => searchParams.get('tab') || 'settings');

  const [editingGptScript, setEditingGptScript] = useState<{
    tool: IAssistantGPTScript;
    index: number;
  } | null>(null);

  
  /**
   * Handles tab change in the app interface
   * @param event - React event
   * @param newValue - New tab value
   */
  const handleTabChange = (event: React.SyntheticEvent, newValue: string) => {
    setTabValue(newValue)
    
    // Update URL search params
    setSearchParams(prev => {
      const newParams = new URLSearchParams(prev.toString())
      newParams.set('tab', newValue)
      
      // Update URL without reload
      if (typeof window !== 'undefined') {
        window.history.replaceState({}, '', `${window.location.pathname}?${newParams}`)
      }
      
      return newParams
    })
  }
  
  /**
   * Launches the app - we assume the app has been saving we it's been edited
   */
  const handleLaunch = async () => {
    if (!appTools.app) {
      snackbar.error('We have no app to launch')
      return
    }
    navigate('new', { app_id: appTools.id })
  }

  const handleCopyEmbedCode = useCallback(() => {
    if (account.apiKeys.length > 0) {
      // TODO: remove model from embed code
      const embedCode = `<script src="https://cdn.jsdelivr.net/npm/@helixml/chat-embed"></script>
<script>
  ChatWidget({
    url: '${window.location.origin}/v1/chat/completions',
    model: 'llama3:instruct',
    bearerToken: '${account.apiKeys[0].key}',
  })
</script>`
      navigator.clipboard.writeText(embedCode).then(() => {
        snackbar.success('Embed code copied to clipboard');
      }, (err) => {
        console.error('Could not copy text: ', err);
        snackbar.error('Failed to copy embed code');
      });
    } else {
      snackbar.error('No API key available');
    }
  }, [account.apiKeys, snackbar]);  

  useEffect(() => {
    endpointProviders.loadData()
  }, [])

  if(!account.user) return null
  if(!appTools.app) return null

  return (
    <Page
      showDrawerButton={false}
      breadcrumbs={[
        {
          title: 'Apps',
          routeName: 'apps'
        },
        {
          title: appTools.flatApp.name || 'App',
        }
      ]}
      topbarContent={(
        <Box sx={{ textAlign: 'right' }}>
          <Button
            sx={{ mr: 2 }}
            type="button"
            color="primary"
            variant="outlined"
            onClick={handleCopyEmbedCode}
            startIcon={<ContentCopyIcon />}
            disabled={account.apiKeys.length === 0 || appTools.isReadOnly}
          >
            Embed
          </Button>
          <Button
            type="button"
            color="secondary"
            variant="contained"
            onClick={handleLaunch}
          >
            Launch
          </Button>
        </Box>
      )}
    >
      <Container
        maxWidth="xl"
        sx={{
          height: '100%',
        }}
      >
        <Box sx={{ height: '100%', width: '100%', flexGrow: 1, p: 2, pb: 0, mb: 0, }}>
          <Box>
            <Tabs value={tabValue} onChange={handleTabChange}>
              <Tab label="Settings" value="settings" />
              <Tab label="Knowledge" value="knowledge" />
              <Tab label="Integrations" value="integrations" />
              <Tab label="GPTScripts" value="gptscripts" />
              <Tab label="API Keys" value="apikeys" />
              <Tab label="Developers" value="developers" />
              <Tab label="Logs" value="logs" />
            </Tabs>
          </Box>
          <Box sx={{ height: 'calc(100% - 48px)', overflow: 'hidden' }}>
            <Grid container spacing={2} sx={{ height: '100%' }}>
              <Grid item sm={12} md={6} sx={{ 
                borderRight: '1px solid #303047',
                height: '100%',
                overflow: 'auto',
                pb: 8 // Add padding at bottom to prevent content being hidden behind fixed bar
              }}>
                <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                  {tabValue === 'settings' && (
                    <AppSettings
                      app={appTools.flatApp}
                      onUpdate={appTools.saveFlatApp}
                      readOnly={appTools.isReadOnly}
                      showErrors={appTools.showErrors}
                      isAdmin={account.admin}
                      providerEndpoints={endpointProviders.data}
                    />
                  )}

                  {tabValue === 'knowledge' && (
                    <Box sx={{ mt: 2 }}>
                      <Typography variant="h6" sx={{ mb: 2 }}>
                        Knowledge Sources
                      </Typography>
                      <KnowledgeEditor
                        knowledgeSources={appTools.knowledge}
                        onUpdate={appTools.handleKnowledgeUpdate}
                        onRefresh={appTools.handleRefreshKnowledge}
                        onCompletePreparation={appTools.handleCompleteKnowledgePreparation}
                        onUpload={appTools.handleFileUpload}
                        loadFiles={appTools.handleLoadFiles}
                        uploadProgress={filestore.uploadProgress}
                        disabled={appTools.isReadOnly}
                        appId={appTools.id}
                        onRequestSave={async () => {
                          console.log('Saving app state after file upload to trigger indexing');
                          // Save the current app state to ensure all knowledge changes are persisted
                          if (!appTools.app) {
                            console.warn('Cannot save app - app is null');
                            return;
                          }
                          return await appTools.saveApp(appTools.app);
                        }}
                      />
                      {appTools.knowledgeErrors && appTools.showErrors && (
                        <Alert severity="error" sx={{ mt: 2 }}>
                          Please specify at least one URL for each knowledge source.
                        </Alert>
                      )}
                    </Box>
                  )}

                  {tabValue === 'integrations' && (
                    <>
                      <ApiIntegrations
                        apis={appTools.apiAssistants}
                        onSaveApiTool={appTools.onSaveApiTool}
                        onDeleteApiTool={appTools.onDeleteApiTool}
                        isReadOnly={appTools.isReadOnly}
                      />

                      <ZapierIntegrations
                        zapier={appTools.zapierAssistants}
                        onSaveZapierTool={appTools.onSaveZapierTool}
                        onDeleteZapierTool={appTools.onDeleteZapierTool}
                        isReadOnly={appTools.isReadOnly}
                      />
                    </>
                  )}

                  {tabValue === 'gptscripts' && (
                    <GPTScriptsSection
                      app={appTools.app}
                      onAddGptScript={() => {
                        const newScript: IAssistantGPTScript = {
                          name: '',
                          description: '',
                          content: '',
                        };
                        setEditingGptScript({
                          tool: newScript,
                          index: appTools.gptscriptsAssistants.length
                        });
                      }}
                      onEdit={(tool, index) => setEditingGptScript({tool, index})}
                      onDeleteGptScript={appTools.onDeleteGptScript}
                      isReadOnly={appTools.isReadOnly}
                      isGithubApp={appTools.isGithubApp}
                    />
                  )}

                  {tabValue === 'apikeys' && (
                    <APIKeysSection
                      apiKeys={account.apiKeys}
                      onAddAPIKey={() => account.addAppAPIKey(appTools.id)}
                      onDeleteKey={(key) => setDeletingAPIKey(key)}
                      allowedDomains={appTools.flatApp.allowedDomains || []}
                      setAllowedDomains={(allowedDomains) => appTools.saveFlatApp({allowedDomains})}
                      isReadOnly={appTools.isReadOnly}
                    />
                  )}

                  {tabValue === 'developers' && (
                    <DevelopersSection
                      schema={schema}
                      setSchema={setSchema}
                      showErrors={appTools.showErrors}
                      appId={appTools.id}
                      navigate={navigate}
                    />
                  )}

                  {tabValue === 'logs' && (
                    <Box sx={{ mt: 2 }}>
                      <AppLogsTable appId={appTools.id} />
                    </Box>
                  )}
                </Box>
              </Grid>
              {/* For API keys section show  */}
              {tabValue === 'apikeys' ? (
                <CodeExamples apiKey={account.apiKeys[0]?.key || ''} />
              ) : (
                <PreviewPanel
                  loading={appTools.isInferenceLoading}
                  name={appTools.flatApp.name || ''}
                  avatar={appTools.flatApp.avatar || ''}
                  image={appTools.flatApp.image || ''}
                  isSearchMode={isSearchMode}
                  setIsSearchMode={setIsSearchMode}
                  inputValue={appTools.inputValue}
                  setInputValue={appTools.setInputValue}
                  onInference={appTools.onInference}
                  onSearch={appTools.onSearch}
                  hasKnowledgeSources={appTools.knowledge.length > 0}
                  searchResults={appTools.searchResults}
                  session={appTools.session.data}
                  serverConfig={account.serverConfig}
                  themeConfig={themeConfig}
                  snackbar={snackbar}
                />
              )}
            </Grid>
          </Box>
        </Box>
      </Container>

      {/* Toast notification for app saving */}
      <SavingToast isSaving={appTools.isAppSaving} />

      {
        showBigSchema && (
          <Window
            title="Schema"
            fullHeight
            size="lg"
            open
            withCancel
            cancelTitle="Close"
            onCancel={() => setShowBigSchema(false)}
          >
            <Box
              sx={{
                p: 2,
                height: '100%',
              }}
            >
              <TextField
                error={appTools.showErrors && !schema}
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                fullWidth
                multiline
                disabled
                label="App Configuration"
                helperText={appTools.showErrors && !schema ? "Please enter a schema" : ""}
                sx={{ height: '100%' }} // Set the height to '100%'
              />
            </Box>
          </Window>
        )
      }
      {
        deletingAPIKey && (
          <DeleteConfirmWindow
            title="this API key"
            onSubmit={async () => {
              const res = await api.delete(`/api/v1/api_keys`, {
                params: {
                  key: deletingAPIKey,
                },
              }, {
                snackbar: true,
              })
              if(!res) return
              snackbar.success('API Key deleted')
              account.loadApiKeys({
                types: 'app',
                app_id: params.app_id,
              })
              setDeletingAPIKey('')
            }}
            onCancel={() => {
              setDeletingAPIKey('')
            }}
          />
        )
      }

      {editingGptScript && (
        <Window
          title={`${editingGptScript.tool.name ? 'Edit' : 'Add'} GPTScript`}
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingGptScript(null)}
          onSubmit={() => {
            if (editingGptScript.tool) {
              appTools.onSaveGptScript(editingGptScript.tool, editingGptScript.index);
            }
          }}
        >
          <Box sx={{ p: 2 }}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              GPTScript
            </Typography>
            <Grid container spacing={2}>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.name}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, name: e.target.value }
                  } : null)}
                  label="Name"
                  fullWidth
                  error={appTools.showErrors && !editingGptScript.tool.name}
                  helperText={appTools.showErrors && !editingGptScript.tool.name ? 'Please enter a name' : ''}
                  disabled={appTools.isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.description}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, description: e.target.value }
                  } : null)}
                  label="Description"
                  fullWidth
                  error={appTools.showErrors && !editingGptScript.tool.description}
                  helperText={appTools.showErrors && !editingGptScript.tool.description ? "Description is required" : ""}
                  disabled={appTools.isReadOnly}
                />
              </Grid>
              <Grid item xs={12}>
                <TextField
                  value={editingGptScript.tool.content}
                  onChange={(e) => setEditingGptScript(prev => prev ? {
                    ...prev,
                    tool: { ...prev.tool, content: e.target.value }
                  } : null)}
                  label="Script Content"
                  fullWidth
                  multiline
                  rows={10}
                  error={appTools.showErrors && !editingGptScript.tool.content}
                  helperText={appTools.showErrors && !editingGptScript.tool.content ? "Script content is required" : ""}
                  disabled={appTools.isReadOnly}
                />
              </Grid>
            </Grid>
          </Box>
        </Window>
      )}
    </Page>
  )
}

export default App
