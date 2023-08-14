import React, { useMemo } from 'react'
import { createTheme, ThemeProvider, Theme } from '@mui/material/styles'
import AllContextProvider from './contexts/all'
import Layout from './pages/Layout'
import useThemeConfig from './hooks/useThemeConfig'

export default function App() {
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
    <AllContextProvider>
      <ThemeProvider theme={theme}>
        <Layout />
      </ThemeProvider>
    </AllContextProvider>
  )
}
