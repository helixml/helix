import { FC } from 'react'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { PanelLeft } from 'lucide-react'

import { TOOLBAR_HEIGHT } from '../../config'
import useLightTheme from '../../hooks/useLightTheme'

const HELIX_WORDMARK_FONT_FAMILY = '"Sora Variable", "Sora", sans-serif'

const ChatSidebarBrandHeader: FC<{ onCollapse: () => void }> = ({ onCollapse }) => {
  const lightTheme = useLightTheme()

  return (
    <Box
      data-chat-sidebar-brand-header
      sx={{
        position: 'relative',
        isolation: 'isolate',
        height: TOOLBAR_HEIGHT,
        minHeight: TOOLBAR_HEIGHT,
        px: 1.5,
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        overflow: 'hidden',
        borderBottom: lightTheme.isLight
          ? '1px solid rgba(14, 116, 144, 0.18)'
          : '1px solid rgba(103, 232, 249, 0.14)',
        backgroundColor: lightTheme.isLight ? '#f7fbff' : '#070711',
        backgroundImage: lightTheme.isLight
          ? [
              'radial-gradient(110% 140% at 8% 0%, rgba(34, 211, 238, 0.20) 0%, transparent 48%)',
              'radial-gradient(95% 150% at 92% -15%, rgba(217, 70, 239, 0.15) 0%, transparent 52%)',
              'linear-gradient(110deg, rgba(255,255,255,0.96) 0%, rgba(243,248,255,0.96) 55%, rgba(252,244,255,0.96) 100%)',
            ].join(', ')
          : [
              'radial-gradient(100% 180% at 8% -25%, rgba(0, 213, 255, 0.32) 0%, transparent 52%)',
              'radial-gradient(90% 190% at 90% -35%, rgba(239, 46, 198, 0.30) 0%, transparent 56%)',
              'linear-gradient(112deg, #080916 0%, #111027 52%, #100817 100%)',
            ].join(', '),
        '&::before': {
          content: '""',
          position: 'absolute',
          inset: 0,
          zIndex: -1,
          opacity: lightTheme.isLight ? 0.22 : 0.34,
          backgroundImage: [
            'radial-gradient(circle at 8px 9px, currentColor 0 0.65px, transparent 0.9px)',
            'radial-gradient(circle at 19px 15px, currentColor 0 0.45px, transparent 0.75px)',
          ].join(', '),
          backgroundSize: '31px 27px, 47px 39px',
          color: lightTheme.isLight ? '#0e7490' : '#dbeafe',
          maskImage: 'linear-gradient(90deg, black 0%, transparent 74%)',
          WebkitMaskImage: 'linear-gradient(90deg, black 0%, transparent 74%)',
        },
        '&::after': {
          content: '""',
          position: 'absolute',
          left: 12,
          right: 12,
          bottom: 0,
          height: '1px',
          background: lightTheme.isLight
            ? 'linear-gradient(90deg, transparent, rgba(14,116,144,0.40), rgba(192,38,211,0.24), transparent)'
            : 'linear-gradient(90deg, transparent, rgba(34,211,238,0.46), rgba(232,121,249,0.34), transparent)',
        },
      }}
    >
      <Box
        aria-hidden="true"
        sx={{
          width: 28,
          height: 28,
          flexShrink: 0,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          border: lightTheme.isLight
            ? '1px solid rgba(14,116,144,0.16)'
            : '1px solid rgba(255,255,255,0.12)',
          borderRadius: '9px',
          backgroundColor: lightTheme.isLight
            ? 'rgba(255,255,255,0.64)'
            : 'rgba(255,255,255,0.055)',
          boxShadow: lightTheme.isLight
            ? '0 3px 12px rgba(14,116,144,0.10)'
            : 'inset 0 1px rgba(255,255,255,0.08), 0 4px 16px rgba(0,0,0,0.28)',
        }}
      >
        <Box
          component="img"
          src="/img/logo.png"
          alt=""
          sx={{ width: 22, height: 18, objectFit: 'contain' }}
        />
      </Box>

      <Typography
        component="span"
        sx={{
          flex: 1,
          minWidth: 0,
          fontFamily: HELIX_WORDMARK_FONT_FAMILY,
          fontSize: '18px',
          fontWeight: 680,
          lineHeight: 1,
          letterSpacing: '-0.055em',
          background: lightTheme.isLight
            ? 'linear-gradient(105deg, #111827 10%, #0e7490 62%, #86198f 100%)'
            : 'linear-gradient(105deg, #ffffff 12%, #a5f3fc 62%, #f0abfc 100%)',
          backgroundClip: 'text',
          WebkitBackgroundClip: 'text',
          color: 'transparent',
          WebkitTextFillColor: 'transparent',
          textShadow: lightTheme.isLight ? 'none' : '0 0 18px rgba(103,232,249,0.12)',
        }}
      >
        helix
      </Typography>

      <Tooltip title="Collapse chat panel">
        <IconButton
          onClick={onCollapse}
          aria-label="Collapse chat panel"
          sx={{
            width: 30,
            height: 30,
            flexShrink: 0,
            color: lightTheme.isLight ? 'rgba(15,23,42,0.64)' : 'rgba(255,255,255,0.70)',
            border: '1px solid transparent',
            '&:hover': {
              color: lightTheme.isLight ? '#0f172a' : '#ffffff',
              borderColor: lightTheme.isLight
                ? 'rgba(14,116,144,0.14)'
                : 'rgba(255,255,255,0.10)',
              backgroundColor: lightTheme.isLight
                ? 'rgba(255,255,255,0.58)'
                : 'rgba(255,255,255,0.07)',
            },
          }}
        >
          <PanelLeft size={18} strokeWidth={1.7} />
        </IconButton>
      </Tooltip>
    </Box>
  )
}

export default ChatSidebarBrandHeader
