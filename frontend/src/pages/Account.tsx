import React, { FC, useEffect, useCallback, useState } from 'react'
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
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'

import Page from '../components/system/Page'
import DeleteIcon from '@mui/icons-material/Delete'
import CopyIcon from '@mui/icons-material/CopyAll'

import useSnackbar from '../hooks/useSnackbar'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'

import { useGetUserWallet } from '../services/useBilling'

const Account: FC = () => {
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()
  const { data: wallet } = useGetUserWallet()
  const [topUpAmount, setTopUpAmount] = useState<number>(20)
  
  const handleCopy = useCallback((text: string) => {
    navigator.clipboard.writeText(text)
      .then(() => {
        snackbar.success('Copied to clipboard')
      })
      .catch((error) => {
        console.error('Failed to copy:', error)
        snackbar.error('Failed to copy to clipboard')
      })
  }, [snackbar])

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

  const handleTopUp = useCallback(async () => {
    const result = await api.post(`/api/v1/top-ups/new`, { amount: topUpAmount }, {}, {
      loading: true,
      snackbar: true,
    })
    if(!result) return
    document.location = result
  }, [
    account.user,
    topUpAmount,
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

  if (!account.user || !account.apiKeys || !account.models || !account.serverConfig) {
    return null
  }

  const paymentsActive = account.serverConfig.stripe_enabled
  const colSize = paymentsActive ? 6 : 12

  const apiKey = account.apiKeys.length > 0 ? account.apiKeys[0].key : ''

  const modelId = account.models && account.models.length > 0 ? account.models[0].id : 'default_model'

  const curlExample = `curl --request POST \\
  --url ${window.location.protocol}//${window.location.host}/api/v1/sessions/chat \\
  --header 'Authorization: Bearer ${apiKey}' \\
  --header 'Content-Type: application/json' \\
  --data '{
    "model": "${modelId}",
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
    "model": "${modelId}",
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
  const cliInstall = `curl -Ls -O https://get.helixml.tech/install.sh && bash install.sh --cli`

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
                  <Typography variant="h4" gutterBottom sx={{mt:4}}>Balance</Typography>
                  <Paper sx={{ p: 2 }}>
                    <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'left', justifyContent: 'center' }}>
                      <Box sx={{ alignItems: 'center', justifyContent: 'center' }}>
                        <Typography variant="h6" gutterBottom>Current Balance</Typography>
                        <Typography variant="h4" gutterBottom color="primary">
                          ${wallet?.balance?.toFixed(2) || '0.00'}
                        </Typography>
                        <Typography variant="body2" gutterBottom>Add credits to your account to use premium models and features</Typography>
                        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mt: 2 }}>
                          <FormControl sx={{ minWidth: 120 }}>
                            <InputLabel id="topup-amount-label">Amount</InputLabel>
                            <Select
                              labelId="topup-amount-label"
                              value={topUpAmount}
                              label="Amount"
                              onChange={(e) => setTopUpAmount(e.target.value as number)}
                            >
                              <MenuItem value={5}>$5</MenuItem>
                              <MenuItem value={10}>$10</MenuItem>
                              <MenuItem value={20}>$20</MenuItem>
                              <MenuItem value={50}>$50</MenuItem>
                              <MenuItem value={100}>$100</MenuItem>
                            </Select>
                          </FormControl>
                          <Button variant="contained" color="secondary" onClick={handleTopUp}>
                            Add Credits
                          </Button>
                        </Box>
                      </Box>
                    </Box>
                  </Paper>
                </Grid>
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
                          <Typography variant="body2" gutterBottom>Get priority access to the Helix GPU cloud. Subscription payment will also be converted to Helix credits that you can spend on LLMs.</Typography>
                          <Button variant="contained" color="secondary" sx={{ mt: 2 }} onClick={handleSubscribe}>
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
                          <IconButton 
                            edge="end" 
                            aria-label="copy" 
                            sx={{ mr: 2 }}
                            onClick={() => handleCopy(apiKey.key)}
                          >
                            <CopyIcon />
                          </IconButton>
                          <IconButton edge="end" aria-label="delete" onClick={() => handleDeleteApiKey(apiKey.key)}>
                            <DeleteIcon />
                          </IconButton>
                        </ListItemSecondaryAction>
                      </ListItem>
                    ))}
                  </List>
                </Paper>

                <Paper sx={{ mt: 2 }}>
                  <Typography sx={{ p: 2}} variant="h6">Helix CLI setup</Typography>
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
                        <IconButton 
                          edge="end" 
                          aria-label="copy" 
                          sx={{ mr: 2 }}
                          onClick={() => handleCopy(cliInstall)}
                        >
                          <CopyIcon />
                        </IconButton>                          
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
                          <IconButton 
                            edge="end" 
                            aria-label="copy" 
                            sx={{ mr: 2 }}
                            onClick={() => handleCopy(cliLogin)}
                          >
                            <CopyIcon />
                          </IconButton>                          
                        </ListItemSecondaryAction>
                      </ListItem>
                    ))}
                  </List>
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
