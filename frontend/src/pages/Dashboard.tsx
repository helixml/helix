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
import SchedulingDecisionSummary from '../components/session/SchedulingDecisionSummary'
import SessionBadgeKey from '../components/session/SessionBadgeKey'
import SessionSummary from '../components/session/SessionSummary'
import SessionToolbar from '../components/session/SessionToolbar'
import Page from '../components/system/Page'
import JsonWindowLink from '../components/widgets/JsonWindowLink'
import Window from '../components/widgets/Window'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import {
  IDashboardData,
  IModelInstanceState,
  IRunnerState,
  ISession,
  ISessionSummary,
  ISlot,
  ISlotAttributesWorkload,
  ISlotData
} from '../types'

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
    if (tab === 'llm_calls') {
      setActiveTab(1)
    } else {
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
    if (newValue === 1) {
      router.setParams({ tab: 'llm_calls' })
    } else {
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


  const convertWorkloadToSessionSummary = (workload: ISlotAttributesWorkload): ISessionSummary | null => {
    if (!workload) return null
    if (workload.Session) {
      const lastInteraction = workload.Session.interactions?.[workload.Session.interactions?.length - 1]
      return {
        created: workload.Session.created,
        updated: workload.Session.updated,
        scheduled: lastInteraction?.created,
        completed: lastInteraction?.completed,
        session_id: workload.Session.id,
        name: workload.Session.name,
        interaction_id: lastInteraction?.id ?? '',
        model_name: workload.Session.model_name,
        mode: workload.Session.mode,
        type: workload.Session.type,
        owner: workload.Session.owner,
        lora_dir: workload.Session.lora_dir,
        summary: lastInteraction?.state
          ? [
            lastInteraction.state,
            lastInteraction.status,
            lastInteraction.message
          ].filter(Boolean).join(" ")
          : "No summary available",
        app_id: workload.Session.parent_app,
      }
    } else if (workload.LLMInferenceRequest) {
      return {
        created: workload.LLMInferenceRequest.CreatedAt,
        updated: workload.LLMInferenceRequest.CreatedAt,
        scheduled: workload.LLMInferenceRequest.CreatedAt,
        completed: workload.LLMInferenceRequest.CreatedAt,
        session_id: '',
        name: '',
        interaction_id: workload.LLMInferenceRequest.InteractionID,
        model_name: workload.LLMInferenceRequest.Request.model,
        mode: "inference",
        type: "text",
        owner: workload.LLMInferenceRequest.OwnerID,
        lora_dir: "",
        app_id: "",
        summary: workload.LLMInferenceRequest?.Request?.messages?.[
          (workload.LLMInferenceRequest?.Request?.messages?.length || 1) - 1
        ]?.content || 'No message content',
      }
    }
    return null
  };

  const convertSlotDataToModelInstanceState = (slotData: ISlotData) => {
    return {
      id: slotData.id,
      model_name: slotData.attributes.model,
      mode: slotData.attributes.mode,
      lora_dir: '',
      initial_session_id: '',
      current_session: convertWorkloadToSessionSummary(slotData.attributes.workload),
      job_history: [],
      timeout: 0,
      last_activity: 0,
      stale: false,
      memory: 0,
      status: slotData.id,
    } as IModelInstanceState
  }

  const convertSlotDataToRunnerState = (runnerId: string, desiredSlots: ISlot[]) => {
    const matchingSlot = desiredSlots.find(slot => slot.id === runnerId)
    if (!matchingSlot) return {} as IRunnerState

    return {
      id: runnerId,
      created: '',
      memory: 0,
      free_memory: 0,
      total_memory: 0,
      labels: {},
      model_instances: matchingSlot.data
        .map(convertSlotDataToModelInstanceState)
        .sort((a, b) => a.model_name.localeCompare(b.model_name)),
      scheduling_decisions: [],
    } as IRunnerState
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
            <Tab label="LLM Calls" />
            <Tab label="Dashboard" />
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
                {data?.runners.map((runner) => {
                  const allSessions = runner.model_instances.reduce<ISessionSummary[]>((allSessions, modelInstance) => {
                    return modelInstance.current_session ? [...allSessions, modelInstance.current_session] : allSessions
                  }, [])
                  return allSessions.length > 0 ? (
                    <React.Fragment key={runner.id}>
                      <Typography variant="h6">Running: {runner.id}</Typography>
                      {allSessions.map(session => (
                        <SessionSummary
                          key={session.session_id}
                          session={session}
                          onViewSession={onViewSession} />
                      ))}
                    </React.Fragment>
                  ) : null
                })}
                {data.session_queue.length > 0 && (
                  <Typography variant="h6">Queued Jobs</Typography>
                )}
                {data.session_queue.map((session) => {
                  return (
                    <SessionSummary
                      key={session.session_id}
                      session={session}
                      onViewSession={onViewSession} />
                  )
                })}
                {data.global_scheduling_decisions.length > 0 && (
                  <Typography variant="h6">Global Scheduling</Typography>
                )}
                {data.global_scheduling_decisions.map((decision, i) => {
                  return (
                    <SchedulingDecisionSummary
                      key={i}
                      decision={decision}
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
                >
                  {data.runners.map((runner) => {
                    return (
                      <Grid item key={runner.id} spacing={2} padding={2}>
                        <Paper>
                          <Grid container spacing={1} padding={1}>
                            <Grid item xs={6}>
                              <Typography variant="h6">Desired State</Typography>
                              <RunnerSummary
                                runner={convertSlotDataToRunnerState(runner.id, data.desired_slots)}
                                onViewSession={onViewSession} />
                            </Grid>
                            <Grid item xs={6}>
                              <Typography variant="h6">Actual State</Typography>
                              <RunnerSummary
                                runner={runner}
                                onViewSession={onViewSession} />
                            </Grid>
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