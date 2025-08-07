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
import Grid from '@mui/material/Grid'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import TextField from '@mui/material/TextField'
import InputAdornment from '@mui/material/InputAdornment'
import VisibilityIcon from '@mui/icons-material/Visibility'
import VisibilityOffIcon from '@mui/icons-material/VisibilityOff'

import Page from '../components/system/Page'
import DeleteIcon from '@mui/icons-material/Delete'
import CopyIcon from '@mui/icons-material/CopyAll'

import useSnackbar from '../hooks/useSnackbar'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'

import { useGetUserWallet } from '../services/useBilling'
import { useGetUserUsage } from '../services/userService'
import TokenUsage from '../components/usage/TokenUsage'
import TotalCost from '../components/usage/TotalCost'
import TotalRequests from '../components/usage/TotalRequests'
import useThemeConfig from '../hooks/useThemeConfig'
import { Prism as SyntaxHighlighterPrism } from 'react-syntax-highlighter'
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism'

const SyntaxHighlighter = SyntaxHighlighterPrism as unknown as React.FC<any>;

const Account: FC = () => {
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const { data: wallet } = useGetUserWallet()
  const [topUpAmount, setTopUpAmount] = useState<number>(20)

  const { data: usage } = useGetUserUsage()
  const [showApiKey, setShowApiKey] = useState(false)

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
    if (!result) return
    document.location = result
  }, [
    account.user,
  ])

  const handleManage = useCallback(async () => {
    const result = await api.post(`/api/v1/subscription/manage`, undefined, {}, {
      loading: true,
      snackbar: true,
    })
    if (!result) return
    document.location = result
  }, [
    account.user,
  ])

  const handleTopUp = useCallback(async () => {
    const result = await api.post(`/api/v1/top-ups/new`, { amount: topUpAmount }, {}, {
      loading: true,
      snackbar: true,
    })
    if (!result) return
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
    if (!account.token) {
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
            <Typography variant="h4" gutterBottom sx={{ mt: 4 }}></Typography>

            {/* Usage Charts Row */}
            <Grid container spacing={2} sx={{ mb: 2, backgroundColor: themeConfig.darkPanel, p: 2, borderRadius: 2 }}>
              <Grid item xs={12} md={4}>
                <TokenUsage usageData={usage ? [{ metrics: usage }] : []} isLoading={false} />
              </Grid>
              <Grid item xs={12} md={4}>
                <TotalCost usageData={usage ? [{ metrics: usage }] : []} isLoading={false} />
              </Grid>
              <Grid item xs={12} md={4}>
                <TotalRequests usageData={usage ? [{ metrics: usage }] : []} isLoading={false} />
              </Grid>
            </Grid>

            <Grid container spacing={2} sx={{ mt: 2, backgroundColor: themeConfig.darkPanel, p: 2, borderRadius: 2 }}>
              {paymentsActive && (
                <>
                  <Grid item xs={12} md={colSize}>
                    <Box sx={{ p: 2, height: 250, display: 'flex', flexDirection: 'column', backgroundColor: 'transparent', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                        <Box sx={{ flex: 1 }}>
                          <Typography variant="h6" gutterBottom>Current Balance</Typography>
                          <Typography variant="h4" gutterBottom color="primary">
                            ${wallet?.balance?.toFixed(2) || '0.00'}
                          </Typography>
                        </Box>

                        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
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
                          <Button variant="contained" color="secondary" onClick={handleTopUp} sx={{ minWidth: 140 }}>
                            Add Credits
                          </Button>
                        </Box>
                      </Box>
                    </Box>
                  </Grid>
                  <Grid item xs={12} md={colSize}>
                    <Box sx={{ p: 2, height: 250, display: 'flex', flexDirection: 'column', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                      <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                        {account.userConfig.stripe_subscription_active ? (
                          <>
                            <Box sx={{ flex: 1 }}>
                              <Typography variant="h6" gutterBottom>Subscription Active</Typography>
                              <Typography variant="h4" gutterBottom color="primary">Helix Premium</Typography>
                              <Typography variant="body2" gutterBottom>You have priority access to the Helix GPU cloud</Typography>
                            </Box>
                            <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                              <Button variant="contained" color="primary" sx={{ minWidth: 140 }} onClick={handleManage}>
                                Manage Subscription
                              </Button>
                            </Box>
                          </>
                        ) : (
                          <>
                            <Box sx={{ flex: 1 }}>
                              <Typography variant="h6" gutterBottom>Helix Premium</Typography>
                              <Typography variant="h4" gutterBottom color="primary">$20.00 / month</Typography>
                              <Typography variant="body2" gutterBottom>Get priority access to the Helix GPU cloud, increase quotas and priority support. Subscription payment will also top-up your Helix credits.</Typography>
                            </Box>
                            <Box sx={{ display: 'flex', mb: 1, justifyContent: 'flex-end' }}>
                              <Button variant="contained" color="secondary" sx={{ minWidth: 140 }} onClick={handleSubscribe}>
                                Start Subscription
                              </Button>
                            </Box>
                          </>
                        )}
                      </Box>
                    </Box>
                  </Grid></>
              )}
            </Grid>

            {/* API keys setup */}
            <Grid container spacing={2} sx={{ mt: 2, backgroundColor: themeConfig.darkPanel, p: 2, borderRadius: 2 }}>
              <Grid item xs={12}>
                {/* <Typography variant="h4" gutterBottom>API Keys</Typography> */}
                
                {/* API Key Display */}
                <Box sx={{ mb: 3 }}>
                  <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>API Key</Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    Specify your key as a header 'Authorization: Bearer &lt;token&gt;' with every request
                  </Typography>
                  
                  {account.apiKeys.map((apiKey) => (
                    <Box key={apiKey.key} sx={{ mb: 2 }}>
                      <TextField
                        fullWidth
                        label="API Key"
                        value={apiKey.key}
                        type={showApiKey ? 'text' : 'password'}
                        variant="outlined"
                        InputProps={{
                          endAdornment: (
                            <InputAdornment position="end">
                              <IconButton
                                onClick={() => setShowApiKey(!showApiKey)}
                                edge="end"
                                sx={{ mr: 0.25 }}
                              >
                                {showApiKey ? <VisibilityOffIcon /> : <VisibilityIcon />}
                              </IconButton>
                              <IconButton
                                onClick={() => handleCopy(apiKey.key)}
                                edge="end"
                                sx={{ mr: 0.25 }}
                              >
                                <CopyIcon />
                              </IconButton>
                              <IconButton
                                onClick={() => handleDeleteApiKey(apiKey.key)}
                                edge="end"
                              >
                                <DeleteIcon />
                              </IconButton>
                            </InputAdornment>
                          ),
                        }}
                      />
                    </Box>
                  ))}
                </Box>

                {/* CLI Installation */}
                <Box sx={{ mb: 3 }}>
                  <Typography variant="h6" gutterBottom>Helix CLI Installation</Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    Install the Helix CLI to interact with the API from your terminal
                  </Typography>
                  
                  <Box sx={{ position: 'relative' }}>
                    <Box sx={{ position: 'absolute', right: 8, top: 8, zIndex: 1 }}>
                      <Button
                        size="small"
                        onClick={() => handleCopy(cliInstall)}
                        startIcon={<CopyIcon />}
                        sx={{
                          backgroundColor: 'rgba(0, 0, 0, 0.6)',
                          '&:hover': {
                            backgroundColor: 'rgba(0, 0, 0, 0.8)',
                          }
                        }}
                      >
                        Copy
                      </Button>
                    </Box>
                    <SyntaxHighlighter
                      language="bash"
                      style={oneDark}
                      customStyle={{
                        margin: 0,
                        borderRadius: '4px',
                      }}
                    >
                      {cliInstall}
                    </SyntaxHighlighter>
                  </Box>
                </Box>

                {/* CLI Authentication */}
                <Box>
                  <Typography variant="h6" gutterBottom>CLI Authentication</Typography>
                  <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                    Set your authentication credentials for the CLI
                  </Typography>
                  
                  {account.apiKeys.map((apiKey) => (
                    <Box key={apiKey.key} sx={{ position: 'relative' }}>
                      <Box sx={{ position: 'absolute', right: 8, top: 8, zIndex: 1 }}>
                        <Button
                          size="small"
                          onClick={() => handleCopy(cliLogin)}
                          startIcon={<CopyIcon />}
                          sx={{
                            backgroundColor: 'rgba(0, 0, 0, 0.6)',
                            '&:hover': {
                              backgroundColor: 'rgba(0, 0, 0, 0.8)',
                            }
                          }}
                        >
                          Copy
                        </Button>
                      </Box>
                      <SyntaxHighlighter
                        language="bash"
                        style={oneDark}
                        customStyle={{
                          margin: 0,
                          borderRadius: '4px',
                        }}
                      >
                        {cliLogin}
                      </SyntaxHighlighter>
                    </Box>
                  ))}
                </Box>
              </Grid>
            </Grid>
          </Box>
        </Box>
      </Container>
    </Page>
  )
}

export default Account
