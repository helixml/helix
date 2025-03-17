import React, { FC, useEffect, useState } from 'react'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import Typography from '@mui/material/Typography'

import ApiIntegrations from '../components/app/ApiIntegrations'
import APIKeysSection from '../components/app/APIKeysSection'
import AppSettings from '../components/app/AppSettings'
import CodeExamples from '../components/app/CodeExamples'
import DevelopersSection from '../components/app/DevelopersSection'
import GPTScriptsSection from '../components/app/GPTScriptsSection'
import GPTScriptEditor from '../components/app/GPTScriptEditor'
import KnowledgeEditor from '../components/app/KnowledgeEditor'
import PreviewPanel from '../components/app/PreviewPanel'
import ZapierIntegrations from '../components/app/ZapierIntegrations'
import Page from '../components/system/Page'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import SavingToast from '../components/widgets/SavingToast'
import { useEndpointProviders } from '../hooks/useEndpointProviders'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useApp from '../hooks/useApp'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useFilestore from '../hooks/useFilestore';
import AppLogsTable from '../components/app/AppLogsTable'
import IdeIntegrationSection from '../components/app/IdeIntegrationSection'

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

  const [ showBigSchema, setShowBigSchema ] = useState(false)
  const [ deletingAPIKey, setDeletingAPIKey ] = useState('')

  const [searchParams, setSearchParams] = useState(() => new URLSearchParams(window.location.search));
  const [isSearchMode, setIsSearchMode] = useState(() => searchParams.get('isSearchMode') === 'true');
  const [tabValue, setTabValue] = useState(() => searchParams.get('tab') || 'settings');

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
          title: appTools.flatApp?.name || 'App',
        }
      ]}
      topbarContent={(
        <Box sx={{ textAlign: 'right' }}>
          <Button
            sx={{ mr: 2 }}
            type="button"
            color="primary"
            variant="outlined"
            onClick={appTools.handleCopyEmbedCode}
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
              <Tab label="IDE" value="ide" />
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
                  {tabValue === 'settings' && appTools.flatApp && (
                    <AppSettings
                      id={appTools.id}
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
                        apis={appTools.apiTools}
                        onSaveApiTool={appTools.onSaveApiTool}
                        onDeleteApiTool={appTools.onDeleteApiTool}
                        isReadOnly={appTools.isReadOnly}
                      />

                      <ZapierIntegrations
                        zapier={appTools.zapierTools}
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
                        appTools.setEditingGptScript({
                          tool: {
                            name: '',
                            description: '',
                            content: '',
                          },
                          index: appTools.gptscriptsTools.length
                        });
                      }}
                      onEdit={(tool, index) => appTools.setEditingGptScript({tool, index})}
                      onDeleteGptScript={appTools.onDeleteGptScript}
                      isReadOnly={appTools.isReadOnly}
                      isGithubApp={appTools.isGithubApp}
                    />
                  )}

                  {tabValue === 'apikeys' && (
                    <APIKeysSection
                      apiKeys={account.appApiKeys}
                      onAddAPIKey={() => account.addAppAPIKey(appTools.id)}
                      onDeleteKey={(key) => setDeletingAPIKey(key)}
                      allowedDomains={appTools.flatApp?.allowedDomains || []}
                      setAllowedDomains={(allowedDomains) => appTools.saveFlatApp({allowedDomains})}
                      isReadOnly={appTools.isReadOnly}
                    />
                  )}

                  {tabValue === 'developers' && (
                    <DevelopersSection
                      schema={appTools.appSchema}
                      setSchema={appTools.setAppSchema}
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

                  {tabValue === 'ide' && (
                    <IdeIntegrationSection
                      appId={appTools.id}
                    />
                  )}
                </Box>
              </Grid>
              {/* For API keys section show  */}
              {tabValue === 'apikeys' ? (
                <CodeExamples apiKey={account.apiKeys[0]?.key || ''} />
              ) : (
                <PreviewPanel
                  loading={appTools.isInferenceLoading}
                  name={appTools.flatApp?.name || ''}
                  avatar={appTools.flatApp?.avatar || ''}
                  image={appTools.flatApp?.image || ''}
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
              await account.loadAppApiKeys(appTools.id)
              setDeletingAPIKey('')
            }}
            onCancel={() => {
              setDeletingAPIKey('')
            }}
          />
        )
      }

      {/* GPT Script Editor Modal */}
      <GPTScriptEditor
        editingGptScript={appTools.editingGptScript}
        setEditingGptScript={appTools.setEditingGptScript}
        onSaveGptScript={appTools.onSaveGptScript}
        showErrors={appTools.showErrors}
        isReadOnly={appTools.isReadOnly}
      />

    </Page>
  )
}

export default App
