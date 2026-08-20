import React, { FC, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import AccountSubscriptions from './AccountSubscriptions'
import SettingsPanel from './SettingsPanel'
import QuotaListView from '../quota/QuotaListView'
import TokenUsage from '../usage/TokenUsage'
import TotalCost from '../usage/TotalCost'
import TotalRequests from '../usage/TotalRequests'

import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import { useGetQuota } from '../../services/quotaService'
import {
  useGetConfig,
  useGetCurrentUser,
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

  const { data: usage } = useGetUserUsage()
  const { data: serverConfig } = useGetConfig()
  const { data: currentUser } = useGetCurrentUser()
  const { data: quotas } = useGetQuota()
  const updateAccount = useUpdateAccount()

  const savedFullName = currentUser?.name || account.user?.name || ''

  const [fullName, setFullName] = useState<string>(savedFullName)

  useEffect(() => {
    setFullName(savedFullName)
  }, [savedFullName])

  const handleFullNameBlur = async () => {
    if (fullName !== savedFullName && fullName.trim() !== '') {
      try {
        await updateAccount.mutateAsync({ full_name: fullName.trim() })
        snackbar.success('Profile name has been updated')
      } catch (error) {
        console.error('Failed to update name:', error)
        snackbar.error('Failed to update name')
        setFullName(savedFullName)
      }
    }
  }

  return (
    <>
      <SettingsPanel>
        <Box
          sx={{
            display: 'grid',
            // One stat per row on a phone, three across from tablet up.
            gridTemplateColumns: { xs: '1fr', md: 'repeat(3, 1fr)' },
            gap: 2,
          }}
        >
          <TokenUsage usageData={usage ? [{ metrics: usage }] : []} isLoading={false} />
          <TotalCost usageData={usage ? [{ metrics: usage }] : []} isLoading={false} />
          <TotalRequests usageData={usage ? [{ metrics: usage }] : []} isLoading={false} />
        </Box>
      </SettingsPanel>

      <AccountSubscriptions />

      <SettingsPanel>
        <Box
          sx={{
            display: 'flex',
            // Side by side there is no room for a usable text field on a
            // phone, so the label and input stack instead.
            flexDirection: { xs: 'column', sm: 'row' },
            justifyContent: 'space-between',
            alignItems: { xs: 'stretch', sm: 'center' },
            gap: { xs: 1, sm: 2 },
          }}
        >
          <Typography variant="h6">Full Name</Typography>
          <Box component="form" autoComplete="off" sx={{ width: { xs: '100%', sm: '50%' } }}>
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
          </Box>
        </Box>
      </SettingsPanel>

      {serverConfig?.auth_provider === TypesAuthProvider.AuthProviderRegular && (
        <SettingsPanel>
          <Box
            sx={{
              display: 'flex',
              flexDirection: { xs: 'column', sm: 'row' },
              justifyContent: 'space-between',
              alignItems: { xs: 'stretch', sm: 'center' },
              gap: { xs: 1.5, sm: 2 },
            }}
          >
            <Typography variant="h6">Update Password</Typography>
            <Button
              variant="contained"
              color="secondary"
              onClick={onOpenPasswordDialog}
            >
              Update Password
            </Button>
          </Box>
        </SettingsPanel>
      )}

      {quotas && (
        <SettingsPanel sx={{ mb: 0 }}>
          <Typography variant="h6" sx={{ mb: 2 }}>Quotas</Typography>
          <QuotaListView />
        </SettingsPanel>
      )}
    </>
  )
}

export default GeneralSettings
