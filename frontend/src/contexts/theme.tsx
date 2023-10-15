import React, { FC, useMemo } from 'react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'

export const ThemeProviderWrapper: FC = ({ children }) => {
  const themeConfig = useThemeConfig()
  const theme = useMemo(() => {
    return createTheme({
      palette: {
        primary: {
          main: themeConfig.primary,
        },
        secondary: {
          main: themeConfig.secondary,
        }
      } 
    })
  }, [
    themeConfig,
  ])

  return (
    <ThemeProvider theme={ theme }>
      { children }
    </ThemeProvider>
  )
}