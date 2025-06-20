import React, { useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import LinearProgress from '@mui/material/LinearProgress'
import Tooltip from '@mui/material/Tooltip'
import Button from '@mui/material/Button'
import InfoIcon from '@mui/icons-material/Info'
import UpgradeIcon from '@mui/icons-material/Upgrade'
import axios from 'axios'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'

interface TokenUsageData {
  quotas_enabled: boolean
  monthly_usage: number
  monthly_limit: number
  is_pro_tier: boolean
  usage_percentage: number
}

const TokenUsageDisplay: React.FC = () => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()
  const [tokenUsage, setTokenUsage] = useState<TokenUsageData | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchTokenUsage = async () => {
      if (!account.user) {
        setLoading(false)
        return
      }

      try {
        const response = await axios.get('/api/v1/users/token-usage', {
          headers: {
            Authorization: `Bearer ${account.user.token}`,
          },
        })
        setTokenUsage(response.data)
      } catch (error) {
        console.error('Failed to fetch token usage:', error)
        // If the endpoint doesn't exist or fails, just don't show the component
        setTokenUsage({ quotas_enabled: false, monthly_usage: 0, monthly_limit: 0, is_pro_tier: false, usage_percentage: 0 })
      } finally {
        setLoading(false)
      }
    }

    fetchTokenUsage()
    // Refresh every 30 seconds
    const interval = setInterval(fetchTokenUsage, 30000)
    return () => clearInterval(interval)
  }, [account.user])

  const handleUpgrade = () => {
    // Navigate to the account page for billing/upgrade
    router.navigate('account')
  }

  if (loading || !tokenUsage || !tokenUsage.quotas_enabled) {
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
  const shouldShowUpgrade = !tokenUsage.is_pro_tier && tokenUsage.usage_percentage >= 50 // Show upgrade button when 50%+ used

  return (
    <Box
      sx={{
        px: 2,
        py: 1.5,
        mx: 1,
        mb: 1,
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
          title={`You've used ${formatNumber(tokenUsage.monthly_usage)} out of ${formatNumber(tokenUsage.monthly_limit)} tokens this month`}
          placement="top"
        >
          <InfoIcon sx={{ fontSize: 14, ml: 0.5, color: lightTheme.textColorFaded }} />
        </Tooltip>
      </Box>
      
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 0.5 }}>
        <Typography variant="body2" sx={{ color: lightTheme.textColor, fontSize: '0.75rem' }}>
          {formatNumber(tokenUsage.monthly_usage)} / {formatNumber(tokenUsage.monthly_limit)}
        </Typography>
        <Typography variant="body2" sx={{ color: lightTheme.textColorFaded, fontSize: '0.75rem' }}>
          {tokenUsage.usage_percentage.toFixed(1)}%
        </Typography>
      </Box>
      
      <LinearProgress
        variant="determinate"
        value={Math.min(tokenUsage.usage_percentage, 100)}
        color={getProgressColor(tokenUsage.usage_percentage)}
        sx={{
          height: 6,
          borderRadius: 3,
          backgroundColor: 'rgba(255, 255, 255, 0.1)',
          '& .MuiLinearProgress-bar': {
            borderRadius: 3,
          },
        }}
      />
      
      {tokenUsage.usage_percentage >= 90 && (
        <Typography 
          variant="caption" 
          sx={{ 
            color: tokenUsage.usage_percentage >= 100 ? 'error.main' : 'warning.main',
            fontSize: '0.7rem',
            mt: 0.5,
            display: 'block'
          }}
        >
          {tokenUsage.usage_percentage >= 100 
            ? 'Limit reached! Upgrade to continue.' 
            : 'Approaching limit. Consider upgrading.'
          }
        </Typography>
      )}

      {shouldShowUpgrade && (
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
          {tokenUsage.usage_percentage >= 90 
            ? 'Upgrade Now' 
            : `Upgrade to Pro (${formatNumber(2500000)} tokens/month)`
          }
        </Button>
      )}
    </Box>
  )
}

export default TokenUsageDisplay 