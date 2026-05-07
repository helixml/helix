import React, { useEffect, useMemo, useState, ReactNode } from 'react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'
import useApi from '../hooks/useApi'
import { PaletteMode } from '@mui/material'

const THEME_MODE_KEY = 'themeMode'

function getInitialMode(): PaletteMode {
  const stored = localStorage.getItem(THEME_MODE_KEY)
  if (stored === 'light' || stored === 'dark') return stored
  if (window.matchMedia('(prefers-color-scheme: light)').matches) return 'light'
  return 'dark'
}

export const ThemeContext = React.createContext({
  mode: 'dark' as PaletteMode,
  toggleMode: () => {},
})

export const ThemeProviderWrapper = ({ children }: { children: ReactNode }) => {
  const themeConfig = useThemeConfig()
  const api = useApi()
  const [mode, setMode] = useState<PaletteMode>(getInitialMode)

  // Live OS preference sync: follow the OS only while the user has not explicitly
  // toggled (no entry in localStorage). Once they toggle, their explicit choice wins.
  // We also push the change to the API so the user's spec-task GNOME desktops and
  // Zed editors flip with them — otherwise the OS-driven flip would only update
  // the browser app, not the inner desktop.
  useEffect(() => {
    const mql = window.matchMedia('(prefers-color-scheme: light)')
    const handler = (e: MediaQueryListEvent) => {
      if (localStorage.getItem(THEME_MODE_KEY)) return
      const next: PaletteMode = e.matches ? 'light' : 'dark'
      setMode(next)
      api.getApiClient().v1UsersMeColorSchemeUpdate({ color_scheme: next })
        .catch(() => { /* non-fatal: anonymous users / transient errors */ })
    }
    mql.addEventListener('change', handler)
    return () => mql.removeEventListener('change', handler)
  }, [api])

  const isLight = mode === 'light'

  const theme = useMemo(() => {
    const bg = isLight ? themeConfig.lightBackgroundColor : themeConfig.darkBackgroundColor
    const scrollbarTrack = isLight ? themeConfig.lightScrollbar : themeConfig.darkScrollbar
    const scrollbarThumb = isLight ? themeConfig.lightScrollbarThumb : themeConfig.darkScrollbarThumb
    const scrollbarHover = isLight ? themeConfig.lightScrollbarHover : themeConfig.darkScrollbarHover

    const scrollbarStyles = {
      '&::-webkit-scrollbar': {
        width: '4px',
        borderRadius: '8px',
      },
      '&::-webkit-scrollbar-track': {
        background: scrollbarTrack,
      },
      '&::-webkit-scrollbar-thumb': {
        background: scrollbarThumb,
        borderRadius: '8px',
      },
      '&::-webkit-scrollbar-thumb:hover': {
        background: scrollbarHover,
      },
    }

    const menuSurfaceBg = isLight ? 'rgba(255, 255, 255, 0.97)' : 'rgba(26, 26, 26, 0.97)'
    const menuBorder = isLight ? '1px solid rgba(0,0,0,0.10)' : '1px solid rgba(255,255,255,0.10)'
    const menuTextColor = isLight ? '#333' : 'white'
    const menuHoverBg = isLight ? 'rgba(0,180,220,0.10)' : 'rgba(0,229,255,0.13)'
    const menuSelectedBg = isLight ? 'rgba(0,180,220,0.15)' : 'rgba(0,229,255,0.18)'
    const menuDividerColor = isLight ? 'rgba(0,0,0,0.10)' : 'rgba(255,255,255,0.10)'
    const menuShadow = isLight ? '0 8px 32px rgba(0,0,0,0.12)' : '0 8px 32px rgba(0,0,0,0.32)'

    const dialogBg = isLight ? '#ffffff' : '#181A20'
    const dialogColor = isLight ? '#333' : '#F1F1F1'
    const dialogShadow = isLight ? '0 8px 32px rgba(0, 0, 0, 0.15)' : '0 8px 32px rgba(0, 0, 0, 0.5)'

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
          default: bg,
        },
      },
      typography: {
        fontFamily: "IBM Plex Sans, Helvetica, Arial, sans-serif",
        fontSize: 14,
        // Light mode is often viewed in sunlight — bump weights for readability.
        ...(isLight && {
          fontWeightLight: 400,
          fontWeightRegular: 500,
          fontWeightMedium: 600,
          fontWeightBold: 700,
          body1: { fontWeight: 500 },
          body2: { fontWeight: 500 },
          subtitle1: { fontWeight: 600 },
          subtitle2: { fontWeight: 600 },
          button: { fontWeight: 600 },
        }),
      },
      components: {
        MuiCssBaseline: {
          styleOverrides: {
            body: {
              backgroundColor: bg,
              ...scrollbarStyles,
            },
            '*': scrollbarStyles,
          },
        },
        MuiMenu: {
          styleOverrides: {
            root: {
              zIndex: 100003,
              '& .MuiMenu-list': {
                padding: 0,
                backgroundColor: menuSurfaceBg,
                backdropFilter: 'blur(10px)',
                minWidth: '160px',
                borderRadius: '10px',
                border: menuBorder,
                boxShadow: menuShadow,
              },
              '& .MuiMenuItem-root': {
                color: menuTextColor,
                fontSize: '0.92rem',
                fontWeight: 500,
                padding: '8px 16px',
                minHeight: '32px',
                borderRadius: '6px',
                transition: 'background 0.15s',
                '&:hover': {
                  backgroundColor: menuHoverBg,
                },
                '&.Mui-selected': {
                  backgroundColor: menuSelectedBg,
                },
              },
              '& .MuiDivider-root': {
                borderColor: menuDividerColor,
                margin: '4px 0',
              },
            },
          },
        },
        MuiPaper: {
          styleOverrides: {
            root: {
              '&.MuiMenu-paper, &.MuiPopover-paper': {
                backgroundColor: menuSurfaceBg,
                backdropFilter: 'blur(10px)',
                borderRadius: '10px',
                boxShadow: menuShadow,
              },
            },
          },
        },
        MuiDialog: {
          defaultProps: {
            disableEnforceFocus: true,
          },
          styleOverrides: {
            paper: {
              background: dialogBg,
              color: dialogColor,
              borderRadius: 16,
              boxShadow: dialogShadow,
              transition: 'all 0.2s ease-in-out',
            },
            root: {
              zIndex: 100002, // Above floating windows (z-index 9999); tooltips (100004) render above
              transition: 'all 0.2s ease-in-out',
            },
          },
        },
        // Tooltips must sit above dialogs (100002), popovers and select menus (100003)
        // so they remain visible when triggered from elements inside a modal.
        MuiTooltip: {
          defaultProps: {
            slotProps: {
              popper: {
                sx: {
                  zIndex: 100004,
                },
              },
            },
          },
        },
        MuiPopover: {
          styleOverrides: {
            root: {
              zIndex: 100003,
            },
          },
        },
        MuiSelect: {
          defaultProps: {
            MenuProps: {
              sx: {
                zIndex: 100003,
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
    themeConfig, mode, isLight
  ])

  const toggleMode = () => {
    setMode((prevMode) => {
      const next = prevMode === 'dark' ? 'light' : 'dark'
      localStorage.setItem(THEME_MODE_KEY, next)
      // Fire-and-forget: persist to the user's account so any spec-task
      // sessions they own can mirror the theme into GNOME and Zed within
      // ~100ms via the settings-sync-daemon's WS subscription.
      api.getApiClient().v1UsersMeColorSchemeUpdate({ color_scheme: next })
        .catch(() => { /* non-fatal: anonymous users or transient errors */ })
      return next
    })
  }

  return (
    <ThemeProvider theme={ theme }>
      <ThemeContext.Provider value={{ mode, toggleMode }}>
        { children }
      </ThemeContext.Provider>
    </ThemeProvider>
  )
}
