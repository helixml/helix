import React, { useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import Typography from '@mui/material/Typography'
import Slide from '@mui/material/Slide'
import { X, Share, PlusSquare, Download, MoreVertical } from 'lucide-react'

interface BeforeInstallPromptEvent extends Event {
  prompt: () => Promise<void>
  userChoice: Promise<{ outcome: 'accepted' | 'dismissed' }>
}

const STORAGE_KEY = 'helix-pwa-prompt-dismissed'
const DISMISS_DURATION_MS = 7 * 24 * 60 * 60 * 1000 // 7 days

const InstallPWA: React.FC = () => {
  const [deferredPrompt, setDeferredPrompt] = useState<BeforeInstallPromptEvent | null>(null)
  const [showPrompt, setShowPrompt] = useState(false)
  const [isIOS, setIsIOS] = useState(false)
  const [isAndroid, setIsAndroid] = useState(false)
  const [isStandalone, setIsStandalone] = useState(false)

  useEffect(() => {
    // Check if running in standalone mode (already installed)
    const standalone = window.matchMedia('(display-mode: standalone)').matches
      || (window.navigator as Navigator & { standalone?: boolean }).standalone === true
    setIsStandalone(standalone)

    if (standalone) return

    // Detect platform
    const userAgent = navigator.userAgent.toLowerCase()
    const ios = /iphone|ipad|ipod/.test(userAgent) && !(window as Window & { MSStream?: unknown }).MSStream
    const android = /android/.test(userAgent)

    setIsIOS(ios)
    setIsAndroid(android)

    // Check if we should show the prompt (not dismissed recently)
    const dismissedAt = localStorage.getItem(STORAGE_KEY)
    if (dismissedAt) {
      const dismissedTime = parseInt(dismissedAt, 10)
      if (Date.now() - dismissedTime < DISMISS_DURATION_MS) {
        return // Don't show if dismissed within the last 7 days
      }
    }

    // Only show on mobile devices
    if (!ios && !android) return

    // For iOS, show the prompt after a delay
    if (ios) {
      const timer = setTimeout(() => setShowPrompt(true), 3000)
      return () => clearTimeout(timer)
    }

    // For Android, listen for the beforeinstallprompt event
    const handleBeforeInstallPrompt = (e: Event) => {
      e.preventDefault()
      setDeferredPrompt(e as BeforeInstallPromptEvent)
      setTimeout(() => setShowPrompt(true), 2000)
    }

    window.addEventListener('beforeinstallprompt', handleBeforeInstallPrompt)
    return () => window.removeEventListener('beforeinstallprompt', handleBeforeInstallPrompt)
  }, [])

  const handleInstall = async () => {
    if (!deferredPrompt) return

    deferredPrompt.prompt()
    const { outcome } = await deferredPrompt.userChoice

    if (outcome === 'accepted') {
      setShowPrompt(false)
    }
    setDeferredPrompt(null)
  }

  const handleDismiss = () => {
    setShowPrompt(false)
    localStorage.setItem(STORAGE_KEY, Date.now().toString())
  }

  // Don't render if already in standalone mode or not on mobile
  if (isStandalone || (!isIOS && !isAndroid && !deferredPrompt)) {
    return null
  }

  return (
    <Slide direction="up" in={showPrompt} mountOnEnter unmountOnExit>
      <Box
        sx={{
          position: 'fixed',
          bottom: 0,
          left: 0,
          right: 0,
          zIndex: 9999,
          p: 2,
          pb: 'calc(env(safe-area-inset-bottom) + 16px)',
          background: 'linear-gradient(135deg, #1a1a2e 0%, #0d0d0d 100%)',
          borderTop: '1px solid rgba(74, 222, 128, 0.3)',
          boxShadow: '0 -4px 20px rgba(0, 0, 0, 0.5)',
        }}
      >
        <IconButton
          onClick={handleDismiss}
          size="small"
          sx={{
            position: 'absolute',
            top: 8,
            right: 8,
            color: 'grey.500',
          }}
        >
          <X size={20} />
        </IconButton>

        <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, pr: 4 }}>
          <Box
            component="img"
            src="/img/logo.png"
            alt="Helix"
            sx={{
              width: 48,
              height: 48,
              borderRadius: 2,
            }}
          />
          <Box sx={{ flex: 1 }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 600, color: 'white' }}>
              Add Helix to Home Screen
            </Typography>
            {isIOS ? (
              <Typography variant="body2" sx={{ color: 'grey.400', display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 0.5 }}>
                Tap <Share size={14} style={{ margin: '0 2px' }} /> then "Add to Home Screen" <PlusSquare size={14} style={{ margin: '0 2px' }} />
              </Typography>
            ) : (
              <Typography variant="body2" sx={{ color: 'grey.400' }}>
                Install for quick access and a better experience
              </Typography>
            )}
          </Box>
        </Box>

        {isAndroid && deferredPrompt && (
          <Button
            variant="contained"
            fullWidth
            onClick={handleInstall}
            startIcon={<Download size={18} />}
            sx={{
              mt: 2,
              py: 1.5,
              background: 'linear-gradient(90deg, #4ade80 0%, #22c55e 100%)',
              color: '#0d0d0d',
              fontWeight: 600,
              '&:hover': {
                background: 'linear-gradient(90deg, #22c55e 0%, #16a34a 100%)',
              },
            }}
          >
            Install App
          </Button>
        )}

        {isIOS && (
          <Box
            sx={{
              mt: 2,
              p: 1.5,
              borderRadius: 1,
              backgroundColor: 'rgba(255,255,255,0.05)',
              border: '1px solid rgba(255,255,255,0.1)',
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5 }}>
              <Box sx={{
                width: 24,
                height: 24,
                borderRadius: '50%',
                backgroundColor: 'rgba(74, 222, 128, 0.2)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}>
                <Typography sx={{ fontSize: 12, color: '#4ade80' }}>1</Typography>
              </Box>
              <Typography variant="body2" sx={{ color: 'grey.300' }}>
                Tap the <Share size={14} style={{ verticalAlign: 'middle', margin: '0 4px' }} /> Share button below
              </Typography>
            </Box>
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <Box sx={{
                width: 24,
                height: 24,
                borderRadius: '50%',
                backgroundColor: 'rgba(74, 222, 128, 0.2)',
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'center',
              }}>
                <Typography sx={{ fontSize: 12, color: '#4ade80' }}>2</Typography>
              </Box>
              <Typography variant="body2" sx={{ color: 'grey.300' }}>
                Scroll and tap "Add to Home Screen" <PlusSquare size={14} style={{ verticalAlign: 'middle', margin: '0 4px' }} />
              </Typography>
            </Box>
          </Box>
        )}
      </Box>
    </Slide>
  )
}

export default InstallPWA
