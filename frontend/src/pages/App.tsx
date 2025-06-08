import React, { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import Typography from '@mui/material/Typography'
import SettingsIcon from '@mui/icons-material/Settings';
import MenuBookIcon from '@mui/icons-material/MenuBook';
import EmojiObjectsIcon from '@mui/icons-material/EmojiObjects';
import VpnKeyIcon from '@mui/icons-material/VpnKey';
import CodeIcon from '@mui/icons-material/Code';
import BarChartIcon from '@mui/icons-material/BarChart';
import CloudDownloadIcon from '@mui/icons-material/CloudDownload';
import GroupIcon from '@mui/icons-material/Group';

import APIKeysSection from '../components/app/APIKeysSection'
import AppSettings from '../components/app/AppSettings'
import AccessManagement from '../components/app/AccessManagement'
import CodeExamples from '../components/app/CodeExamples'
import DevelopersSection from '../components/app/DevelopersSection'
import KnowledgeEditor from '../components/app/KnowledgeEditor'
import PreviewPanel from '../components/app/PreviewPanel'
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

const App: FC = () => {
  const account = useAccount()  
  const api = useApi()
  const snackbar = useSnackbar()
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
        <Box sx={{ height: '100%', width: '100%', flexGrow: 1, p: 2, pb: 0, mb: 0 }}>
          <Grid container sx={{ height: '100%' }}>
            {/* Left: Vertical Tabs */}
            <Grid item xs={12} sm={3} md={2} sx={{ borderRight: '1px solid #303047', minHeight: '80vh', pt: 3 }}>
              <Tabs
                orientation="vertical"
                variant="scrollable"
                value={tabValue}
                onChange={handleTabChange}
                sx={{
                  borderRight: 0,
                  minWidth: 180,
                  alignItems: 'flex-start',
                  '.MuiTabs-flexContainer': { alignItems: 'flex-start' },
                  '.MuiTab-root': {
                    justifyContent: 'flex-start',
                    textAlign: 'left',
                    color: '#8a8a9e',
                    fontWeight: 400,
                    fontSize: '0.85rem',
                    minHeight: 36,
                    pl: 2,
                    pr: 2,
                    borderRadius: 2,
                    transition: 'color 0.2s',
                    '& .MuiTab-iconWrapper': {
                      fontSize: '1.1rem',
                      marginRight: 4,
                    },
                  },
                  '.Mui-selected': {
                    color: '#fff !important',
                    background: 'none',
                  },
                  '.MuiTabs-indicator': {
                    display: 'none',
                  },
                }}
              >
                <Tab icon={<SettingsIcon sx={{ mr: 0.5 }} />} iconPosition="start" label="Settings" value="settings" />
                <Tab icon={<MenuBookIcon sx={{ mr: 0.5 }} />} iconPosition="start" label="Knowledge" value="knowledge" />
                <Tab icon={<EmojiObjectsIcon sx={{ mr: 0.5 }} />} iconPosition="start" label="Skills" value="skills" />                
                <Tab icon={<VpnKeyIcon sx={{ mr: 0.5 }} />} iconPosition="start" label="API Keys" value="apikeys" />
                <Tab icon={<CodeIcon sx={{ mr: 0.5 }} />} iconPosition="start" label="IDE" value="ide" />
                <Tab icon={<BarChartIcon sx={{ mr: 0.5 }} />} iconPosition="start" label="Usage" value="usage" />
                <Tab icon={<CloudDownloadIcon sx={{ mr: 0.5 }} />} iconPosition="start" label="Export" value="developers" />
                {
                  appTools.app?.organization_id && userAccess.isAdmin && (
                    <Tab icon={<GroupIcon sx={{ mr: 0.5 }} />} iconPosition="start" label="Access" value="access" />
                  )
                }
              </Tabs>
            </Grid>
            {/* Right: Tab Content */}
            <Grid item xs={12} sm={9} md={10} sx={{ height: '100%', overflow: 'hidden', backgroundColor: 'rgba(255, 255, 255, 0.05)', p: 0 }}>
              <Box sx={{ height: '100%', width: '100%', p: 0, pl: 4 }}>
                <Grid container spacing={0} sx={{ height: '100%' }}>
                  {tabValue === 'usage' ? (
                    <Grid item xs={12} sx={{ height: '100%', overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                        <AppUsage appId={appTools.id} />
                      </Box>
                    </Grid>
                  ) : tabValue === 'skills' ? (
                    <Grid item xs={12} sx={{ height: '100%', overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ mt: "-1px", borderTop: '1px solid #303047', p: 3 }}>
                        { appTools.flatApp && (
                          <Skills
                            app={appTools.flatApp}
                            onUpdate={appTools.saveFlatApp}
                          />
                        )}
                      </Box>
                    </Grid>
                  ) : (
                    <>
                      <Grid item xs={12} md={6} sx={{
                        borderRight: '1px solid #303047',
                        height: '100%',
                        overflow: 'auto',
                        pb: 8,
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