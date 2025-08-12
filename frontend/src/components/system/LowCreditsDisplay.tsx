import React from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import WarningIcon from '@mui/icons-material/Warning'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'
import { useGetConfig } from '../../services/userService'
import { useGetWallet } from '../../services/useBilling'

const LOW_BALANCE_THRESHOLD = 5.0

const LowCreditsDisplay: React.FC = () => {  
  const router = useRouter()
  const lightTheme = useLightTheme()
  const account = useAccount()
  
  const { data: serverConfig, isLoading: isLoadingServerConfig } = useGetConfig()
  
  // Get organization context from router
  const orgId = router.params.org_id
  const isOrgContext = Boolean(orgId)
  
  // Fetch appropriate wallet based on context
  const { data: wallet, isLoading: isLoadingWallet } = useGetWallet(orgId)
  
  // While loading, don't render anything
  if (isLoadingServerConfig || isLoadingWallet) {
    return null
  }

  // Only show if billing is enabled
  if (!serverConfig?.billing_enabled) {
    return null
  }

  // Only show if wallet exists and balance is below $1
  if (!wallet || (wallet.balance && wallet.balance >= LOW_BALANCE_THRESHOLD)) {
    return null
  }

  const handleBillingClick = () => {
    if (isOrgContext && orgId) {
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
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 1, maxWidth: '100%' }}>
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
        maxWidth: '100%',
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
        maxWidth: '100%',
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