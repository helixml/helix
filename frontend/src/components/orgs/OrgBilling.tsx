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

const OrgBilling: FC = () => {
  const account = useAccount()
  const api = useApi()
  const client = api.getApiClient()
  const router = useRouter()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  
  const orgId = router.params.org_id
  const organization = account.organizationTools.organization
  
  const { data: wallet } = useGetWallet(orgId)
  const { data: usage } = useGetOrgUsage(orgId || '', !!orgId)
  
  const [topUpAmount, setTopUpAmount] = useState<number>(10)

  const handleSubscribe = useCallback(async () => {
    if (!orgId) {
      snackbar.error('Organization not found')
      return
    }
    
    const result = await api.post(`/api/v1/subscription/new`, { org_id: orgId }, {}, {
      loading: true,
      snackbar: true,
    })
    if (!result) return
    document.location = result
  }, [api, orgId, snackbar])

  const handleManage = useCallback(async () => {
    if (!orgId) {
      snackbar.error('Organization not found')
      return
    }
    
    const result = await api.post(`/api/v1/subscription/manage`, { org_id: orgId }, {}, {
      loading: true,
      snackbar: true,
    })
    if (!result) return
    document.location = result
  }, [api, orgId, snackbar])

  const handleTopUp = useCallback(async () => {
    if (!orgId) {
      snackbar.error('Organization not found')
      return
    }

    const resp = await client.v1TopUpsNewCreate({
      amount: topUpAmount,
      org_id: orgId
    })
    if (!resp.data) return
    document.location = resp.data
  }, [api, orgId, topUpAmount, snackbar])

  if (!account.user || !organization) {
    return null
  }

  const paymentsActive = account.serverConfig?.stripe_enabled
  const colSize = paymentsActive ? 6 : 12

  // Check if user has admin permissions for this org
  const isReadOnly = !account.isOrgAdmin

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
                          ${wallet?.balance?.toFixed(2) || '0.00'}
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
                          <Button variant="contained" color="secondary" onClick={handleTopUp} sx={{ minWidth: 140 }}>
                            Add Credits
                          </Button>
                        </Box>
                      )}
                    </Box>
                  </Box>
                </Grid>
                
                <Grid item xs={12} md={colSize}>
                  <Box sx={{ p: 2, height: 250, display: 'flex', flexDirection: 'column', borderRadius: 1, border: '1px solid', borderColor: 'divider' }}>
                    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
                      {/* Note: Organization subscription logic would need to be implemented in the backend */}
                      <Box sx={{ flex: 1 }}>
                        <Typography variant="h6" gutterBottom>Organization Subscription</Typography>
                        <Typography variant="h4" gutterBottom color="primary">Helix Enterprise</Typography>
                        <Typography variant="body2" gutterBottom>
                          Enhanced quotas, priority support, and advanced organization features for teams.
                        </Typography>
                      </Box>
                      
                      {!isReadOnly && (
                        <Box sx={{ display: 'flex', mb: 1, justifyContent: 'flex-end' }}>
                          <Button variant="contained" color="secondary" sx={{ minWidth: 140 }} onClick={handleSubscribe}>
                            Manage Subscription
                          </Button>
                        </Box>
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
