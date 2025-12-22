import React, { useMemo, useState, ReactNode } from 'react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'
import { PaletteMode } from '@mui/material'

export const ThemeContext = React.createContext({
  mode: 'dark',
  toggleMode: () => {},
})

export const ThemeProviderWrapper = ({ children }: { children: ReactNode }) => {
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
        background: {
          default: '#23272f',
        },
      },
      typography: {
        fontFamily: "IBM Plex Sans, Helvetica, Arial, sans-serif",
        fontSize: 14,
      },
      components: {
        // Global scrollbar styles
        MuiCssBaseline: {
          styleOverrides: {
            body: {
              '&::-webkit-scrollbar': {
                width: '4px',
                borderRadius: '8px',
              },
              '&::-webkit-scrollbar-track': {
                background: mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
              },
              '&::-webkit-scrollbar-thumb': {
                background: mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
                borderRadius: '8px',
              },
              '&::-webkit-scrollbar-thumb:hover': {
                background: mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
              },
            },
            '*': {
              '&::-webkit-scrollbar': {
                width: '4px',
                borderRadius: '8px',
              },
              '&::-webkit-scrollbar-track': {
                background: mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbar,
              },
              '&::-webkit-scrollbar-thumb': {
                background: mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarThumb,
                borderRadius: '8px',
              },
              '&::-webkit-scrollbar-thumb:hover': {
                background: mode === 'light' ? themeConfig.lightBackgroundColor : themeConfig.darkScrollbarHover,
              },
            },
          },
        },
        // Adding dark style to the menus + high z-index for floating windows
        MuiMenu: {
          styleOverrides: {
            root: {
              zIndex: 100001, // Above floating windows (z-index 9999)
              '& .MuiMenu-list': {
                padding: 0,
                backgroundColor: 'rgba(26, 26, 26, 0.97)',
                backdropFilter: 'blur(10px)',
                minWidth: '160px',
                borderRadius: '10px',
                border: '1px solid rgba(255,255,255,0.10)',
                boxShadow: '0 8px 32px rgba(0,0,0,0.32)',
              },
              '& .MuiMenuItem-root': {
                color: 'white',
                fontSize: '0.92rem',
                fontWeight: 500,
                padding: '8px 16px',
                minHeight: '32px',
                borderRadius: '6px',
                transition: 'background 0.15s',
                '&:hover': {
                  backgroundColor: 'rgba(0,229,255,0.13)',
                },
                '&.Mui-selected': {
                  backgroundColor: 'rgba(0,229,255,0.18)',
                },
              },
              '& .MuiDivider-root': {
                borderColor: 'rgba(255,255,255,0.10)',
                margin: '4px 0',
              },
            },
          },
        },
        MuiPaper: {
          styleOverrides: {
            root: {
              '&.MuiMenu-paper, &.MuiPopover-paper': {
                backgroundColor: 'rgba(26, 26, 26, 0.97)',
                backdropFilter: 'blur(10px)',
                borderRadius: '10px',
                boxShadow: '0 8px 32px rgba(0,0,0,0.32)',
              },
            },
          },
        },
        MuiDialog: {
          styleOverrides: {
            paper: {
              background: '#181A20',
              color: '#F1F1F1',
              borderRadius: 16,
              boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
              transition: 'all 0.2s ease-in-out',
            },
            root: {
              zIndex: 100002, // Above floating windows (z-index 9999) and tooltips (100001)
              transition: 'all 0.2s ease-in-out',
            },
          },
        },
        // Ensure tooltips appear above floating windows (z-index 9999) and modals
        MuiTooltip: {
          defaultProps: {
            // Higher z-index ensures tooltips appear above floating windows
            slotProps: {
              popper: {
                sx: {
                  zIndex: 100001,
                },
              },
            },
          },
        },
        // Ensure popovers (including Select dropdowns) appear above floating windows
        MuiPopover: {
          styleOverrides: {
            root: {
              zIndex: 100001,
            },
          },
        },
        // Ensure Select menus appear above floating windows
        MuiSelect: {
          defaultProps: {
            MenuProps: {
              sx: {
                zIndex: 100001,
              },
            },
          },
        },
      },
      chartGradientStart: themeConfig.chartGradientStart,
      chartGradientEnd: themeConfig.chartGradientEnd,
      chartGradientStartOpacity: themeConfig.chartGradientStartOpacity,
      chartGradientEndOpacity: themeConfig.chartGradientEndOpacity,
      chartHighlightGradientStart: themeConfig.chartHighlightGradientStart,
      chartHighlightGradientEnd: themeConfig.chartHighlightGradientEnd,
      chartHighlightGradientStartOpacity: themeConfig.chartHighlightGradientStartOpacity,
      chartHighlightGradientEndOpacity: themeConfig.chartHighlightGradientEndOpacity,
      chartActionGradientStart: themeConfig.chartActionGradientStart,
      chartActionGradientEnd: themeConfig.chartActionGradientEnd,
      chartActionGradientStartOpacity: themeConfig.chartActionGradientStartOpacity,
      chartActionGradientEndOpacity: themeConfig.chartActionGradientEndOpacity,
      chartErrorGradientStart: themeConfig.chartErrorGradientStart,
      chartErrorGradientEnd: themeConfig.chartErrorGradientEnd,
      chartErrorGradientStartOpacity: themeConfig.chartErrorGradientStartOpacity,
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