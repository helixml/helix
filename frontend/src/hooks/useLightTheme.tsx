import { useTheme, SxProps } from '@mui/material/styles'
import useThemeConfig from './useThemeConfig'

const useLightTheme = () => {
  const theme = useTheme()
  const themeConfig = useThemeConfig()
  const isLight = theme.palette.mode === 'light'
  const border = isLight ? themeConfig.lightBorder : themeConfig.darkBorder
  const backgroundColor = isLight ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor
  const icon = isLight ? themeConfig.lightIcon : themeConfig.darkIcon
  const iconHover = isLight ? themeConfig.lightIconHover : themeConfig.darkIconHover
  const highlightColor = isLight ? themeConfig.lightHighlight : themeConfig.darkHighlight
  const textColor = isLight ? themeConfig.lightText : themeConfig.darkText
  const textColorFaded = isLight ? themeConfig.lightTextFaded : themeConfig.darkTextFaded
  const panelColor = isLight ? themeConfig.lightPanel : themeConfig.darkPanel
  const scrollbar: SxProps = {
    '&::-webkit-scrollbar': {
      width: '4px',
      borderRadius: '8px',
      my: 2,
    },
    '&::-webkit-scrollbar-track': {
      background: isLight ? themeConfig.lightScrollbar : themeConfig.darkScrollbar,
    },
    '&::-webkit-scrollbar-thumb': {
      background: isLight ? themeConfig.lightScrollbarThumb : themeConfig.darkScrollbarThumb,
      borderRadius: '8px',
    },
    '&::-webkit-scrollbar-thumb:hover': {
      background: isLight ? themeConfig.lightScrollbarHover : themeConfig.darkScrollbarHover,
    },
  }
  return {
    isLight,
    isDark: !isLight,
    border,
    backgroundColor,
    icon,
    iconHover,
    highlightColor,
    textColor,
    textColorFaded,
    panelColor,
    scrollbar,
  }
}

export default useLightTheme
