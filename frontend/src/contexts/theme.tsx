import React, { FC, useMemo, useState } from 'react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'
import { PaletteMode } from '@mui/material'

export const ThemeContext = React.createContext({
  mode: 'dark',
  toggleMode: () => {},
})

export const ThemeProviderWrapper: FC = ({ children }) => {
  const themeConfig = useThemeConfig()
  const [mode, setMode] = useState<PaletteMode>('dark')
  const theme = useMemo(() => {
    return createTheme({
      palette: {
        primary: {
          main: themeConfig.primary,
        },
        secondary: {
          main: themeConfig.secondary,
        },
        mode: mode,
      },
      typography: {
        fontFamily: "Assistant, Helvetica, Arial, sans-serif",
        fontSize: 14,
      }
    })
  }, [
    themeConfig, mode
  ])
  
  const toggleMode = () => {
    setMode((prevMode: any) => prevMode === 'dark' ? 'light' : 'dark')
  }

  return (
    <ThemeProvider theme={ theme }>
      <ThemeContext.Provider value={{ mode, toggleMode }}>
        { children }
      </ThemeContext.Provider>
    </ThemeProvider>
  )
}