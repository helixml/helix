import React, { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'
import ChatOutlinedIcon from '@mui/icons-material/ChatOutlined'

import Typography from '@mui/material/Typography'


import APIKeysSection from '../components/app/APIKeysSection'
import AppSettings from '../components/app/AppSettings'
import AgentInfoPanel from '../components/app/AgentInfoPanel'
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
import EvaluationTab from '../components/app/EvaluationTab'
import IdeIntegrationSection from '../components/app/IdeIntegrationSection'
import useLightTheme from '../hooks/useLightTheme'
import Skills from '../components/app/Skills'
import OrgAgentSettings from '../components/app/OrgAgentSettings'
import FocusedAgentDetails from '../components/app/FocusedAgentDetails'
import MemoriesManagement from '../components/app/MemoriesManagement'
import HelixOrgTopNav from '../components/helix-org/HelixOrgTopNav'
import {
  useActivateBot,
  useListHelixOrgBotDetails,
  useListHelixOrgBots,
} from '../services/helixOrgService'
import { AGENT_TYPE_ZED_EXTERNAL } from '../types'
import { isOrgAgent, usesFocusedAgentDetails } from '../utils/apps'

const App: FC = () => {
  const account = useAccount()  
  const api = useApi()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const router = useRouter()
  const { params } = router

  const appTools = useApp(params.app_id)
  const appIsOrgAgent = isOrgAgent(appTools.app)
  const { data: orgAgents = [], isLoading: orgAgentsLoading } = useListHelixOrgBots({
    enabled: appIsOrgAgent,
  })
  const orgAgentDetails = useListHelixOrgBotDetails(
    orgAgents.map((agent) => agent.id).filter((id): id is string => !!id),
    { enabled: appIsOrgAgent },
  )
  const linkedOrgAgentDetail = orgAgentDetails.find(
    (detail) => (detail?.agent_id ?? detail?.agent_app_id) === params.app_id,
  )
  const linkedOrgAgent = linkedOrgAgentDetail?.bot
  const orgAgentDetailLoading = orgAgentsLoading
    || (orgAgents.length > 0 && orgAgentDetails.some((detail) => !detail))
  const activateOrgAgent = useActivateBot()
  // Get user access information from appTools
  const { userAccess } = appTools

  const lightTheme = useLightTheme()

  const [deletingAPIKey, setDeletingAPIKey] = useState('')
  const [isAccessDenied, setIsAccessDenied] = useState(false)

  const [searchParams, setSearchParams] = useState(() => new URLSearchParams(window.location.search));
  const [isSearchMode, setIsSearchMode] = useState(() => searchParams.get('isSearchMode') === 'true');
  
  const legacyGeneralTabs = ['settings', 'instructions', 'appearance']
  const tabValue = !params.tab || legacyGeneralTabs.includes(params.tab) ? 'general' : params.tab

  useEffect(() => {
    if (params.tab && legacyGeneralTabs.includes(params.tab)) {
      router.mergeParams({ tab: 'general' })
    }
  }, [params.tab])

  useEffect(() => {
    const checkAccess = async () => {
      try {
        const result = await api.getApiClient().v1AgentsDetail(params.app_id)
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
  const appIsFocusedAgent = usesFocusedAgentDetails(appTools.app)

  const openChat = async () => {
    if (!linkedOrgAgent?.id) {
      account.orgNavigate('new', { app_id: appTools.id, resource_type: 'apps' })
      return
    }
    if (!params.org_id) return
    try {
      const result = await activateOrgAgent.mutateAsync(linkedOrgAgent.id)
      let sessionID = result.session_id
      if (!sessionID) {
        if (!result.project_id) throw new Error('failed to open agent chat')
        const response = await api.getApiClient().v1ProjectsExploratorySessionCreate(result.project_id)
        sessionID = response.data?.id
      }
      if (!sessionID) throw new Error('failed to open agent chat')
      router.navigate('org_session', { org_id: params.org_id, session_id: sessionID })
    } catch (error: any) {
      snackbar.error(error?.response?.data?.error ?? error?.message ?? 'failed to open agent chat')
    }
  }

  return (
    <Page
      showDrawerButton={false}
      orgBreadcrumbs={true}
      breadcrumbs={[
        {
          title: 'Agents',
          routeName: 'agents'
        },
        {
          title: appTools.flatApp?.name || 'Agent',
        }
      ]}
      topbarContent={(
        <>
          <Button
            variant="contained"
            color="secondary"
            startIcon={activateOrgAgent.isPending ? <CircularProgress size={16} /> : <ChatOutlinedIcon />}
            onClick={() => void openChat()}
            disabled={activateOrgAgent.isPending || (appIsOrgAgent && !linkedOrgAgent)}
          >
            Open chat
          </Button>
          <HelixOrgTopNav />
        </>
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
            <Grid item xs={12} sx={{
              p: 0,
              mt: 2,
              mb: 2,
            }}>
              <Box sx={{ width: '100%' }}>
                <Grid container spacing={0}>
                  {appIsFocusedAgent && appTools.flatApp ? (
                    <Grid item xs={12} sx={{ pb: 8 }}>
                      <FocusedAgentDetails
                        agentID={appTools.id}
                        app={appTools.flatApp}
                        kind={appIsOrgAgent ? 'org' : 'coding'}
                        onUpdate={appTools.saveFlatApp}
                        onCanonicalUpdate={() => appTools.loadApp(appTools.id)}
                        readOnly={isReadOnly}
                        showErrors={appTools.showErrors}
                        isAdmin={account.admin}
                        orgAgentDetail={linkedOrgAgentDetail}
                        orgAgentDetailLoading={orgAgentDetailLoading}
                        accessManagement={userAccess?.isAdmin ? (
                          <AccessManagement
                            appId={appTools.id}
                            accessGrants={appTools.accessGrants}
                            isLoading={false}
                            isReadOnly={isReadOnly}
                            onCreateGrant={appTools.createAccessGrant}
                            onDeleteGrant={appTools.deleteAccessGrant}
                          />
                        ) : undefined}
                      />
                    </Grid>
                  ) : tabValue === 'general' ? (
                    <Grid item xs={12} sx={{ pb: 8 }}>
                      <Grid container spacing={4}>
                        <Grid item xs={12} md={8}>
                          {appTools.flatApp && (
                            <AppSettings
                              id={appTools.id}
                              app={appTools.flatApp}
                              onUpdate={appTools.saveFlatApp}
                              readOnly={isReadOnly}
                              showErrors={appTools.showErrors}
                              isAdmin={account.admin}
                              section="general"
                              generalAside={(
                                <AppearanceSettings
                                  app={appTools.flatApp}
                                  onUpdate={appTools.saveFlatApp}
                                  readOnly={isReadOnly}
                                  id={appTools.id}
                                  section="avatar"
                                />
                              )}
                            />
                          )}
                          {appTools.flatApp && (
                            <AppearanceSettings
                              app={appTools.flatApp}
                              onUpdate={appTools.saveFlatApp}
                              readOnly={isReadOnly}
                              showErrors={appTools.showErrors}
                              id={appTools.id}
                              section="conversation-starters"
                            />
                          )}
                          {appIsOrgAgent && (
                            <OrgAgentSettings
                              agentID={appTools.id}
                              section="runtime"
                              readOnly={isReadOnly}
                              detail={linkedOrgAgentDetail}
                            />
                          )}
                        </Grid>
                        <Grid item xs={12} md={4}>
                          <AgentInfoPanel app={appTools.app} orgAgent={linkedOrgAgent} />
                        </Grid>
                      </Grid>
                    </Grid>
                  ) : tabValue === 'runtime' ? (
                    <Grid item xs={12} sx={{ pb: 8 }}>
                      <Box sx={{ maxWidth: 960 }}>
                        {appTools.flatApp && (
                          <AppSettings
                            id={appTools.id}
                            app={appTools.flatApp}
                            onUpdate={appTools.saveFlatApp}
                            readOnly={isReadOnly}
                            showErrors={appTools.showErrors}
                            isAdmin={account.admin}
                            section="runtime"
                            hideAgentType={appIsOrgAgent}
                          />
                        )}
                      </Box>
                    </Grid>
                  ) : tabValue === 'usage' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box>
                        <AppUsage appId={appTools.id} />
                      </Box>
                    </Grid>
                  ) : tabValue === 'skills' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box>
                        { appTools.flatApp && (
                          <Skills
                            app={appTools.flatApp}
                            appId={appTools.id}
                            onUpdate={appTools.saveFlatApp}
                          />
                        )}
                        {appIsOrgAgent && (
                          <OrgAgentSettings agentID={appTools.id} section="tools" readOnly={isReadOnly} />
                        )}
                      </Box>
                    </Grid>
                  ) : tabValue === 'tests' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box>
                        { appTools.flatApp && (
                          <TestsEditor
                            app={appTools.flatApp}
                            onUpdate={appTools.saveFlatApp}
                            appId={appTools.id}
                          />
                        )}
                      </Box>
                    </Grid>
                  ) : tabValue === 'evaluation' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box>
                        <EvaluationTab appId={appTools.id} />
                      </Box>
                    </Grid>
                  ) : tabValue === 'memories' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box>
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
                      <Box>
                        <DevelopersSection
                          schema={appTools.appSchema}
                          setSchema={appTools.setAppSchema}
                          showErrors={appTools.showErrors}
                          appId={appTools.id}
                          appName={appTools.flatApp?.name}
                        />
                      </Box>
                    </Grid>
                  ) : tabValue === 'mcp' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box>
                        <IdeIntegrationSection
                          appId={appTools.id}
                        />
                      </Box>
                    </Grid>
                  ) : tabValue === 'triggers' ? (
                    <Grid item xs={12} sx={{ overflow: 'auto', pb: 8, ...lightTheme.scrollbar }}>
                      <Box sx={{ maxWidth: 1100 }}>
                        {appIsOrgAgent && (
                          <OrgAgentSettings agentID={appTools.id} section="subscriptions" readOnly={isReadOnly} />
                        )}
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
                      <Box sx={{ mt: 2, maxWidth: 1100 }}>
                        {appIsOrgAgent && (
                          <OrgAgentSettings agentID={appTools.id} section="access" readOnly={isReadOnly} />
                        )}
                        {userAccess?.isAdmin && (
                          <AccessManagement
                            appId={appTools.id}
                            accessGrants={appTools.accessGrants}
                            isLoading={false}
                            isReadOnly={isReadOnly}
                            onCreateGrant={appTools.createAccessGrant}
                            onDeleteGrant={appTools.deleteAccessGrant}
                          />
                        )}
                      </Box>
                    </Grid>
                  ) : (
                    <>
                      <Grid item xs={12} md={appTools.flatApp?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL && tabValue !== 'apikeys' ? 12 : 6} sx={{
                        pb: 8,
                        minHeight: 'calc(100vh - 120px)'
                      }}>
                        <Box>
                          <Box sx={{ display: tabValue === 'knowledge' ? 'block' : 'none', height: '100%', overflow: 'auto', mr: 2 }}>
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

                          <Box sx={{ display: tabValue === 'apikeys' ? 'block' : 'none' }}>
                            <APIKeysSection
                              apiKeys={account.appApiKeys}
                              onAddAPIKey={() => account.addAppAPIKey(appTools.id)}
                              onDeleteKey={(key) => setDeletingAPIKey(key)}
                              allowedDomains={appTools.flatApp?.allowedDomains || []}
                              setAllowedDomains={(allowedDomains) => appTools.saveFlatApp({ allowedDomains })}
                              isReadOnly={isReadOnly}
                            />
                          </Box>
                        </Box>
                      </Grid>
                      {tabValue === 'apikeys' ? (
                        <CodeExamples apiKey={account.appApiKeys[0]?.key || ''} />
                      ) : appTools.flatApp?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL ? null : (
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
