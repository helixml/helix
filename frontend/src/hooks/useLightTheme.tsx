import { useTheme, SxProps } from '@mui/material/styles'
import useThemeConfig from './useThemeConfig'

const useLightTheme = () => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const isLight = theme.palette.mode === 'light'
  const border = isLight ? themeConfig.lightBorder : themeConfig.darkBorder
  const backgroundColor = isLight ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor
  const icon = isLight ? themeConfig.lightIcon : themeConfig.darkIcon
  const textColor = isLight ? themeConfig.lightText : themeConfig.neutral300
  const scrollbar: SxProps = {
    '&::-webkit-scrollbar': {
      width: '4px',
      borderRadius: '8px',
      my: 2,
    },
    '&::-webkit-scrollbar-track': {
      background: isLight ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
    },
    '&::-webkit-scrollbar-thumb': {
      background: isLight ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
      borderRadius: '8px',
    },
    '&::-webkit-scrollbar-thumb:hover': {
      background: isLight ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
    },
  }
  return {
    isLight,
    isDark: !isLight,
    border,
    backgroundColor,
    icon,
    textColor,
    scrollbar,
  }
}

export default useLightTheme