import React, { FC, useEffect, useCallback } from 'react'
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

import Page from '../components/system/Page'
import DeleteIcon from '@mui/icons-material/Delete'
import CopyIcon from '@mui/icons-material/CopyAll'

import useSnackbar from '../hooks/useSnackbar'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'

const Account: FC = () => {
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()

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
      snackbar.success('Subscription successful')
    }
  }, [])

  useEffect(() => {
    if(!account.token) {
      return
    }
    account.loadApiKeys({
      types: 'api',
    })
  }, [
    account.token,
  ])

  if(!account.user) return null
  if(!account.apiKeys) return null
  // Get API key
  const apiKey = account.apiKeys.length > 0 ? account.apiKeys[0].key : ''

  // TODO: replace with 
  // https://www.npmjs.com/package/@readme/httpsnippet
  // and have a selector for python/javascript/curl/golang
  const curlExample = `curl --request POST \\
  --url ${window.location.protocol}//${window.location.host}/api/v1/sessions/chat \\
  --header 'Authorization: Bearer ${apiKey}' \\
  --header 'Content-Type: application/json' \\
  --data '{
    "model": "${account.models[0].id}",
    "session_id": "",
    "system": "you are an intelligent assistant that helps with geography",
    "messages": [
      {
        "role": "user",
        "content": { "content_type": "text", "parts": ["where are the Faroe islands located?"] }
      }
    ]
  }'`

  const openAICurlExample = `curl --request POST \\
  --url ${window.location.protocol}//${window.location.host}/v1/chat/completions \\
  --header 'Authorization: Bearer ${apiKey}' \\
  --header 'Content-Type: application/json' \\
  --data '{
    "model": "${account.models[0].id}",
    "stream": false,
    "messages": [
      { "role": "system", "content": "You are a helpful assistant." },
      { "role": "user", "content": "how big was the roman empire?" }
    ]
  }'`

  const openAIAzureEnvVars = `export AZURE_OPENAI_ENDPOINT=${window.location.protocol}//${window.location.host}
export AZURE_OPENAI_API_BASE=${window.location.protocol}//${window.location.host}
export AZURE_OPENAI_API_KEY=${apiKey}
`
  const cliInstall = `curl -Ls -O https://get.helix.ml/install.sh && bash install.sh --cli`

  const cliLogin = `export HELIX_URL=${window.location.protocol}//${window.location.host}
export HELIX_API_KEY=${apiKey}
`

  return (
    <Page
      breadcrumbTitle="Account"
    >
      <Container maxWidth="lg">
        <Box sx={{ width: '100%', maxHeight: '100%', display: 'flex', flexDirection: 'row', alignItems: 'center', justifyContent: 'center' }}>
          <Box sx={{ width: '100%', flexGrow: 1, overflowY: 'auto', px: 2 }}>
            <Grid container spacing={2}>
              {paymentsActive && (
                <>
                <Grid item xs={12} md={colSize}>
                  <Typography variant="h4" gutterBottom sx={{mt:4}}>Billing</Typography>
                  <Paper sx={{ p: 2 }}>
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'left', justifyContent: 'center' }}>
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
                </Grid></>
              )}
              <Grid item xs={12} md={colSize}>
                <Typography variant="h4" gutterBottom sx={{mt:4}}>API Keys</Typography>
                <Paper sx={{ p: 0 }}>
                  <Typography sx={{ p: 2}} variant="h6">API Keys</Typography>
                  <List>
                  <ListItem >
                    <ListItemText 
                      primary={'Authenticating to the API'} 
                      secondary={`Specify your key as a header 'Authorization: Bearer <token>' with every request`} />

                  </ListItem>
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

                <Paper sx={{ mt: 2 }}>
                  <Typography sx={{ p: 2}} variant="h6">CLI install &amp; login</Typography>
                  <List>
                    <ListItem>
                      <ListItemText                         
                        secondary={'To install the Helix CLI, run:'} />
                    </ListItem>
                    <ListItem>
                      <Typography component="pre" 
                          sx={{
                          wordBreak: 'break-all',
                          wordWrap: 'break-all',
                          whiteSpace: 'pre-wrap',
                          fontSize: '0.8rem',                      
                          ml: 0,
                          fontFamily: "monospace",
                          }}
                      >
                        {cliInstall}                  
                      </Typography>
                      <ListItemSecondaryAction>
                        <CopyToClipboard text={cliInstall} onCopy={() => snackbar.success('Copied to clipboard')}>
                          <IconButton edge="end" aria-label="copy" sx={{ mr: 2 }}>
                            <CopyIcon />
                          </IconButton>
                        </CopyToClipboard>                          
                      </ListItemSecondaryAction>
                    </ListItem>

                    <ListItem>
                      <ListItemText                         
                        secondary={'Set authentication credentials:'} />
                    </ListItem>
                    
                    {account.apiKeys.map((apiKey) => (
                      <ListItem key={apiKey.key}>
                        <Typography component="pre" 
                            sx={{
                            wordBreak: 'break-all',
                            wordWrap: 'break-all',
                            whiteSpace: 'pre-wrap',
                            fontSize: '0.8rem',                                                  
                            fontFamily: "monospace",
                            }}
                        >
                          {cliLogin}                  
                        </Typography>
                        <ListItemSecondaryAction>
                          <CopyToClipboard text={cliLogin} onCopy={() => snackbar.success('Copied to clipboard')}>
                            <IconButton edge="end" aria-label="copy" sx={{ mr: 2 }}>
                              <CopyIcon />
                            </IconButton>
                          </CopyToClipboard>                          
                        </ListItemSecondaryAction>
                      </ListItem>
                    ))}
                  </List>
                </Paper>

                <Typography variant="h4" gutterBottom sx={{mt:4}}>API Docs</Typography>
                <Paper sx={{ p: 2 }}>
                  <Typography variant="h6">Text generation</Typography>
                  <List>
                  <ListItem >
                    <ListItemText 
                      primary={'Helix session chat'} 
                      secondary={'Provides an easy way to set system prompt, fine-tuning adapters. Supply session_id to continue existing session.'} />
                    
                    <ListItemSecondaryAction sx={{ pr: 4 }}>
                      <CopyToClipboard text={curlExample} onCopy={() => snackbar.success('Copied to clipboard')}>
                        <IconButton edge="end" aria-label="copy">
                          <CopyIcon />
                        </IconButton>
                      </CopyToClipboard>
                    </ListItemSecondaryAction>

                  </ListItem>
                  </List>
                  <Typography component="pre" 
                      sx={{
                      wordBreak: 'break-all',
                      wordWrap: 'break-all',
                      whiteSpace: 'pre-wrap',
                      fontSize: '0.8rem',                      
                      ml: 2,
                      fontFamily: "monospace"
                      }}
                  >
                    {curlExample}
                  </Typography>
                  <List>
                    <ListItem >
                      <ListItemText                     
                        primary={'OpenAI chat'} 
                        secondary={'Each API call creates a new Helix session, provide multiple messages to keep the context.'} />
                      <ListItemSecondaryAction sx={{ pr: 4 }}>
                        <CopyToClipboard text={openAICurlExample} onCopy={() => snackbar.success('Copied to clipboard')}>
                          <IconButton edge="end" aria-label="copy">
                            <CopyIcon />
                          </IconButton>
                        </CopyToClipboard>
                      </ListItemSecondaryAction>
                    </ListItem>
                  </List>
                  
                  <Typography component="pre" 
                      sx={{
                      wordBreak: 'break-all',
                      wordWrap: 'break-all',
                      whiteSpace: 'pre-wrap',
                      fontSize: '0.8rem',                      
                      ml: 2,
                      fontFamily: "monospace"
                      }}
                  >
                    {openAICurlExample}                  
                  </Typography>

                  <List>
                    <ListItem >
                      <ListItemText                     
                        primary={'OpenAI SDK compatibility'} 
                        secondary={'Use any OpenAI SDK or tool with the following environment variables or settings to connect to Helix.'} />
                      <ListItemSecondaryAction sx={{ pr: 4 }}>
                        <CopyToClipboard text={openAIAzureEnvVars} onCopy={() => snackbar.success('Copied to clipboard')}>
                          <IconButton edge="end" aria-label="copy">
                            <CopyIcon />
                          </IconButton>
                        </CopyToClipboard>
                      </ListItemSecondaryAction>
                    </ListItem>
                  </List>

                  <Typography component="pre" 
                      sx={{
                      wordBreak: 'break-all',
                      wordWrap: 'break-all',
                      whiteSpace: 'pre-wrap',
                      fontSize: '0.8rem',                      
                      ml: 2,
                      fontFamily: "monospace",
                      }}
                  >
                    {openAIAzureEnvVars}                  
                  </Typography>
                  
                </Paper>
              </Grid>
            </Grid>
          </Box>
        </Box>
      </Container>
    </Page>
  )
}

export default Account
