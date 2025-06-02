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
import AccessManagement from '../components/app/AccessManagement'
import CodeExamples from '../components/app/CodeExamples'
import DevelopersSection from '../components/app/DevelopersSection'
import GPTScriptsSection from '../components/app/GPTScriptsSection'
import GPTScriptEditor from '../components/app/GPTScriptEditor'
import KnowledgeEditor from '../components/app/KnowledgeEditor'
import PreviewPanel from '../components/app/PreviewPanel'
import ZapierIntegrations from '../components/app/ZapierIntegrations'
import Page from '../components/system/Page'
import AccessDenied from '../components/system/AccessDenied'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import SavingToast from '../components/widgets/SavingToast'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useApp from '../hooks/useApp'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useThemeConfig from '../hooks/useThemeConfig'
import useFilestore from '../hooks/useFilestore';
import AppUsage from '../components/app/AppUsage'
import IdeIntegrationSection from '../components/app/IdeIntegrationSection'
import useLightTheme from '../hooks/useLightTheme'

const App: FC = () => {
  const account = useAccount()  
  const api = useApi()
  const snackbar = useSnackbar()
  const filestore = useFilestore()
  const themeConfig = useThemeConfig()
  const {
    params,
    navigate,
  } = useRouter()

  const appTools = useApp(params.app_id)
  // Get user access information from appTools
  const { userAccess } = appTools

  const lightTheme = useLightTheme()

  const [deletingAPIKey, setDeletingAPIKey] = useState('')
  const [isAccessDenied, setIsAccessDenied] = useState(false)

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
    account.orgNavigate('new', { app_id: appTools.id, resource_type: 'apps' })
  }

  useEffect(() => {
    const checkAccess = async () => {
      try {
        const result = await api.getApiClient().v1AppsDetail(params.app_id)        
        if (!result) {
          setIsAccessDenied(true)
        }
      } catch (error: any) {
        if (error.response?.status === 403) {
          setIsAccessDenied(true)
        }
      }
    }
    if (account.user) {
      checkAccess()
    }
  }, [account.user, params.app_id])

  if (!account.user) return null
  if (isAccessDenied) return <AccessDenied />
  if (!appTools.app) return null

  const isReadOnly = appTools.isReadOnly || !appTools.isSafeToSave

  return (
    <Page
      showDrawerButton={false}
      orgBreadcrumbs={true}
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
            disabled={account.apiKeys.length === 0 || isReadOnly}
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
              <Tab label="Usage" value="usage" />
              {
                // Only show Access tab if user is an admin and app has an organization_id
                appTools.app?.organization_id && userAccess.isAdmin && (
                  <Tab label="Access" value="access" />
                )
              }
            </Tabs>
          </Box>
          <Box sx={{ height: 'calc(100% - 48px)', overflow: 'hidden' }}>
            <Grid container spacing={2} sx={{ height: '100%' }}>
              {tabValue === 'usage' ? (
                <Grid item xs={12} sx={{ height: '100%', overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                  <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                    <AppUsage appId={appTools.id} />
                  </Box>
                </Grid>
              ) : (
                <>
                  <Grid item sm={12} md={6} sx={{
                    borderRight: '1px solid #303047',
                    height: '100%',
                    overflow: 'auto',
                    pb: 8, // Add padding at bottom to prevent content being hidden behind fixed bar
                    ...lightTheme.scrollbar
                  }}>
                    <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                      {tabValue === 'settings' && appTools.flatApp && (
                        <AppSettings
                          id={appTools.id}
                          app={appTools.flatApp}
                          onUpdate={appTools.saveFlatApp}
                          readOnly={isReadOnly}
                          showErrors={appTools.showErrors}
                          isAdmin={account.admin}
                        />
                      )}

                      {tabValue === 'access' && (
                        <Box sx={{ mt: 2 }}>
                          <AccessManagement
                            appId={appTools.id}
                            accessGrants={appTools.accessGrants}
                            isLoading={false}
                            isReadOnly={isReadOnly}
                            onCreateGrant={appTools.createAccessGrant}
                            onDeleteGrant={appTools.deleteAccessGrant}
                          />
                        </Box>
                      )}

                      {tabValue === 'knowledge' && (
                        <Box sx={{ mt: 2 }}>
                          <Typography variant="h6" sx={{ mb: 2 }}>
                            Knowledge Sources
                          </Typography>
                          <KnowledgeEditor
                            appId={appTools.id}
                            disabled={isReadOnly}
                            saveKnowledgeToApp={async (knowledge) => {
                              // the knowledge has changed so we need to keep the app hook
                              // in sync so it knows about the knowledge IDs then we
                              // can use that for the preview panel
                              await appTools.saveFlatApp({
                                knowledge,
                              })
                              await appTools.loadServerKnowledge()
                            }}
                            onSaveApp={async () => {
                              console.log('Saving app state after file upload to trigger indexing');
                              // Save the current app state to ensure all knowledge changes are persisted
                              if (!appTools.app) {
                                console.warn('Cannot save app - app is null');
                                return;
                              }
                              return await appTools.saveApp(appTools.app);
                            }}
                          />
                        </Box>
                      )}

                      {tabValue === 'integrations' && appTools.flatApp && (
                        <>
                          <ApiIntegrations
                            apis={appTools.apiTools}
                            tools={appTools.apiToolsFromTools}
                            onSaveApiTool={appTools.onSaveApiTool}
                            onDeleteApiTool={appTools.onDeleteApiTool}
                            isReadOnly={isReadOnly}
                            app={appTools.flatApp}
                            onUpdate={appTools.saveFlatApp}
                          />

                          <ZapierIntegrations
                            zapier={appTools.zapierTools}
                            onSaveZapierTool={appTools.onSaveZapierTool}
                            onDeleteZapierTool={appTools.onDeleteZapierTool}
                            isReadOnly={isReadOnly}
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
                          onEdit={(tool, index) => appTools.setEditingGptScript({ tool, index })}
                          onDeleteGptScript={appTools.onDeleteGptScript}
                          isReadOnly={isReadOnly}
                          isGithubApp={appTools.isGithubApp}
                        />
                      )}

                      {tabValue === 'apikeys' && (
                        <APIKeysSection
                          apiKeys={account.appApiKeys}
                          onAddAPIKey={() => account.addAppAPIKey(appTools.id)}
                          onDeleteKey={(key) => setDeletingAPIKey(key)}
                          allowedDomains={appTools.flatApp?.allowedDomains || []}
                          setAllowedDomains={(allowedDomains) => appTools.saveFlatApp({ allowedDomains })}
                          isReadOnly={isReadOnly}
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

                      {tabValue === 'ide' && (
                        <IdeIntegrationSection
                          appId={appTools.id}
                        />
                      )}
                    </Box>
                  </Grid>
                  {/* For API keys section show  */}
                  {tabValue === 'apikeys' ? (
                    <CodeExamples apiKey={account.appApiKeys[0]?.key || ''} />
                  ) : (
                    <PreviewPanel
                      appId={appTools.id}
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
                      hasKnowledgeSources={(appTools.flatApp?.knowledge?.length || 0) > 0}
                      searchResults={appTools.searchResults}
                      session={appTools.session.data}
                      serverConfig={account.serverConfig}
                      themeConfig={themeConfig}
                      snackbar={snackbar}
                      conversationStarters={appTools.flatApp?.conversation_starters || []}
                    />
                  )}
                </>
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
              if (!res) return
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
        isReadOnly={isReadOnly}
      />

    </Page>
  )
}

export default App