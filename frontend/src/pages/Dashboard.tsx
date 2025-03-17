import ClearIcon from '@mui/icons-material/Clear'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Divider from '@mui/material/Divider'
import FormControlLabel from '@mui/material/FormControlLabel'
import FormGroup from '@mui/material/FormGroup'
import Grid from '@mui/material/Grid'
import IconButton from '@mui/material/IconButton'
import Paper from '@mui/material/Paper/Paper'
import Switch from '@mui/material/Switch'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import React, { FC, useCallback, useEffect, useRef, useState } from 'react'
import LLMCallsTable from '../components/dashboard/LLMCallsTable'
import Interaction from '../components/session/Interaction'
import RunnerSummary from '../components/session/RunnerSummary'
import SessionBadgeKey from '../components/session/SessionBadgeKey'
import SessionToolbar from '../components/session/SessionToolbar'
import SessionSummary from '../components/session/SessionSummary'
import Page from '../components/system/Page'
import JsonWindowLink from '../components/widgets/JsonWindowLink'
import Window from '../components/widgets/Window'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import {
  IDashboardData,
  IQueueItem,
  ISession,
  ISessionSummary
} from '../types'
import ProviderEndpointsTable from '../components/dashboard/ProviderEndpointsTable'
import OAuthProvidersTable from '../components/dashboard/OAuthProvidersTable'

const START_ACTIVE = true

const Dashboard: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const api = useApi()

  const activeRef = useRef(START_ACTIVE)

  const [viewingSession, setViewingSession] = useState<ISession>()
  const [active, setActive] = useState(START_ACTIVE)
  const [data, setData] = useState<IDashboardData>()
  const [activeTab, setActiveTab] = useState(0)
  const [sessionFilter, setSessionFilter] = useState('')

  const {
    session_id,
    tab,
    filter_sessions,
  } = router.params

  const onViewSession = useCallback((session_id: string) => {
    router.setParams({
      session_id,
    })
  }, [])

  const onCloseViewingSession = useCallback(() => {
    setViewingSession(undefined)
    router.removeParams(['session_id'])
  }, [])

  useEffect(() => {
    if (!session_id) return
    if (!account.user) return
    const loadSession = async () => {
      const session = await api.get<ISession>(`/api/v1/sessions/${session_id}`)
      if (!session) return
      setViewingSession(session)
    }
    loadSession()
  }, [
    account.user,
    session_id,
  ])

  useEffect(() => {
    if (!account.user) return
    const loadDashboard = async () => {
      if (!activeRef.current) return
      const data = await api.get<IDashboardData>(`/api/v1/dashboard`)
      if (!data) return
      setData(originalData => {
        return JSON.stringify(data) == JSON.stringify(originalData) ? originalData : data
      })
    }
    const intervalId = setInterval(loadDashboard, 1000)
    if (activeRef.current) loadDashboard()
    return () => {
      clearInterval(intervalId)
    }
  }, [
    account.user,
  ])

  useEffect(() => {
    switch (tab) {
      case 'llm_calls':
        setActiveTab(1)
        break
      case 'providers':
        setActiveTab(2)
        break
      case 'oauth_providers':
        setActiveTab(3)
        break
      default:
        setActiveTab(0)
    }
  }, [tab])

  useEffect(() => {
    if (filter_sessions) {
      setSessionFilter(filter_sessions)
    }
  }, [filter_sessions])

  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    setActiveTab(newValue)
    switch (newValue) {
      case 1:
        router.setParams({ tab: 'llm_calls' })
        break
      case 2:
        router.setParams({ tab: 'providers' })
        break
      case 3:
        router.setParams({ tab: 'oauth_providers' })
        break
      default:
        router.removeParams(['tab'])
    }
  }

  const handleFilterChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newFilter = event.target.value
    setSessionFilter(newFilter)
    if (newFilter) {
      router.setParams({ filter_sessions: newFilter })
    } else {
      router.removeParams(['filter_sessions'])
    }
  }

  const clearFilter = () => {
    setSessionFilter('')
    router.removeParams(['filter_sessions'])
  }

  if (!account.user) return null
  if (!data) return null

  return (
    <Page
      breadcrumbTitle="Dashboard"
      topbarContent={(
        <Box
          sx={{
            width: '100%',
            display: 'flex',
            flexDirection: 'row',
            alignItems: 'center',
            justifyContent: 'flex-end',
          }}
        >
          <SessionBadgeKey />
        </Box>
      )}
    >
      <Container
        maxWidth="xl"
        sx={{
          mt: 12,
          height: 'calc(100% - 100px)',
        }}
      >
        <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
          <Tabs value={activeTab} onChange={handleTabChange}>
            <Tab label="Sessions" />
            <Tab label="LLM Calls" />
            <Tab label="Providers" />
            <Tab label="OAuth Providers" />
          </Tabs>
        </Box>

        {activeTab === 1 && (
          <Box
            sx={{
              width: '100%',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'flex-start',
              justifyContent: 'flex-start',
            }}
          >
            <Box
              sx={{
                width: '100%',
                display: 'flex',
                flexDirection: 'row',
                alignItems: 'flex-start',
                justifyContent: 'flex-start',
              }}
            >
              <Box
                sx={{
                  p: 3,
                  flexGrow: 0,
                  width: '480px',
                  minWidth: '480px',
                  overflowY: 'auto',
                  display: { xs: 'none', md: 'block' }
                }}
              >
                <Box
                  sx={{
                    display: 'flex',
                    flexDirection: 'row',
                    alignItems: 'center',
                  }}
                >
                  <Box
                    sx={{
                      flexGrow: 0,
                    }}
                  >
                    <FormGroup>
                      <FormControlLabel
                        control={<Switch
                          checked={active}
                          onChange={(event: React.ChangeEvent<HTMLInputElement>) => {
                            activeRef.current = event.target.checked
                            setActive(event.target.checked)
                          }} />}
                        label="Live Updates?" />
                    </FormGroup>
                  </Box>
                  <Box
                    sx={{
                      flexGrow: 1,
                      textAlign: 'right',
                    }}
                  >
                    <JsonWindowLink
                      data={data}
                    >
                      view data
                    </JsonWindowLink>
                  </Box>

                </Box>
                <Divider
                  sx={{
                    mt: 1,
                    mb: 1,
                  }} />
                {account.serverConfig.version && (
                  <Box>
                    <Typography variant="h6">
                      Helix Control Plane version: {account.serverConfig.version}
                    </Typography>
                  </Box>
                )}               
                {data.queue.length > 0 && (
                  <Typography variant="h6">Queued Jobs</Typography>
                )}
                {data.queue.map((item: IQueueItem) => {
                  return (
                    <SessionSummary
                      key={item.id}
                      session={
                        {
                          session_id: item.id,
                          created: item.created,
                          updated: item.updated,
                          model_name: item.model_name,
                          mode: item.mode,
                          type: item.runtime,
                          owner: "todo",
                          lora_dir: item.lora_dir,
                          summary: item.summary,
                          app_id: "todo",
                        } as ISessionSummary
                      }
                      onViewSession={onViewSession} />
                  )
                })}             
              </Box>
              <Box
                sx={{
                  flexGrow: 1,
                  p: 2,
                  height: '100%',
                  width: '100%',
                  overflowY: 'auto',
                }}
              >
                <Grid
                  container
                  sx={{
                    width: '100%',
                    overflow: "auto",
                  }}
                  spacing={2} padding={2}
                >
                  {data.runners?.map((runner) => {
                    return (
                      <Grid item key={runner.id}>
                        <Paper>
                          <Grid item>
                            <Typography variant="h6">Runner State</Typography>
                            <RunnerSummary
                              runner={runner}
                              onViewSession={onViewSession} />
                          </Grid>
                        </Paper>
                      </Grid>
                    )
                  })}
                </Grid>
              </Box>
            </Box>
          </Box>
        )}

        {activeTab === 0 && (
          <Box
            sx={{
              width: '100%',
              height: 'calc(200vh - 200px)',
              overflow: 'auto',
            }}
          >
            <Box sx={{ mb: 2, display: 'flex', alignItems: 'center' }}>
              <TextField
                label="Filter by Session ID"
                variant="outlined"
                value={sessionFilter}
                onChange={handleFilterChange}
                sx={{ flexGrow: 1, mr: 1 }}
              />
              {sessionFilter && (
                <IconButton onClick={clearFilter} size="small">
                  <ClearIcon />
                </IconButton>
              )}
            </Box>
            <LLMCallsTable sessionFilter={sessionFilter} />
          </Box>
        )}

        {activeTab === 2 && (
          <Box
            sx={{
              width: '100%',
              height: 'calc(100vh - 200px)',
              overflow: 'auto',
            }}
          >
            <ProviderEndpointsTable />
          </Box>
        )}

        {activeTab === 3 && (
          <Box
            sx={{
              width: '100%',
              height: 'calc(100vh - 200px)',
              overflow: 'auto',
              p: 2,
            }}
          >
            <OAuthProvidersTable />
          </Box>
        )}

        {viewingSession && (
          <Window
            open
            size="lg"
            background="#FAEFE0"
            withCancel
            cancelTitle="Close"
            onCancel={onCloseViewingSession}
          >
            <SessionToolbar
              session={viewingSession}
            />
            {viewingSession.interactions.map((interaction: any, i: number) => {
              return (
                <Interaction
                  key={i}
                  showFinetuning={true}
                  serverConfig={account.serverConfig}
                  interaction={interaction}
                  session={viewingSession}
                />
              )
            })}
          </Window>
        )}
      </Container>
    </Page>
  )
}

export default Dashboard