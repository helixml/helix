import React, { FC, useCallback, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import ClaudeSubscription from './ClaudeSubscription'
import QuotaListView from '../quota/QuotaListView'
import TokenUsage from '../usage/TokenUsage'
import TotalCost from '../usage/TotalCost'
import TotalRequests from '../usage/TotalRequests'

import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import useLightTheme from '../../hooks/useLightTheme'
import useThemeConfig from '../../hooks/useThemeConfig'
import { useGetQuota } from '../../services/quotaService'
import {
  useGetConfig,
  useGetUserUsage,
  useUpdateAccount,
} from '../../services/userService'
import { TypesAuthProvider } from '../../api/api'

interface GeneralSettingsProps {
  onOpenPasswordDialog: () => void
}

const GeneralSettings: FC<GeneralSettingsProps> = ({ onOpenPasswordDialog }) => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const panelBg = lightTheme.isLight ? lightTheme.panelColor : themeConfig.darkPanel

  const { data: usage } = useGetUserUsage()
  const { data: serverConfig } = useGetConfig()
  const { data: quotas } = useGetQuota()
  const updateAccount = useUpdateAccount()

  const [fullName, setFullName] = useState<string>(account.user?.name || '')

  useEffect(() => {
    setFullName(account.user?.name || '')
  }, [account.user?.name])

  const handleFullNameBlur = useCallback(async () => {
    const currentFullName = account.user?.name || ''
    if (fullName !== currentFullName && fullName.trim() !== '') {
      try {
        await updateAccount.mutateAsync({ full_name: fullName.trim() })
        snackbar.success('Profile name has been updated')
      } catch (error) {
        console.error('Failed to update name:', error)
        snackbar.error('Failed to update name')
        setFullName(currentFullName)
      }
    }
  }, [fullName, account.user, updateAccount, snackbar])

  return (
    <>
      <Grid container spacing={2} sx={{ mb: 2, backgroundColor: panelBg, p: 2, borderRadius: 2 }}>
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

      <ClaudeSubscription />

      <Grid container spacing={2} sx={{ mt: 2, backgroundColor: panelBg, p: 2, borderRadius: 2 }}>
        <Grid item xs={12}>
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Typography variant="h6" gutterBottom>Full Name</Typography>
            <form autoComplete="off" style={{ width: '50%' }}>
              <TextField
                fullWidth
                value={fullName}
                autoComplete="name"
                data-form-type="other"
                onChange={(e) => setFullName(e.target.value)}
                onBlur={handleFullNameBlur}
                variant="outlined"
                disabled={updateAccount.isPending}
              />
            </form>
          </Box>
        </Grid>
      </Grid>

      {serverConfig?.auth_provider === TypesAuthProvider.AuthProviderRegular && (
        <Grid container spacing={2} sx={{ mt: 2, backgroundColor: panelBg, p: 2, borderRadius: 2 }}>
          <Grid item xs={12}>
            <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Typography variant="h6" gutterBottom>Update Password</Typography>
              <Button
                variant="contained"
                color="secondary"
                onClick={onOpenPasswordDialog}
              >
                Update Password
              </Button>
            </Box>
          </Grid>
        </Grid>
      )}

      {quotas && (
        <Grid container spacing={2} sx={{ mt: 2, backgroundColor: panelBg, p: 2, borderRadius: 2 }}>
          <Grid item xs={12}>
            <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>Quotas</Typography>
            <QuotaListView />
          </Grid>
        </Grid>
      )}
    </>
  )
}

export default GeneralSettings
