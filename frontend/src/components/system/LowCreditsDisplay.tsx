import React, { useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import CloseIcon from '@mui/icons-material/Close'
import WarningIcon from '@mui/icons-material/Warning'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'
import { useGetConfig } from '../../services/userService'
import { useGetWallet } from '../../services/useBilling'
import { getWithTTL, setWithTTL } from '../../utils/localStorage'

const LOW_BALANCE_THRESHOLD = 5.0
const DISMISS_STORAGE_KEY = 'low-credits-dismissed'
const DISMISS_TTL_HOURS = 12

const LowCreditsDisplay: React.FC = () => {  
  const router = useRouter()
  const lightTheme = useLightTheme()
  const account = useAccount()
  const [isDismissed, setIsDismissed] = useState(false)
  
  const { data: serverConfig, isLoading: isLoadingServerConfig } = useGetConfig()
  
  // Get organization context from router
  const orgId = router.params.org_id
  const isOrgContext = Boolean(orgId)
  
  // Fetch appropriate wallet based on context
  const { data: wallet, isLoading: isLoadingWallet } = useGetWallet(orgId, !isLoadingServerConfig && serverConfig?.billing_enabled)

  // Check if dialog was dismissed
  useEffect(() => {
    const dismissed = getWithTTL<boolean>(DISMISS_STORAGE_KEY)
    if (dismissed) {
      setIsDismissed(true)
    }
  }, [])

  // Handle dismiss
  const handleDismiss = () => {
    setWithTTL(DISMISS_STORAGE_KEY, true, DISMISS_TTL_HOURS)
    setIsDismissed(true)
  }
  
  // While loading, don't render anything
  if (isLoadingServerConfig || isLoadingWallet) {
    return null
  }

  // Only show if billing is enabled
  if (!serverConfig?.billing_enabled) {
    return null
  }

  // Only show if wallet exists and balance is below threshold
  if (!wallet || (wallet.balance && wallet.balance >= LOW_BALANCE_THRESHOLD)) {
    return null
  }

  // Don't show if dismissed
  if (isDismissed) {
    return null
  }

  const handleBillingClick = () => {
    if (isOrgContext && wallet.org_id) {
      // Navigate to organization billing page
      router.navigate('org_billing', { org_id: orgId })
    } else {
      // Navigate to personal account/billing page
      router.navigate('account')
    }
  }

  const getContextLabel = () => {
    if (isOrgContext && account.organizationTools.organization) {
      return account.organizationTools.organization.display_name || account.organizationTools.organization.name
    }
    return 'Personal'
  }

  const getBillingButtonText = () => {
    if (isOrgContext) {
      return 'Manage Billing'
    }
    return 'Add Credits'
  }

  return (    
    <Box
      sx={{
        px: 2,
        py: 1.5,
        mx: 1,
        mb: 1,
        mt: 1,        
        overflow: 'hidden',
        backgroundColor: lightTheme.backgroundColor,
        border: lightTheme.border,
        borderRadius: 1,
        borderColor: 'warning.main',
        borderWidth: 1,
        position: 'relative',
      }}
    >
      {/* Close button */}
      <IconButton
        onClick={handleDismiss}
        size="small"
        sx={{
          position: 'absolute',
          top: 8,
          right: 8,
          color: 'warning.main',
          '&:hover': {
            backgroundColor: 'rgba(255, 152, 0, 0.1)',
          },
        }}
      >
        <CloseIcon fontSize="small" />
      </IconButton>

      <Box sx={{ display: 'flex', alignItems: 'center', mb: 1, maxWidth: 'calc(100% - 40px)' }}>
        <WarningIcon sx={{ color: 'warning.main', mr: 1, fontSize: 20, flexShrink: 0 }} />
        <Typography variant="body2" sx={{ 
          color: 'warning.main', 
          fontWeight: 'bold',
          maxWidth: '100%',
          overflow: 'hidden'
        }}>
          Low Credits Warning
        </Typography>
      </Box>
      
      <Typography variant="body2" sx={{ 
        color: lightTheme.textColor, 
        mb: 1,
        maxWidth: 'calc(100% - 40px)',
        wordBreak: 'break-word',
        overflowWrap: 'break-word',
        whiteSpace: 'normal',
        overflow: 'hidden'
      }}>
        Your {getContextLabel()} wallet balance is low: <strong>${wallet.balance?.toFixed(2) || '0.00'}</strong>
      </Typography>
      
      <Typography variant="caption" sx={{ 
        color: lightTheme.textColorFaded, 
        mb: 1.5, 
        display: 'block',
        maxWidth: 'calc(100% - 40px)',
        wordBreak: 'break-word',
        overflowWrap: 'break-word',
        whiteSpace: 'normal',
        overflow: 'hidden'
      }}>
        Add credits to continue using Helix services without interruption.
      </Typography>
      
      <Button
        onClick={handleBillingClick}
        size="small"
        variant="contained"
        sx={{
          width: '100%',
          backgroundColor: 'warning.main',
          color: '#000',
          fontSize: '0.8rem',
          fontWeight: 'bold',
          textTransform: 'none',
          '&:hover': {
            backgroundColor: 'warning.dark',
          },
        }}
      >
        {getBillingButtonText()}
      </Button>
    </Box>
  )
}

export default LowCreditsDisplay 