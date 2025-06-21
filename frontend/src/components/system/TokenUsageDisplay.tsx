import React, { useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import LinearProgress from '@mui/material/LinearProgress'
import Tooltip from '@mui/material/Tooltip'
import Button from '@mui/material/Button'
import InfoIcon from '@mui/icons-material/Info'
import UpgradeIcon from '@mui/icons-material/Upgrade'
import TrendingUpIcon from '@mui/icons-material/TrendingUp'
import axios from 'axios'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import { keyframes } from '@mui/material/styles'

interface TokenUsageData {
  quotas_enabled: boolean
  monthly_usage: number
  monthly_limit: number
  is_pro_tier: boolean
  usage_percentage: number
}

// Animation keyframes for modern effects
const shimmer = keyframes`
  0% {
    background-position: -200% center;
  }
  100% {
    background-position: 200% center;
  }
`

const pulse = keyframes`
  0% {
    transform: scale(1);
    box-shadow: 0 0 20px rgba(0, 229, 255, 0.3);
  }
  50% {
    transform: scale(1.02);
    box-shadow: 0 0 30px rgba(0, 229, 255, 0.5);
  }
  100% {
    transform: scale(1);
    box-shadow: 0 0 20px rgba(0, 229, 255, 0.3);
  }
`

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
    if (percentage >= 90) return '#FF4444'
    if (percentage >= 75) return '#FF8A00'
    return '#00E5FF'
  }

  const getGradientByUsage = (percentage: number) => {
    if (percentage >= 90) {
      return 'linear-gradient(135deg, rgba(255, 68, 68, 0.15) 0%, rgba(255, 20, 147, 0.15) 100%)'
    }
    if (percentage >= 75) {
      return 'linear-gradient(135deg, rgba(255, 138, 0, 0.15) 0%, rgba(255, 215, 0, 0.15) 100%)'
    }
    return 'linear-gradient(135deg, rgba(0, 229, 255, 0.15) 0%, rgba(147, 51, 234, 0.15) 100%)'
  }

  const tierName = tokenUsage.is_pro_tier ? 'Pro' : 'Free'
  const shouldShowUpgrade = !tokenUsage.is_pro_tier && tokenUsage.usage_percentage >= 50 // Show upgrade button when 50%+ used

  return (
    <Box
      sx={{
        position: 'relative',
        mx: 2,
        my: 2,
        p: 3,
        borderRadius: '20px',
        background: getGradientByUsage(tokenUsage.usage_percentage),
        backdropFilter: 'blur(20px)',
        border: '1px solid rgba(255, 255, 255, 0.1)',
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.3)',
        transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
        cursor: 'pointer',
        overflow: 'hidden',
        '&:hover': {
          transform: 'translateY(-2px)',
          boxShadow: '0 12px 40px rgba(0, 0, 0, 0.4)',
          background: getGradientByUsage(tokenUsage.usage_percentage).replace('0.15', '0.25'),
        },
        // Glassmorphism overlay
        '&::before': {
          content: '""',
          position: 'absolute',
          top: 0,
          left: 0,
          right: 0,
          bottom: 0,
          background: 'linear-gradient(135deg, rgba(255, 255, 255, 0.1) 0%, rgba(255, 255, 255, 0.05) 100%)',
          borderRadius: '20px',
          zIndex: 0,
        },
      }}
      onClick={() => router.navigate('account')}
    >
      {/* Content Container */}
      <Box sx={{ position: 'relative', zIndex: 1 }}>
        {/* Header with Plan Info */}
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', mb: 2 }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Box
              sx={{
                width: 24,
                height: 24,
                borderRadius: '8px',
                background: tokenUsage.is_pro_tier 
                  ? 'linear-gradient(135deg, #FFD700 0%, #FFA500 100%)'
                  : 'linear-gradient(135deg, #00E5FF 0%, #9333EA 100%)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
                animation: tokenUsage.usage_percentage >= 75 ? `${pulse} 2s ease-in-out infinite` : 'none',
              }}
            >
              <TrendingUpIcon sx={{ fontSize: 14, color: 'white' }} />
            </Box>
            <Typography 
              variant="subtitle2" 
              sx={{ 
                color: 'rgba(255, 255, 255, 0.9)', 
                fontWeight: 700,
                fontSize: '0.875rem',
                letterSpacing: '0.5px'
              }}
            >
              {tierName} Plan
            </Typography>
          </Box>
          
          <Tooltip 
            title={`You've used ${formatNumber(tokenUsage.monthly_usage)} out of ${formatNumber(tokenUsage.monthly_limit)} tokens this month`}
            placement="top"
            arrow
            sx={{
              '& .MuiTooltip-tooltip': {
                backgroundColor: 'rgba(0, 0, 0, 0.9)',
                borderRadius: '12px',
                backdropFilter: 'blur(10px)',
                border: '1px solid rgba(255, 255, 255, 0.1)',
              },
            }}
          >
            <InfoIcon 
              sx={{ 
                fontSize: 18, 
                color: 'rgba(255, 255, 255, 0.7)',
                cursor: 'pointer',
                transition: 'all 0.2s ease',
                '&:hover': {
                  color: 'rgba(255, 255, 255, 1)',
                  transform: 'scale(1.1)',
                }
              }} 
            />
          </Tooltip>
        </Box>

        {/* Monthly Tokens Label */}
        <Typography 
          variant="caption" 
          sx={{ 
            color: 'rgba(255, 255, 255, 0.7)', 
            fontWeight: 500,
            fontSize: '0.75rem',
            textTransform: 'uppercase',
            letterSpacing: '1px',
            display: 'block',
            mb: 1.5,
          }}
        >
          Monthly Tokens
        </Typography>
        
        {/* Usage Stats */}
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline', mb: 2 }}>
          <Typography 
            variant="h6" 
            sx={{ 
              color: 'white',
              fontWeight: 800,
              fontSize: '1.5rem',
              background: 'linear-gradient(135deg, #FFFFFF 0%, rgba(255, 255, 255, 0.8) 100%)',
              backgroundClip: 'text',
              WebkitBackgroundClip: 'text',
              WebkitTextFillColor: 'transparent',
            }}
          >
            {formatNumber(tokenUsage.monthly_usage)} <span style={{ fontSize: '0.8em', fontWeight: 400 }}>/ {formatNumber(tokenUsage.monthly_limit)}</span>
          </Typography>
          <Typography 
            variant="h6" 
            sx={{ 
              color: getProgressColor(tokenUsage.usage_percentage),
              fontWeight: 700,
              fontSize: '1.25rem',
            }}
          >
            {tokenUsage.usage_percentage.toFixed(1)}%
          </Typography>
        </Box>
        
        {/* Modern Progress Bar */}
        <Box sx={{ position: 'relative', mb: 2 }}>
          <Box
            sx={{
              height: 8,
              borderRadius: '12px',
              backgroundColor: 'rgba(255, 255, 255, 0.1)',
              overflow: 'hidden',
              backdropFilter: 'blur(10px)',
            }}
          >
            <Box
              sx={{
                width: `${Math.min(tokenUsage.usage_percentage, 100)}%`,
                height: '100%',
                background: `linear-gradient(90deg, ${getProgressColor(tokenUsage.usage_percentage)} 0%, ${getProgressColor(tokenUsage.usage_percentage)}AA 100%)`,
                borderRadius: '12px',
                position: 'relative',
                transition: 'width 0.8s cubic-bezier(0.4, 0, 0.2, 1)',
                '&::after': {
                  content: '""',
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  right: 0,
                  bottom: 0,
                  background: `linear-gradient(90deg, transparent 0%, ${getProgressColor(tokenUsage.usage_percentage)}44 50%, transparent 100%)`,
                  backgroundSize: '200% 100%',
                  animation: tokenUsage.usage_percentage > 0 ? `${shimmer} 2s linear infinite` : 'none',
                },
              }}
            />
          </Box>
        </Box>
        
        {/* Warning Messages */}
        {tokenUsage.usage_percentage >= 90 && (
          <Typography 
            variant="caption" 
            sx={{ 
              color: tokenUsage.usage_percentage >= 100 ? '#FF4444' : '#FF8A00',
              fontSize: '0.75rem',
              fontWeight: 600,
              display: 'flex',
              alignItems: 'center',
              gap: 0.5,
              p: 1,
              borderRadius: '8px',
              backgroundColor: 'rgba(255, 255, 255, 0.05)',
              border: `1px solid ${tokenUsage.usage_percentage >= 100 ? 'rgba(255, 68, 68, 0.3)' : 'rgba(255, 138, 0, 0.3)'}`,
            }}
          >
            <InfoIcon sx={{ fontSize: 14 }} />
            {tokenUsage.usage_percentage >= 100 
              ? 'Limit reached! Upgrade to continue.' 
              : 'Approaching limit. Consider upgrading.'
            }
          </Typography>
        )}

        {/* Upgrade Button */}
        {shouldShowUpgrade && (
          <Button
            onClick={(e) => {
              e.stopPropagation()
              handleUpgrade()
            }}
            variant="contained"
            startIcon={<UpgradeIcon />}
            sx={{
              mt: 2,
              width: '100%',
              borderRadius: '12px',
              background: 'linear-gradient(135deg, #00E5FF 0%, #9333EA 100%)',
              backgroundSize: '200% 100%',
              color: 'white',
              fontSize: '0.875rem',
              fontWeight: 700,
              textTransform: 'none',
              py: 1.5,
              border: 'none',
              boxShadow: '0 4px 20px rgba(0, 229, 255, 0.3)',
              transition: 'all 0.3s cubic-bezier(0.4, 0, 0.2, 1)',
              '&:hover': {
                backgroundPosition: '100% 0',
                transform: 'translateY(-1px)',
                boxShadow: '0 6px 25px rgba(0, 229, 255, 0.4)',
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
    </Box>
  )
}

export default TokenUsageDisplay 