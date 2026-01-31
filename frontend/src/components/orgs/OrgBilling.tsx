import React, { FC, useCallback, useState } from 'react'
import Container from '@mui/material/Container'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Grid from '@mui/material/Grid'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import CircularProgress from '@mui/material/CircularProgress'

import Page from '../system/Page'
import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetWallet } from '../../services/useBilling'
import { useGetOrgUsage } from '../../services/orgService'
import TokenUsage from '../usage/TokenUsage'
import TotalCost from '../usage/TotalCost'
import TotalRequests from '../usage/TotalRequests'
import useThemeConfig from '../../hooks/useThemeConfig'
import { useGetConfig } from '../../services/userService'

// Extended wallet interface to include subscription fields
interface ExtendedWallet {
  subscription_status?: string
  subscription_current_period_end?: number
  subscription_current_period_start?: number
  subscription_created?: number
  stripe_subscription_id?: string
  balance?: number
  created_at?: string
  id?: string
  org_id?: string
  stripe_customer_id?: string
  updated_at?: string
  user_id?: string
}

const OrgBilling: FC = () => {
  const account = useAccount()
  const api = useApi()
  const client = api.getApiClient()
  const router = useRouter()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  
  const orgId = router.params.org_id
  const organization = account.organizationTools.organization

  const { data: serverConfig, isLoading: isLoadingServerConfig } = useGetConfig()
  
  const { data: wallet } = useGetWallet(orgId, !isLoadingServerConfig && serverConfig?.billing_enabled)
  const { data: usage } = useGetOrgUsage(orgId || '', !!orgId)
  
  const [topUpAmount, setTopUpAmount] = useState<number>(10)
  const [isSubscribing, setIsSubscribing] = useState<boolean>(false)
  const [isToppingUp, setIsToppingUp] = useState<boolean>(false)

  const handleSubscribe = useCallback(async () => {
    if (!orgId) {
      snackbar.error('Organization not found')
      return
    }
    
    try {
      setIsSubscribing(true)
      
      const resp = await client.v1SubscriptionNewCreate({
        org_id: orgId
      })
      if (!resp.data) return

      document.location = resp.data
    } catch (error) {
      console.error('Subscription error:', error)
      snackbar.error('Failed to start subscription process')
    } finally {
      setIsSubscribing(false)
    }
  }, [client, orgId, snackbar])

  const handleManage = useCallback(async () => {
    if (!orgId) {
      snackbar.error('Organization not found')
      return
    }
    
    const resp = await client.v1SubscriptionManageCreate({
      org_id: orgId
    })
    if (!resp.data) return
    document.location = resp.data
  }, [api, orgId, snackbar])

  const handleTopUp = useCallback(async () => {
    if (!orgId) {
      snackbar.error('Organization not found')
      return
    }

    try {
      setIsToppingUp(true)
      
      const resp = await client.v1TopUpsNewCreate({
        amount: topUpAmount,
        org_id: orgId
      })
      if (!resp.data) return
      document.location = resp.data
    } catch (error) {
      console.error('Top-up error:', error)
      snackbar.error('Failed to start top-up process')
    } finally {
      setIsToppingUp(false)
    }
  }, [client, orgId, topUpAmount, snackbar])

  if (!account.user || !organization || isLoadingServerConfig) {
    return null
  }

  const paymentsActive = serverConfig?.stripe_enabled && serverConfig?.billing_enabled
  const colSize = paymentsActive ? 6 : 12

  // Check if user has admin permissions for this org
  const isReadOnly = !account.isOrgAdmin

  // Check if subscription is active based on wallet status from backend
  // The wallet contains subscription information from Stripe stored in the database
  const extendedWallet = wallet as ExtendedWallet
  const isSubscriptionActive = extendedWallet?.subscription_status === 'active'

  return (
    <Page
      breadcrumbTitle="Billing"
      breadcrumbShowHome={true}
      orgBreadcrumbs={true}
    >
      <Container maxWidth="lg" sx={{ mb: 4 }}>
        <Box sx={{ width: '100%', maxHeight: '100%', display: 'flex', flexDirection: 'row', alignItems: 'center', justifyContent: 'center' }}>
          <Box sx={{ width: '100%', flexGrow: 1, overflowY: 'auto', px: 2 }}>
            <Typography variant="h4" gutterBottom sx={{ mt: 4, mb: 4  }}>
              Organization Billing
              {isReadOnly && (
                <Typography variant="caption" color="text.secondary" sx={{ ml: 2 }}>
                  (Read-only: Admin privileges required to make changes)
                </Typography>
              )}
            </Typography>

            {/* Usage Charts Row */}
            {usage && (
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
            )}

            {/* Billing Section */}
            {paymentsActive && (
              <Grid container spacing={2} sx={{ mt: 2, backgroundColor: themeConfig.darkPanel, p: 2, borderRadius: 2 }}>
                <Grid item xs={12} md={colSize}>
                  <Box sx={{ p: 2, height: 250, display: 'flex', flexDirection: 'column', backgroundColor: 'transparent', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                      <Box sx={{ flex: 1 }}>
                        <Typography variant="h6" gutterBottom>Organization Balance</Typography>
                        <Typography variant="h4" gutterBottom color="primary">
                          ${extendedWallet?.balance?.toFixed(2) || '0.00'} credits
                        </Typography>
                        <Typography variant="body2" color="text.secondary" gutterBottom>
                          Available credits for {organization.display_name || organization.name}
                        </Typography>
                      </Box>

                      {!isReadOnly && (
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
                              <MenuItem value={500}>$500</MenuItem>
                              <MenuItem value={1000}>$1000</MenuItem>
                            </Select>
                          </FormControl>
                          <Button 
                            variant="contained" 
                            color="secondary" 
                            onClick={handleTopUp} 
                            disabled={isToppingUp}
                            startIcon={isToppingUp ? <CircularProgress size={16} color="inherit" /> : undefined}
                            sx={{ minWidth: 140 }}
                          >
                            {isToppingUp ? 'Processing...' : 'Add Credits'}
                          </Button>
                        </Box>
                      )}
                    </Box>
                  </Box>
                </Grid>
                
                <Grid item xs={12} md={colSize}>
                  <Box sx={{ p: 2, height: 250, display: 'flex', flexDirection: 'column', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                      {isSubscriptionActive ? (
                        <>
                          <Box sx={{ flex: 1 }}>
                            <Typography variant="h6" gutterBottom>Subscription</Typography>
                            <Typography variant="h4" gutterBottom color="primary">Helix Business</Typography>
                            <Typography variant="body2" gutterBottom>
                              Enhanced quotas, priority support, and advanced organization features for teams.
                            </Typography>
                            {extendedWallet?.subscription_current_period_end && (
                              <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
                                Next billing: {new Date(extendedWallet.subscription_current_period_end * 1000).toLocaleDateString()}
                              </Typography>
                            )}
                          </Box>
                          
                          {!isReadOnly && (
                            <Box sx={{ display: 'flex', mb: 1, justifyContent: 'flex-end' }}>
                              <Button variant="contained" color="secondary" sx={{ minWidth: 140 }} onClick={handleManage}>
                                Manage Subscription
                              </Button>
                            </Box>
                          )}
                        </>
                      ) : (
                        <>
                          <Box sx={{ flex: 1 }}>
                            <Typography variant="h6" gutterBottom>Subscription</Typography>
                            <Typography variant="h4" gutterBottom color="text.secondary">Free</Typography>
                            <Typography variant="body2" color="text.secondary"  gutterBottom>
                              Subscribe to Helix Business for enhanced quotas, priority support, and advanced organization features.
                              Monthly fee is converted to credits and added to your balance.
                            </Typography>
                          </Box>
                          
                          {!isReadOnly && (
                            <Box sx={{ display: 'flex', mb: 1, justifyContent: 'flex-end' }}>
                              <Button 
                                variant="contained" 
                                color="secondary" 
                                onClick={handleSubscribe}
                                disabled={isSubscribing}
                                startIcon={isSubscribing ? <CircularProgress size={16} color="inherit" /> : undefined}
                                sx={{ minWidth: 140 }}
                              >
                                {isSubscribing ? 'Processing...' : 'Start Subscription ($399/m)'}
                              </Button>
                            </Box>
                          )}
                        </>
                      )}
                    </Box>
                  </Box>
                </Grid>
              </Grid>
            )}

            {!paymentsActive && (
              <Box sx={{ mt: 2, p: 3, backgroundColor: themeConfig.darkPanel, borderRadius: 2, textAlign: 'center' }}>
                <Typography variant="h6" gutterBottom>
                  Billing Not Available
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  Billing features are not currently enabled on this Helix instance.
                </Typography>
              </Box>
            )}
          </Box>
        </Box>
      </Container>
    </Page>
  )
}

export default OrgBilling
