import React from 'react'
import Box from '@mui/material/Box'
import useLightTheme from '../../hooks/useLightTheme'
import { TOOLBAR_HEIGHT } from '../../config'
import TokenExpiryCounter from '../auth/TokenExpiryCounter'
import { LIGHT_SIDEBAR_COLORS } from '../../styles/themeTokens'

const SidebarContextHeader: React.FC = () => {
  const lightTheme = useLightTheme()

  return (
    <Box
      sx={{
        width: '100%',
        height: TOOLBAR_HEIGHT,
        flex: `0 0 ${TOOLBAR_HEIGHT}px`,
        px: 1,
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'flex-end',
        backgroundColor: lightTheme.isLight ? LIGHT_SIDEBAR_COLORS.background : lightTheme.backgroundColor,
        borderBottom: lightTheme.isLight ? `1px solid ${LIGHT_SIDEBAR_COLORS.border}` : lightTheme.border,
      }}
    >
      <TokenExpiryCounter />
    </Box>
  )
}

export default SidebarContextHeader
