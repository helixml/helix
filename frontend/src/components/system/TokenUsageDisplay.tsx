import React from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import LinearProgress from '@mui/material/LinearProgress'
import Tooltip from '@mui/material/Tooltip'
import Button from '@mui/material/Button'
import InfoIcon from '@mui/icons-material/Info'
import UpgradeIcon from '@mui/icons-material/Upgrade'
import KeyIcon from '@mui/icons-material/Key'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useLightTheme from '../../hooks/useLightTheme'
import { useGetUserTokenUsage } from '../../services/userService'
import { useListProviders } from '../../services/providersService'

const TokenUsageDisplay: React.FC = () => {
  const router = useRouter()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const { data: tokenUsage, isLoading: loading, error } = useGetUserTokenUsage()

  // Load providers
  const { data: providers, isLoading: loadingProviders } = useListProviders({
    loadModels: false,
    orgId: router.params.org_id,
  })

  // While loading, don't render anything
  if (loadingProviders) {
    return null
  }

  // If there are user providers, don't show the upgrade button
  const hasUserProviders = providers?.some(provider => provider.endpoint_type === 'user')

  if (hasUserProviders) {
    return null
  }

  const handleUpgrade = () => {
    // Navigate to the account page for billing/upgrade
    router.navigate('account')
  }

  const handleAddProviders = () => {
    // Navigate to the providers page
    router.navigate('providers')
  }

  // If loading, error, no data, or quotas not enabled, don't render
  if (loading || error || !tokenUsage || !tokenUsage.quotas_enabled) {
    return null
  }

  const formatNumber = (num: number) => {
    if (num >= 1000000) {
      return `${(num / 1000000).toFixed(1)}M`
    }
    if (num >= 1000) {
      return `${(num / 1000).toFixed(1)}K`
    }
    return num.toString()
  }

  const getProgressColor = (percentage: number) => {
    if (percentage >= 90) return 'error'
    if (percentage >= 75) return 'warning'
    return 'primary'
  }

  const tierName = tokenUsage.is_pro_tier ? 'Pro' : 'Free'
  const shouldShowUpgrade = !tokenUsage.is_pro_tier && tokenUsage.usage_percentage && tokenUsage.usage_percentage >= 50 // Show upgrade button when 50%+ used
  const isLimitReached = tokenUsage.usage_percentage && tokenUsage.usage_percentage >= 100

  return (    
    <Box
      sx={{
        px: 2,
        py: 1.5,
        mx: 1,
        mb: 1,
        mt: 1,
        backgroundColor: lightTheme.backgroundColor,
        border: lightTheme.border,
        borderRadius: 1,
      }}
    >
      <Box sx={{ display: 'flex', alignItems: 'center', mb: 0.5 }}>
        <Typography variant="caption" sx={{ color: lightTheme.textColorFaded, fontWeight: 'bold' }}>
          {tierName} Plan - Monthly Tokens
        </Typography>
        <Tooltip
          title={account.serverConfig.providers_management_enabled
            ? `You've used ${formatNumber(tokenUsage.monthly_usage ?? 0)} out of ${formatNumber(tokenUsage.monthly_limit ?? 0)} tokens this month using Helix providers. Add your own LLM API keys to avoid running out of tokens.`
            : `You've used ${formatNumber(tokenUsage.monthly_usage ?? 0)} out of ${formatNumber(tokenUsage.monthly_limit ?? 0)} tokens this month using Helix providers.`
          }
          placement="top"
        >
          <InfoIcon sx={{ fontSize: 14, ml: 0.5, color: lightTheme.textColorFaded }} />
        </Tooltip>
      </Box>
      
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 0.5 }}>
        <Typography variant="body2" sx={{ color: lightTheme.textColor, fontSize: '0.75rem' }}>
          {formatNumber(tokenUsage.monthly_usage ?? 0)} / {formatNumber(tokenUsage.monthly_limit ?? 0)}
        </Typography>
        <Typography variant="body2" sx={{ color: lightTheme.textColorFaded, fontSize: '0.75rem' }}>
          {tokenUsage.usage_percentage?.toFixed(1) ?? '0.0'}%
        </Typography>
      </Box>
      
      <LinearProgress
        variant="determinate"
        value={Math.min(tokenUsage.usage_percentage ?? 0, 100)}
        color={getProgressColor(tokenUsage.usage_percentage ?? 0)}
        sx={{
          height: 6,
          borderRadius: 3,
          backgroundColor: 'rgba(255, 255, 255, 0.1)',
          '& .MuiLinearProgress-bar': {
            borderRadius: 3,
          },
        }}
      />
      
      {tokenUsage.usage_percentage && tokenUsage.usage_percentage >= 90 && (
        <Typography 
          variant="caption" 
          sx={{ 
            color: tokenUsage.usage_percentage && tokenUsage.usage_percentage >= 100 ? 'error.main' : 'warning.main',
            fontSize: '0.7rem',
            mt: 0.5,
            display: 'block',
            wordWrap: 'break-word',
            overflowWrap: 'break-word',
            whiteSpace: 'normal'
          }}
        >
          {tokenUsage.usage_percentage && tokenUsage.usage_percentage >= 100
            ? (account.serverConfig.providers_management_enabled
                ? 'Limit reached! Upgrade or add your own LLM API keys to continue.'
                : 'Limit reached! Upgrade to continue.')
            : (account.serverConfig.providers_management_enabled
                ? 'Approaching limit. Consider upgrading or adding your own LLM API keys.'
                : 'Approaching limit. Consider upgrading.')
          }
        </Typography>
      )}

      {isLimitReached && (
        <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1, mt: 1 }}>
          <Button
            onClick={handleUpgrade}
            size="small"
            variant="contained"
            startIcon={<UpgradeIcon />}
            sx={{
              width: '100%',
              backgroundColor: '#00E5FF',
              color: '#000',
              fontSize: '0.7rem',
              fontWeight: 'bold',
              textTransform: 'none',
              '&:hover': {
                backgroundColor: '#00B8CC',
              },
            }}
          >
            Upgrade to Pro
          </Button>
          {account.serverConfig.providers_management_enabled && (
            <Button
              onClick={handleAddProviders}
              size="small"
              variant="outlined"
              startIcon={<KeyIcon />}
              sx={{
                width: '100%',
                borderColor: '#00E5FF',
                color: '#00E5FF',
                fontSize: '0.7rem',
                fontWeight: 'bold',
                textTransform: 'none',
                '&:hover': {
                  borderColor: '#00B8CC',
                  backgroundColor: 'rgba(0, 229, 255, 0.1)',
                },
              }}
            >
              Add my own API Keys
            </Button>
          )}
        </Box>
      )}

      {shouldShowUpgrade && !isLimitReached && (
        <Button
          onClick={handleUpgrade}
          size="small"
          variant="contained"
          startIcon={<UpgradeIcon />}
          sx={{
            mt: 1,
            width: '100%',
            backgroundColor: '#00E5FF',
            color: '#000',
            fontSize: '0.7rem',
            fontWeight: 'bold',
            textTransform: 'none',
            '&:hover': {
              backgroundColor: '#00B8CC',
            },
          }}
        >
          {tokenUsage.usage_percentage && tokenUsage.usage_percentage >= 90 
            ? 'Upgrade Now' 
            : `Upgrade to Pro (${formatNumber(2500000)} tokens/month)`
          }
        </Button>
      )}
    </Box>
  )
}

export default TokenUsageDisplay 