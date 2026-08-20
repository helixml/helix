import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import { SxProps, Theme } from '@mui/material/styles'

import useLightTheme from '../../hooks/useLightTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

interface SettingsPanelProps {
  children: ReactNode
  sx?: SxProps<Theme>
}

/**
 * A card in the account settings pages.
 *
 * Each of these used to be a `Grid container spacing={2}` that carried the
 * panel background itself. MUI implements spacing as a negative margin on the
 * container, so the background painted 16px outside the box it was supposed to
 * fill — on a phone the panels bled off the left edge while leaving a gap on
 * the right. The background belongs to a plain Box; any grid goes inside it.
 */
const SettingsPanel: FC<SettingsPanelProps> = ({ children, sx }) => {
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()
  const panelBg = lightTheme.isLight ? lightTheme.panelColor : themeConfig.darkPanel

  return (
    <Box
      sx={{
        backgroundColor: panelBg,
        p: { xs: 2, sm: 2 },
        borderRadius: 2,
        mb: 2,
        ...sx,
      }}
    >
      {children}
    </Box>
  )
}

export default SettingsPanel
