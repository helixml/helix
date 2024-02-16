import React, { FC, useState, useEffect, useCallback } from 'react'
import { HTTPSnippet } from '@readme/httpsnippet'
import {CopyToClipboard} from 'react-copy-to-clipboard'
import Container from '@mui/material/Container'
import Button from '@mui/material/Button'
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

import useSnackbar from '../hooks/useSnackbar'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'

const Account: FC = () => {
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()

  let [success, setSuccess] = useState(false)

  const paymentsActive = account.serverConfig.stripe_enabled
  const colSize = paymentsActive ? 6 : 12

  const handleDeleteApiKey = useCallback(async (key: string) => {
    await api.delete(`/api/v1/api_keys`, {
      params: {
        key,
      }
    }, {
      loading: true,
      snackbar: true,
    })
  }, [])

  const handleSubscribe = useCallback(async () => {
    const result = await api.post(`/api/v1/subscription/new`, undefined, {}, {
      loading: true,
      snackbar: true,
    })
    if(!result) return
    document.location = result
  }, [
    account.user,
  ])

  const handleManage = useCallback(async () => {
    const result = await api.post(`/api/v1/subscription/manage`, undefined, {}, {
      loading: true,
      snackbar: true,
    })
    if(!result) return
    document.location = result
  }, [
    account.user,
  ])

  useEffect(() => {
    const query = new URLSearchParams(window.location.search)
    if (query.get('success')) {
      setSuccess(true)
      snackbar.success('Subscription successful')
    }
  }, [])

  if(!account.user) return null

  const apiToken = account.apiKeys[0].key

  let headers = [
    {
      name: 'Content-Type',
      value: 'application/json'
    },
    {
      name: 'Authorization',
      value: `Bearer ${apiToken}`
    }
  ]

  const snippet = new HTTPSnippet({
    method: 'POST',
    url: `${window.location.protocol}//${window.location.host}/api/images/generations`,
    headers: headers,
    postData: {
      text: `{"prompt": "futuristic city, extra detailed"}`,
      mimeType: "application/json"
    }
  })

  const code = snippet.convert('shell', 'curl', { indent: '\t' })

  return (
    <Container maxWidth="lg" sx={{ mt: 12, height: 'calc(100% - 100px)' }}>
      <Box sx={{ width: '100%', maxHeight: '100%', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
        <Box sx={{ width: '100%', flexGrow: 1, overflowY: 'auto', px: 2 }}>
          <Grid container spacing={2}>
            <Grid item xs={12} md={colSize}>
              <Paper sx={{ p: 2 }}>
                <Typography variant="h6">API</Typography>

                <Typography sx={{ mt: 8 }}>Keys:</Typography>
                <List>
                  {account.apiKeys.map((apiKey) => (
                    <ListItem key={apiKey.key}>
                      <ListItemText primary={apiKey.name} secondary={apiKey.key} />
                      <ListItemSecondaryAction>
                        <CopyToClipboard text={apiKey.key} onCopy={() => snackbar.success('Copied to clipboard')}>
                          <IconButton edge="end" aria-label="copy" sx={{ mr: 2 }}>
                            <CopyIcon />
                          </IconButton>
                        </CopyToClipboard>
                        <IconButton edge="end" aria-label="delete" onClick={() => handleDeleteApiKey(apiKey.key)}>
                          <DeleteIcon />
                        </IconButton>
                      </ListItemSecondaryAction>
                    </ListItem>
                  ))}
                </List>                              
              </Paper>

              <Paper sx={{ p: 2 }}>
                <Typography variant="h6">Using API</Typography>
                  
                <Typography>Example:</Typography>
                <pre>
                  {code}
                </pre>
              </Paper>
            </Grid>
            {paymentsActive && (
              <Grid item xs={12} md={colSize}>
                <Paper sx={{ p: 2 }}>
                  <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center' }}>
                    {account.userConfig.stripe_subscription_active ? (
                      <Box sx={{ alignItems: 'center', justifyContent: 'center' }}>
                        <Typography variant="h6" gutterBottom>Subscription Active</Typography>
                        <Typography variant="subtitle1" gutterBottom>Helix Premium : $20.00 / month</Typography>
                        <Typography variant="body2" gutterBottom>You have priority access to the Helix GPU cloud</Typography>
                        <Button variant="contained" color="primary" sx={{ mt: 2 }} onClick={handleManage}>
                          Manage Subscription
                        </Button>
                      </Box>
                    ) : (
                      <Box sx={{ alignItems: 'center', justifyContent: 'center' }}>
                        <Typography variant="h6" gutterBottom>Helix Premium</Typography>
                        <Typography variant="subtitle1" gutterBottom>$20.00 / month</Typography>
                        <Typography variant="body2" gutterBottom>Get priority access to the Helix GPU cloud</Typography>
                        <Button variant="contained" color="primary" sx={{ mt: 2 }} onClick={handleSubscribe}>
                          Start Subscription
                        </Button>
                      </Box>
                    )}
                  </Box>
                </Paper>
              </Grid>
            )}
          </Grid>
        </Box>
      </Box>
    </Container>
  )
}

export default Account
