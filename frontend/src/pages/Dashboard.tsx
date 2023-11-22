import React, { FC, useMemo, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import axios from 'axios'

import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import Typography from '@mui/material/Typography'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import IconButton from '@mui/material/IconButton'
import DeleteIcon from '@mui/icons-material/Delete'
import Grid from '@mui/material/Grid'
import Container from '@mui/material/Container'

import SessionSummaryRow from '../components/session/SessionSummaryRow'

import {
  IDashboardData,
  ISession,
} from '../types'

const Dashboard: FC = () => {
  const account = useAccount()
  const api = useApi()

  const [ data, setData ] = useState<IDashboardData>()

  const runningSessions = useMemo<ISession[]>(() => {
    return [
      {
          "id": "b329fa8b-dff1-45c1-b4f3-b3c01e1c2efe",
          "name": "constructive-chat-754",
          "created": "0001-01-01T00:00:00Z",
          "updated": "2023-11-22T10:24:06.488428277Z",
          "parent_session": "",
          "mode": "inference",
          "type": "image",
          "model_name": "stabilityai/stable-diffusion-xl-base-1.0",
          "lora_dir": "",
          "interactions": [
              {
                  "id": "27c7c116-b9d8-4053-9cfd-fa86052dc9bd",
                  "created": "2023-11-22T09:39:15.508280625Z",
                  "scheduled": "",
                  "completed": "",
                  "creator": "user",
                  "runner": "",
                  "message": "a balloon",
                  "progress": 0,
                  "files": [],
                  "finished": true,
                  "metadata": {},
                  "state": "complete",
                  "status": "",
                  "error": "",
                  "lora_dir": ""
              },
              {
                  "id": "837d2207-4915-4b53-baf9-7d69e7e6d7fc",
                  "created": "2023-11-22T09:39:15.508282264Z",
                  "scheduled": "",
                  "completed": "",
                  "creator": "system",
                  "runner": "",
                  "message": "",
                  "progress": 0,
                  "files": [
                      "dev/users/0dd34fab-81c0-4e13-b603-edfb5acea7f2/sessions/b329fa8b-dff1-45c1-b4f3-b3c01e1c2efe/results/image_b329fa8b-dff1-45c1-b4f3-b3c01e1c2efe_20231122-093954_000.png"
                  ],
                  "finished": true,
                  "metadata": {},
                  "state": "complete",
                  "status": "",
                  "error": "",
                  "lora_dir": ""
              },
              {
                  "id": "dd7b4f7a-c5ae-4166-9190-944d1b3dd2d3",
                  "created": "2023-11-22T10:24:06.488140088Z",
                  "scheduled": "",
                  "completed": "",
                  "creator": "user",
                  "runner": "",
                  "message": "a tall building",
                  "progress": 0,
                  "files": [],
                  "finished": true,
                  "metadata": {},
                  "state": "complete",
                  "status": "",
                  "error": "",
                  "lora_dir": ""
              },
              {
                  "id": "3ddbe350-7ff3-40c2-9c9d-acda880c81e8",
                  "created": "2023-11-22T10:24:06.488142172Z",
                  "scheduled": "",
                  "completed": "",
                  "creator": "system",
                  "runner": "",
                  "message": "",
                  "progress": 0,
                  "files": [],
                  "finished": false,
                  "metadata": {},
                  "state": "waiting",
                  "status": "",
                  "error": "",
                  "lora_dir": ""
              }
          ],
          "owner": "0dd34fab-81c0-4e13-b603-edfb5acea7f2",
          "owner_type": "user"
      }
  ]
    // return data?.runners.reduce<ISession[]>((acc, runner) => {
    //   const runnerSessions = runner.model_instances.reduce<ISession[]>((acc, modelInstance) => {
    //     if(!modelInstance.current_session) return acc
    //     return [
    //       ...acc,
    //       modelInstance.current_session,
    //     ]
    //   }, [])
    //   return acc.concat(runnerSessions)
    // }, [])
  }, [
    data,
  ])
    
  useEffect(() => {
    const loadData = async () => {
      const data = await api.get<IDashboardData>(`/api/v1/dashboard`)
      if(!data) return
      setData(data)
    }
    const intervalId = setInterval(loadData, 1000)
    loadData()
    return () => {
      clearInterval(intervalId)
    }
  })

  if(!account.user) return null
  if(!data) return null

  console.log('--------------------------------------------')
  console.dir(runningSessions)

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
        <Typography variant="h6">Running Jobs</Typography>
        {
          runningSessions?.map((session) => {
            return (
              <SessionSummaryRow
                key={ session.id }
                session={ session }
              />
            )
          })
        }
        <Typography variant="h6">Queued Jobs</Typography>
        {
          data.session_queue.map((session) => {
            return (
              <SessionSummaryRow
                key={ session.id }
                session={ session }
              />
            )
          })
        }
      </Box>
      <Box
        sx={{
          flexGrow: 1,
        }}
      >
        Runners
      </Box>
    </Box>
  )
}

export default Dashboard