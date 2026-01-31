import React, { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'

import Typography from '@mui/material/Typography'
import { useTheme } from '@mui/material/styles'


import APIKeysSection from '../components/app/APIKeysSection'
import AppSettings from '../components/app/AppSettings'
import AppearanceSettings from '../components/app/AppearanceSettings'
import AccessManagement from '../components/app/AccessManagement'
import CodeExamples from '../components/app/CodeExamples'
import DevelopersSection from '../components/app/DevelopersSection'
import KnowledgeEditor from '../components/app/KnowledgeEditor'
import TestsEditor from '../components/app/TestsEditor'
import PreviewPanel from '../components/app/PreviewPanel'
import Triggers from '../components/app/Triggers'
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
import AppUsage from '../components/app/AppUsage'
import IdeIntegrationSection from '../components/app/IdeIntegrationSection'
import useLightTheme from '../hooks/useLightTheme'
import Skills from '../components/app/Skills'
import MemoriesManagement from '../components/app/MemoriesManagement'

const App: FC = () => {
  const account = useAccount()  
  const api = useApi()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const theme = useTheme()
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
  
  // Get tab from URL params instead of local state
  const tabValue = params.tab || 'appearance';

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
          title: 'Agents',
          routeName: 'apps'
        },
        {
          title: appTools.flatApp?.name || 'Agent',
        }
      ]}
      topbarContent={(
        <Box sx={{ textAlign: 'right' }}>
          <Button
            type="button"
            color="secondary"
            variant="outlined"
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
          display: 'block',
        }}
      >
        <Box sx={{ width: '100%', pl: 2, pr: 2, mt: 2 }}>
          <Grid container>
            {/* Tab Content - Full Width */}
            <Grid item xs={12} sx={{
              backgroundColor: themeConfig.darkPanel,
              p: 0,
              mt: 2,
              mb: 2,
              borderRadius: 2,
              boxShadow: '0 4px 24px 0 rgba(0,0,0,0.12)',
            }}>
              <Box sx={{ width: '100%', p: 0, pl: 4 }}>
                <Grid container spacing={0}>
                  {tabValue === 'usage' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 0 }}>
                        <AppUsage appId={appTools.id} />
                      </Box>
                    </Grid>
                  ) : tabValue === 'skills' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 0 }}>
                        { appTools.flatApp && (
                          <Skills
                            app={appTools.flatApp}
                            onUpdate={appTools.saveFlatApp}
                          />
                        )}
                      </Box>
                    </Grid>
                  ) : tabValue === 'tests' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 0 }}>
                        { appTools.flatApp && (
                          <TestsEditor
                            app={appTools.flatApp}
                            onUpdate={appTools.saveFlatApp}
                            appId={appTools.id}
                            navigate={navigate}
                          />
                        )}
                      </Box>
                    </Grid>
                  ) : tabValue === 'memories' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 0 }}>
                        <MemoriesManagement 
                          appId={appTools.id} 
                          memory={appTools.flatApp?.memory || false}
                          onMemoryChange={(value) => appTools.saveFlatApp({ memory: value })}
                          readOnly={appTools.isReadOnly}
                        />
                      </Box>
                    </Grid>
                  ) : tabValue === 'developers' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 0 }}>
                        <DevelopersSection
                          schema={appTools.appSchema}
                          setSchema={appTools.setAppSchema}
                          showErrors={appTools.showErrors}
                          appId={appTools.id}
                          appName={appTools.flatApp?.name}
                          navigate={navigate}
                        />
                      </Box>
                    </Grid>
                  ) : tabValue === 'mcp' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 0 }}>
                        <IdeIntegrationSection
                          appId={appTools.id}
                        />
                      </Box>
                    </Grid>
                  ) : tabValue === 'triggers' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 0 }}>
                        <Triggers
                          app={appTools.flatApp || {}}
                          appId={appTools.id}
                          triggers={appTools.flatApp?.triggers || []}
                          onUpdate={(triggers) => appTools.saveFlatApp({ triggers })}
                          readOnly={isReadOnly}
                        />
                      </Box>
                    </Grid>                  
                  ) : tabValue === 'access' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: 2, mr: 3 }}>
                        <AccessManagement
                          appId={appTools.id}
                          accessGrants={appTools.accessGrants}
                          isLoading={false}
                          isReadOnly={isReadOnly}
                          onCreateGrant={appTools.createAccessGrant}
                          onDeleteGrant={appTools.deleteAccessGrant}
                        />
                      </Box>
                    </Grid>
                  ) : (
                    <>
                      <Grid item xs={12} md={6} sx={{
                        borderRight: '1px solid #303047',
                        overflow: 'auto',
                        pb: 8,
                        minHeight: 'calc(100vh - 120px)', // Ensure minimum height minus header
                        ...lightTheme.scrollbar
                      }}>
                        <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 0 }}>
                          {tabValue === 'appearance' && appTools.flatApp && (
                            <Box sx={{ height: '100%', overflow: 'auto' }}>
                              <AppearanceSettings
                                app={appTools.flatApp}
                                onUpdate={appTools.saveFlatApp}
                                readOnly={isReadOnly}
                                showErrors={appTools.showErrors}
                                id={appTools.id}
                              />
                            </Box>
                          )}

                          {tabValue === 'settings' && appTools.flatApp && (
                            <Box sx={{ 
                              height: 'calc(100vh - 200px)', 
                              overflow: 'auto', 
                              ...lightTheme.scrollbar,
                              pb: 4
                            }}>
                              <AppSettings
                                id={appTools.id}
                                app={appTools.flatApp}
                                onUpdate={appTools.saveFlatApp}
                                readOnly={isReadOnly}
                                showErrors={appTools.showErrors}
                                isAdmin={account.admin}
                              />
                            </Box>
                          )}

                          {tabValue === 'knowledge' && (
                            <Box sx={{ height: '100%', overflow: 'auto', mr: 2 }}>
                              <Typography variant="h6" sx={{ mb: 2, mt: 2 }}>
                                Knowledge Sources
                              </Typography>
                              <KnowledgeEditor
                                appId={appTools.id}
                                disabled={isReadOnly}
                                saveKnowledgeToApp={async (knowledge) => {
                                  await appTools.saveFlatApp({ knowledge })
                                  await appTools.loadServerKnowledge()
                                }}
                                onSaveApp={async () => {
                                  if (!appTools.app) return;
                                  return await appTools.saveApp(appTools.app);
                                }}
                              />
                            </Box>
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
                          onSessionUpdate={appTools.onSessionUpdate}
                        />
                      )}
                    </>
                  )}
                </Grid>
              </Box>
            </Grid>
          </Grid>
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
    </Page>
  )
}

export default App