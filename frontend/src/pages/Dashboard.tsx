import React, { FC, useState, useEffect } from 'react'
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

import {ISession} from '../types'

const Dashboard: FC = () => {
  const account = useAccount()
  const api = useApi()

  const [ sessions, setSessions ] = useState<ISession[]>(
    [{id: "Loading...",
      name: "Loading...",
      created: 0,
      updated: 0,
      mode: "inference",
      type: "text",
      model_name: "",
      lora_dir: "",
      interactions: [],
      owner: "",
      owner_type: "user",
      parent_session: "",
      error: "",
    }]
  )

  useEffect(() => {
    const intervalId = setInterval(async () => {
      const data = await api.get(`/api/v1/dashboard`)
      console.log(JSON.stringify(data, null, 4))
      // setSessions(response.data.sessions)
    }, 1000)
    return () => {
      clearInterval(intervalId)
    }
  })

  
  if(!account.user) return null

  return (
    <Box sx={{mt:4}}>
        <Container maxWidth="lg">
          <Grid container spacing={3} direction="row" justifyContent="flex-start">
            <Grid item xs={4} md={4}>
              <Typography variant="h6">Session Queue</Typography>
              <ul>
              {sessions.map((session) => {
                return (<li key={session.id}>{session.id}</li>)
              })}
              </ul>
            </Grid>
            <Grid item xs={8} md={8}>
              <Typography variant="h6">Runners</Typography>
            </Grid>
          </Grid>
        </Container>
    </Box>
  )
}

export default Dashboard