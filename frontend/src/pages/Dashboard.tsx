import React, { FC, useState, useEffect, useRef, useCallback } from 'react'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Divider from '@mui/material/Divider'
import Typography from '@mui/material/Typography'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Switch from '@mui/material/Switch'
import Container from '@mui/material/Container'

import Page from '../components/system/Page'
import Interaction from '../components/session/Interaction'
import Window from '../components/widgets/Window'
import JsonWindowLink from '../components/widgets/JsonWindowLink'
import SessionSummary from '../components/session/SessionSummary'
import SessionHeader from '../components/session/SessionHeader'
import RunnerSummary from '../components/session/RunnerSummary'
import SchedulingDecisionSummary from '../components/session/SchedulingDecisionSummary'
import SessionBadgeKey from '../components/session/SessionBadgeKey'

import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'

import {
  IDashboardData,
  ISession,
  ISessionSummary,
} from '../types'

const START_ACTIVE = true

const Dashboard: FC = () => {
  const account = useAccount()
  const router = useRouter()
  const api = useApi()

  const activeRef = useRef(START_ACTIVE)

  const [ viewingSession, setViewingSession ] = useState<ISession>()
  const [ active, setActive ] = useState(START_ACTIVE)
  const [ data, setData ] = useState<IDashboardData>()

  const {
    session_id,
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
    if(!session_id) return
    if(!account.user) return
    const loadSession = async () => {
      const session = await api.get<ISession>(`/api/v1/sessions/${ session_id }`)
      if(!session) return
      setViewingSession(session)
    }
    loadSession()
  }, [
    account.user,
    session_id,
  ])

  useEffect(() => {
    if(!account.user) return
    const loadDashboard = async () => {
      if(!activeRef.current) return
      const data = await api.get<IDashboardData>(`/api/v1/dashboard`)
      if(!data) return
      setData(originalData => {
        return JSON.stringify(data) == JSON.stringify(originalData) ? originalData : data
      })
    }
    const intervalId = setInterval(loadDashboard, 1000)
    if(activeRef.current) loadDashboard()
    return () => {
      clearInterval(intervalId)
    }
  }, [
    account.user,
  ])

  if(!account.user) return null
  if(!data) return null

  return (
    <Page
      topbarTitle="Dashboard"
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
                    control={
                      <Switch
                        checked={ active }
                        onChange={ (event: React.ChangeEvent<HTMLInputElement>) => {
                          activeRef.current = event.target.checked
                          setActive(event.target.checked)
                        }}
                      />
                    }
                    label="Live Updates?"
                  />
                </FormGroup>
              </Box>
              <Box
                sx={{
                  flexGrow: 1,
                  textAlign: 'right',
                }}
              >
                <JsonWindowLink
                  data={ data }
                >
                  view data
                </JsonWindowLink>
              </Box>
              
            </Box>
            <Divider
              sx={{
                mt: 1,
                mb: 1,
              }}
            />
            {
              data?.runners.map((runner) => {
                const allSessions = runner.model_instances.reduce<ISessionSummary[]>((allSessions, modelInstance) => {
                  return modelInstance.current_session ? [ ...allSessions, modelInstance.current_session ] : allSessions
                }, [])
                return allSessions.length > 0 ? (
                  <React.Fragment key={ runner.id }>
                    <Typography variant="h6">Running: { runner.id }</Typography>
                    {
                      allSessions.map(session => (
                        <SessionSummary
                          key={ session.session_id }
                          session={ session }
                          onViewSession={ onViewSession }
                        />
                      ))
                    }
                  </React.Fragment>
                ) : null
              })
            }
            {
              data.session_queue.length > 0 && (
                <Typography variant="h6">Queued Jobs</Typography>
              )
            }
            {
              data.session_queue.map((session) => {
                return (
                  <SessionSummary
                    key={ session.session_id }
                    session={ session }
                    onViewSession={ onViewSession }
                  />
                )
              })
            }
            {
              data.global_scheduling_decisions.length > 0 && (
                <Typography variant="h6">Global Scheduling</Typography>
              )
            }
            {
              data.global_scheduling_decisions.map((decision, i) => {
                return (
                  <SchedulingDecisionSummary
                    key={ i }
                    decision={ decision }
                    onViewSession={ onViewSession }
                  />
                )
              })
            }
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
              spacing={ 2 }
              sx={{
                width: '100%',
              }}
            >
              {
                data.runners.map((runner) => {
                  return (
                    <Grid item key={ runner.id } sm={ 12 } md={ 6 }>
                      <RunnerSummary
                        runner={ runner }
                        onViewSession={ onViewSession }
                      />
                    </Grid>
                  )
                })
              }
            </Grid>
          </Box>
          {
            viewingSession && (
              <Window
                open
                size="lg"
                background="#FAEFE0"
                withCancel
                cancelTitle="Close"
                onCancel={ onCloseViewingSession }
              >  
                <SessionHeader
                  session={ viewingSession }
                />
                {
                  viewingSession.interactions.map((interaction: any, i: number) => {
                    return (
                      <Interaction
                        key={ i }
                        showFinetuning={ true }
                        serverConfig={ account.serverConfig }
                        interaction={ interaction }
                        session={ viewingSession }
                      />
                    )   
                  })
                }
              </Window>
            )
          }
        </Box>
      </Container>
    </Page>
  )
}

export default Dashboard