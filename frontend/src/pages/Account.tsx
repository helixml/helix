import React, { FC } from 'react'
import axios from 'axios'
import {CopyToClipboard} from 'react-copy-to-clipboard'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import Typography from '@mui/material/Typography'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import IconButton from '@mui/material/IconButton'
import Paper from '@mui/material/Paper'
import Grid from '@mui/material/Grid'

import DeleteIcon from '@mui/icons-material/Delete'
import CopyIcon from '@mui/icons-material/CopyAll'

import Subscription from '../components/account/Subscription'

import useSnackbar from '../hooks/useSnackbar'
import useAccount from '../hooks/useAccount'

const Account: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  
  if(!account.user) return null
  const handleDeleteApiKey = async (key: string) => {
    try {
      await axios.delete(`/api/v1/api_keys?key=${key}`)
    } catch (error) {
      console.error(error)
    }
  }

  return (
    <Box
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <Box
        sx={{
          width: '100%',
          flexGrow: 1,
          overflowY: 'auto',
          p: 2,
        }}
      >
        <Container maxWidth="lg">
          <Grid container spacing={2}>
            <Grid item xs={12} md={6}>
              <Paper sx={{ p: 2 }}>
                <Typography variant="h6">API Keys</Typography>
                <List>
                  {account.apiKeys.map((apiKey) => (
                    <ListItem key={apiKey.key}>
                      <ListItemText primary={apiKey.name} secondary={apiKey.key} />
                      <ListItemSecondaryAction>
                        <CopyToClipboard
                          text={ apiKey.key }
                          onCopy={ () => {
                            snackbar.success('Copied to clipboard')
                          }}
                        >
                          <IconButton
                            edge="end"
                            aria-label="delete"
                            sx={{
                              mr: 2,
                            }}
                          >
                            <CopyIcon />
                          </IconButton>
                        </CopyToClipboard>
                        <IconButton
                          edge="end"
                          aria-label="delete"
                          onClick={() => handleDeleteApiKey(apiKey.key)}
                        >
                          <DeleteIcon />
                        </IconButton>
                      </ListItemSecondaryAction>
                    </ListItem>
                  ))}
                </List>
              </Paper>
            </Grid>
            <Grid item xs={12} md={6}>
              <Paper sx={{ p: 2 }}>
                <Subscription />
              </Paper>
            </Grid>
          </Grid>
        </Container>
      </Box>
    </Box>
  )
}

export default Account