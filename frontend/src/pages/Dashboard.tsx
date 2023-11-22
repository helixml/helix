import React, { FC, useState, useEffect, useRef } from 'react'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import Divider from '@mui/material/Divider'
import Typography from '@mui/material/Typography'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Switch from '@mui/material/Switch'

import JsonWindowLink from '../components/widgets/JsonWindowLink'
import SessionSummary from '../components/session/SessionSummary'
import RunnerSummary from '../components/session/RunnerSummary'
import SchedulingDecisionSummary from '../components/session/SchedulingDecisionSummary'

import {
  IDashboardData,
  ISession,
} from '../types'

const Dashboard: FC = () => {
  const account = useAccount()
  const api = useApi()

  const activeRef = useRef(true)
  const [ active, setActive ] = useState(true)
  const [ data, setData ] = useState<IDashboardData>()

  useEffect(() => {
    const loadData = async () => {
      if(!activeRef.current) return
      const data = await api.get<IDashboardData>(`/api/v1/dashboard`)
      if(!data) return
      setData(originalData => {
        return JSON.stringify(data) == JSON.stringify(originalData) ? originalData : data
      })
    }
    const intervalId = setInterval(loadData, 1000)
    loadData()
    return () => {
      clearInterval(intervalId)
    }
  }, [])

  if(!account.user) return null
  if(!data) return null

  return (
    <Box
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'flex-start',
        justifyContent: 'flex-start',
      }}
    >
      <Box
        sx={{
          p: 2,
          flexGrow: 0,
          height: '100%',
          width: '400px',
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
            const allSessions = runner.model_instances.reduce<ISession[]>((allSessions, modelInstance) => {
              return modelInstance.current_session ? [ ...allSessions, modelInstance.current_session ] : allSessions
            }, [])
            return allSessions.length > 0 ? (
              <React.Fragment key={ runner.id }>
                <Typography variant="h6">Running: { runner.id }</Typography>
                {
                  allSessions.map(session => (
                    <SessionSummary
                      key={ session.id }
                      session={ session }
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
                key={ session.id }
                session={ session }
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
          overflowY: 'auto',
        }}
      >
        <Grid container spacing={ 2 }>
          {
            data.runners.map((runner) => {
              return (
                <Grid item key={ runner.id } xs={ 12 } md={ 6 }>
                  <RunnerSummary
                    runner={ runner }
                  />
                </Grid>
              )
            })
          }
        </Grid>
      </Box>
    </Box>
  )
}

export default Dashboard