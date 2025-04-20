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
import Chip from '@mui/material/Chip'

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
        setActiveTab(0)
        break
      case 'providers':
        setActiveTab(1)
        break
      case 'oauth_providers':
        setActiveTab(2)
        break
      case 'runners':
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
      case 0:
        router.setParams({ tab: 'llm_calls' })
        break
      case 1:
        router.setParams({ tab: 'providers' })
        break
      case 2:
        router.setParams({ tab: 'oauth_providers' })
        break
      case 3:
        router.setParams({ tab: 'runners' })
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
            justifyContent: 'space-between',
          }}
        >
          {account.serverConfig.version && (
            <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.7)' }}>
              Helix Control Plane version: {account.serverConfig.version}
            </Typography>
          )}
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
            <Tab label="LLM Calls" />
            <Tab label="Inference Providers" />
            <Tab label="OAuth Providers" />
            <Tab label="Runners" />
          </Tabs>
        </Box>

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

        {activeTab === 1 && (
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

        {activeTab === 2 && (
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

        {activeTab === 3 && (
          <Box
            sx={{
              width: '100%',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'flex-start',
              justifyContent: 'flex-start',
            }}
          >
            {/* Controls for entire Runners tab */}
            <Box
              sx={{
                width: '100%',
                display: 'flex',
                flexDirection: 'row',
                alignItems: 'center',
                justifyContent: 'space-between',
                mb: 3,
                px: 2,
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
              
              <JsonWindowLink data={data}>
                view data
              </JsonWindowLink>
            </Box>

            <Box
              sx={{
                width: '100%',
                display: 'flex',
                flexDirection: 'row',
                alignItems: 'flex-start',
                justifyContent: 'flex-start',
              }}
            >
              {/* Queue Section */}
              <Box
                sx={{
                  p: 3,
                  flexGrow: 0,
                  width: '480px',
                  minWidth: '480px',
                  overflowY: 'auto',
                  display: { xs: 'none', md: 'block' },
                  borderRight: '1px solid rgba(255, 255, 255, 0.08)',
                }}
              >
                {/* Queue Section Header */}
                <Box
                  sx={{
                    mb: 3,
                    pb: 1,
                    borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
                  }}
                >
                  <Typography 
                    variant="h5"
                    sx={{ 
                      color: 'rgba(255, 255, 255, 0.95)',
                      fontWeight: 600,
                      display: 'flex',
                      alignItems: 'center',
                    }}
                  >
                    Queue
                    {data.queue.length > 0 && (
                      <Chip
                        size="small"
                        label={data.queue.length}
                        sx={{
                          ml: 2,
                          height: 22,
                          minWidth: 20,
                          backgroundColor: 'rgba(128, 90, 213, 0.15)',
                          color: 'rgba(255, 255, 255, 0.7)',
                          border: '1px solid rgba(128, 90, 213, 0.3)',
                          '& .MuiChip-label': {
                            px: 1,
                            fontSize: '0.7rem',
                            fontWeight: 600,
                          }
                        }}
                      />
                    )}
                  </Typography>
                </Box>

                {data.queue.length === 0 && (
                  <Box sx={{ 
                    py: 4, 
                    textAlign: 'center',
                    backgroundColor: 'rgba(25, 25, 28, 0.3)',
                    borderRadius: '3px',
                  }}>
                    <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.5)' }}>
                      No jobs in queue
                    </Typography>
                  </Box>
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

              {/* Runners Section */}
              <Box
                sx={{
                  flexGrow: 1,
                  p: 3,
                  height: '100%',
                  width: '100%',
                  overflowY: 'auto',
                }}
              >
                {/* Runners Section Header */}
                <Typography 
                  variant="h5"
                  sx={{ 
                    mb: 4,
                    color: 'rgba(255, 255, 255, 0.95)',
                    fontWeight: 600,
                    borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
                    pb: 1,
                  }}
                >
                  Runner State
                </Typography>

                <Grid
                  container
                  sx={{
                    width: '100%',
                    overflow: "auto",
                  }}
                  spacing={3}
                >
                  {data.runners?.length === 0 && (
                    <Grid item xs={12}>
                      <Box sx={{ 
                        py: 4, 
                        textAlign: 'center',
                        backgroundColor: 'rgba(25, 25, 28, 0.3)',
                        borderRadius: '3px',
                      }}>
                        <Typography variant="body2" sx={{ color: 'rgba(255, 255, 255, 0.5)' }}>
                          No active runners
                        </Typography>
                      </Box>
                    </Grid>
                  )}

                  {data.runners?.map((runner) => {
                    return (
                      <Grid item xs={12} key={runner.id}>
                        <RunnerSummary
                          runner={runner}
                          onViewSession={onViewSession} 
                        />
                      </Grid>
                    )
                  })}
                </Grid>
              </Box>
            </Box>
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