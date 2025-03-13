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

import { v4 as uuidv4 } from 'uuid'
import { parse as parseYaml, stringify as stringifyYaml } from 'yaml'

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
  removeEmptyValues,
  getAppFlatState,
  validateApp,
} from '../utils/app'

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
} from '../types'

const App: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const endpointProviders = useEndpointProviders()
  const api = useApi()
  const snackbar = useSnackbar()
  const session = useSession()
  const filestore = useFilestore();
  const {
    params,
    navigate,
  } = useRouter()

  const appTools = useApp(params.app_id)

  // this is the value used for the preview inference
  const [ inputValue, setInputValue ] = useState('')

  // TODO: use this from appTools
  const [ hasInitialised, setHasInitialised ] = useState(false)

  const [ secrets, setSecrets ] = useState<Record<string, string>>({})
  const [ allowedDomains, setAllowedDomains ] = useState<string[]>([])
  const [ schema, setSchema ] = useState('')
  const [ showErrors, setShowErrors ] = useState(false)
  const [ showBigSchema, setShowBigSchema ] = useState(false)
  const [ hasLoaded, setHasLoaded ] = useState(false)
  const [ deletingAPIKey, setDeletingAPIKey ] = useState('')

  const [app, setApp] = useState<IApp | null>(null);
  const [isNewApp, setIsNewApp] = useState(false);

  const [searchParams, setSearchParams] = useState(() => new URLSearchParams(window.location.search));
  const [isSearchMode, setIsSearchMode] = useState(() => searchParams.get('isSearchMode') === 'true');
  const [tabValue, setTabValue] = useState(() => searchParams.get('tab') || 'settings');

  const themeConfig = useThemeConfig()

  
  const [knowledgeSources, setKnowledgeSources] = useState<IKnowledgeSource[]>([]);

  

  const [knowledgeErrors, setKnowledgeErrors] = useState<boolean>(false);

  

  const [knowledgeList, setKnowledgeList] = useState<IKnowledgeSource[]>([]);
  const fetchKnowledgeTimeoutRef = useRef<NodeJS.Timeout | null>(null);
  const lastFetchTimeRef = useRef<number>(0);

  const [searchResults, setSearchResults] = useState<IKnowledgeSearchResult[]>([]);

  const [hasKnowledgeSources, setHasKnowledgeSources] = useState(true);
  const [loading, setLoading] = useState(false);

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
    if (!app) {
      snackbar.error('We have no app to launch')
      return
    }
    navigate('new', { app_id: app.id })
  }

  const isReadOnly = useMemo(() => {
    return app?.app_source === APP_SOURCE_GITHUB && !isNewApp;
  }, [app, isNewApp]);

  const readOnly = useMemo(() => {
    if(!app) return true
    return false
  }, [
    app,
  ])

  const sessionID = useMemo(() => {
    return session.data?.id || ''
  }, [
    session.data,
  ])


  // TODO: remove the need for duplicate websocket connections, currently this is used for knowing when the interaction has finished
  useWebsocket(sessionID, (parsedData) => {
    if(parsedData.type === WEBSOCKET_EVENT_TYPE_SESSION_UPDATE && parsedData.session) {
      const newSession: ISession = parsedData.session
      console.debug(`[${new Date().toISOString()}] App.tsx: Received session update via WebSocket:`, {
        sessionId: newSession.id,
        documentIds: newSession.config.document_ids,
        documentGroupId: newSession.config.document_group_id,
        parentApp: newSession.parent_app,
        hasDocumentIds: newSession.config.document_ids !== null && 
                      Object.keys(newSession.config.document_ids || {}).length > 0,
        documentIdKeys: Object.keys(newSession.config.document_ids || {}),
        documentIdValues: Object.values(newSession.config.document_ids || {}),
        sessionData: JSON.stringify(newSession)
      })
      session.setData(newSession)
    }
  })

  const isGithubApp = useMemo(() => app?.app_source === APP_SOURCE_GITHUB, [app]); 

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


  const assistants = app?.config.helix.assistants || []
  const apiAssistants = assistants.length > 0 ? assistants[0].apis || [] : []
  const zapierAssistants = assistants.length > 0 ? assistants[0].zapier || [] : []
  const gptscriptsAssistants = assistants.length > 0 ? assistants[0].gptscripts || [] : []

  useEffect(() => {
    if(!account.user) return
    endpointProviders.loadData()
    if(params.app_id) {
      account.loadApiKeys({
        types: 'app',
        app_id: params.app_id,
      })
    }
  }, [account.user])

  if(!account.user) return null
  if(!app) return null
  if(!hasLoaded && params.app_id !== "new") return null
  
  return (
    <Page
      showDrawerButton={false}
      breadcrumbs={[
        {
          title: 'Apps',
          routeName: 'apps'
        },
        {
          title: app.config.helix.name || 'App',
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
            disabled={account.apiKeys.length === 0 || isReadOnly}
          >
            Embed
          </Button>
          <Button
            type="button"
            color="secondary"
            variant="contained"
            onClick={handleLaunch}
            disabled={app.id === 'new'}
          >
            Launch
          </Button>
        </Box>
      )}
    >
      <Container maxWidth="xl" sx={{ height: 'calc(100% - 100px)' }}>
        <Box sx={{ height: 'calc(100vh - 100px)', width: '100%', flexGrow: 1, p: 2 }}>
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
                      readOnly={readOnly}
                      showErrors={showErrors}
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
                        disabled={isReadOnly}
                        knowledgeList={knowledgeList}
                        appId={app.id}
                        onRequestSave={async () => {
                          console.log('--------------------------------------------')
                          console.log('run onSave()')
                        }}
                      />
                      {knowledgeErrors && showErrors && (
                        <Alert severity="error" sx={{ mt: 2 }}>
                          Please specify at least one URL for each knowledge source.
                        </Alert>
                      )}
                    </Box>
                  )}

                  {tabValue === 'integrations' && (
                    <>
                      <ApiIntegrations
                        apis={apiAssistants}
                        onSaveApiTool={appTools.onSaveApiTool}
                        onDeleteApiTool={appTools.onDeleteApiTool}
                        isReadOnly={isReadOnly}
                      />

                      <ZapierIntegrations
                        zapier={zapierAssistants}
                        onSaveZapierTool={appTools.onSaveZapierTool}
                        onDeleteZapierTool={appTools.onDeleteZapierTool}
                        isReadOnly={isReadOnly}
                      />
                    </>
                  )}

                  {tabValue === 'gptscripts' && (
                    <GPTScriptsSection
                      app={app}
                      onAddGptScript={() => {
                        const newScript: IAssistantGPTScript = {
                          name: '',
                          description: '',
                          content: '',
                        };
                        setEditingGptScript({
                          tool: newScript,
                          index: gptscriptsAssistants.length
                        });
                      }}
                      onEdit={(tool, index) => setEditingGptScript({tool, index})}
                      onDeleteGptScript={appTools.onDeleteGptScript}
                      isReadOnly={isReadOnly}
                      isGithubApp={isGithubApp}
                    />
                  )}

                  {tabValue === 'apikeys' && (
                    <APIKeysSection
                      apiKeys={account.apiKeys}
                      onAddAPIKey={() => account.addAppAPIKey(app.id)}
                      onDeleteKey={(key) => setDeletingAPIKey(key)}
                      allowedDomains={allowedDomains}
                      setAllowedDomains={setAllowedDomains}
                      isReadOnly={isReadOnly}
                      readOnly={readOnly}
                    />
                  )}

                  {tabValue === 'developers' && (
                    <DevelopersSection
                      schema={schema}
                      setSchema={setSchema}
                      showErrors={showErrors}
                      appId={app.id}
                      navigate={navigate}
                    />
                  )}

                  {tabValue === 'logs' && (
                    <Box sx={{ mt: 2 }}>
                      <AppLogsTable appId={app.id} />
                    </Box>
                  )}
                </Box>
              </Grid>
              {/* For API keys section show  */}
              {tabValue === 'apikeys' ? (
                <CodeExamples apiKey={account.apiKeys[0]?.key || ''} />
              ) : (
                <PreviewPanel
                  loading={loading}
                  name={appTools.flatApp.name || ''}
                  avatar={appTools.flatApp.avatar || ''}
                  image={appTools.flatApp.image || ''}
                  isSearchMode={isSearchMode}
                  setIsSearchMode={setIsSearchMode}
                  inputValue={inputValue}
                  setInputValue={setInputValue}
                  onInference={appTools.onInference}
                  onSearch={appTools.onSearch}
                  hasKnowledgeSources={hasKnowledgeSources}
                  searchResults={searchResults}
                  session={session.data}
                  serverConfig={account.serverConfig}
                  themeConfig={themeConfig}
                  snackbar={snackbar}
                />
              )}
            </Grid>
          </Box>
        </Box>
      </Container>

      {/* Fixed bottom bar with save button */}
      {tabValue !== 'developers' && tabValue !== 'apikeys' && tabValue !== 'logs' && (
        <Box sx={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          borderTop: '1px solid #303047',
          bgcolor: 'background.paper',
          zIndex: 1000,
        }}>
          <Container maxWidth="xl">
            <Box sx={{ p: 2 }}>
              <Button
                type="button"
                color="secondary"
                variant="contained"
                onClick={async () => true/*onSave(false)*/}
                disabled={isReadOnly && !isGithubApp}
              >
                Save
              </Button>
            </Box>
          </Container>
        </Box>
      )}

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
                error={showErrors && !schema}
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                fullWidth
                multiline
                disabled
                label="App Configuration"
                helperText={showErrors && !schema ? "Please enter a schema" : ""}
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
                  error={showErrors && !editingGptScript.tool.name}
                  helperText={showErrors && !editingGptScript.tool.name ? 'Please enter a name' : ''}
                  disabled={isReadOnly}
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
                  error={showErrors && !editingGptScript.tool.description}
                  helperText={showErrors && !editingGptScript.tool.description ? "Description is required" : ""}
                  disabled={isReadOnly}
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
                  error={showErrors && !editingGptScript.tool.content}
                  helperText={showErrors && !editingGptScript.tool.content ? "Script content is required" : ""}
                  disabled={isReadOnly}
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
