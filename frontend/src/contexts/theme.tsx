import React, { useEffect, useMemo, useState, ReactNode } from 'react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'
import useApi from '../hooks/useApi'
import { PaletteMode } from '@mui/material'
import { APP_FONT_FAMILY, APP_MONO_FONT_FAMILY, TYPOGRAPHY, typographyCssVariables } from '../styles/typography'

const THEME_MODE_KEY = 'themeMode'

// themePinnedByQuery reports whether the embedder stated the mode explicitly.
function themePinnedByQuery(): boolean {
  try {
    const q = new URLSearchParams(window.location.search).get('theme')
    return q === 'dark' || q === 'light'
  } catch {
    return false
  }
}

function storedThemeMode(): PaletteMode | null {
  try {
    const stored = localStorage.getItem(THEME_MODE_KEY)
    return stored === 'dark' || stored === 'light' ? stored : null
  } catch {
    return null
  }
}

function getInitialMode(): PaletteMode {
  // An explicit ?theme= wins over the browser override and OS preference.
  //
  // This exists for embedding. An iframe inherits the VIEWER's OS setting, not
  // the host page's, so a dark site embedding Helix gets a white panel dropped
  // into it and cannot do anything about it from the parent — the frame is
  // cross-origin. Letting the embedder state the mode is the only fix available
  // to them.
  try {
    const q = new URLSearchParams(window.location.search).get('theme')
    if (q === 'dark' || q === 'light') return q
  } catch { /* malformed query string — fall through to the browser override */ }

  const stored = storedThemeMode()
  if (stored) return stored

  if (window.matchMedia('(prefers-color-scheme: light)').matches) return 'light'
  return 'dark'
}

export const ThemeContext = React.createContext({
  mode: 'dark' as PaletteMode,
  toggleMode: () => {},
})

export const getTooltipStyleOverrides = (isLight: boolean) => ({
  tooltip: isLight ? {} : {
    backgroundColor: '#1b1b1b',
    border: '1px solid #3a3a3a',
    borderRadius: '7px',
    boxShadow: '0 4px 12px rgba(0, 0, 0, 0.35)',
    color: '#f5f5f5',
    fontFamily: APP_FONT_FAMILY,
    fontSize: '0.75rem',
    fontWeight: 400,
    lineHeight: 1.35,
    padding: '5px 8px',
  },
  arrow: isLight ? {} : {
    color: '#1b1b1b',
  },
})

export const getDialogStyleTokens = (isLight: boolean, background: string) => {
  const surfaceBase = isLight
    ? background
    : `color-mix(in srgb, ${background} 94%, white)`

  return {
    surfaceFallback: isLight ? background : '#191919',
    surface: `color-mix(in srgb, ${surfaceBase} 80%, transparent)`,
    surfaceFilter: `blur(${isLight ? 12 : 16}px) saturate(${isLight ? 1.14 : 1.08})`,
    border: isLight
      ? '1px solid rgba(0, 0, 0, 0.10)'
      : '1px solid rgba(255, 255, 255, 0.08)',
    shadow: isLight
      ? '0 24px 64px -24px rgba(0, 0, 0, 0.35)'
      : 'inset 0 1px rgba(255, 255, 255, 0.04), 0 24px 72px -20px rgba(0, 0, 0, 0.90)',
    backdropFallback: isLight ? 'rgba(255, 255, 255, 0.60)' : 'rgba(0, 0, 0, 0.64)',
    backdrop: `color-mix(in srgb, ${background} ${isLight ? 60 : 64}%, transparent)`,
  }
}

/**
 * Shared dropdown behaviour for every Select: anchored below the field so it
 * always opens downward from a fixed point rather than overlaying the control
 * aligned to the selected item, and capped so long lists scroll.
 *
 * Spread this when a Select needs to override one part — passing `MenuProps`
 * on a component replaces the theme default outright, so overriding naively
 * loses the anchoring and the max height.
 */
export const SELECT_MENU_PROPS = {
  sx: {
    zIndex: 100003,
  },
  anchorOrigin: { vertical: 'bottom' as const, horizontal: 'left' as const },
  transformOrigin: { vertical: 'top' as const, horizontal: 'left' as const },
  PaperProps: {
    sx: { maxHeight: 320 },
  },
}

export const getFlatSelectOverrides = (isLight: boolean) => ({
  '&:has(> .MuiSelect-select)': {
    borderRadius: '6px',
    backgroundColor: 'transparent',
    transition: 'background-color 0.1s, color 0.1s',
    '& .MuiOutlinedInput-notchedOutline': {
      border: '0 !important',
    },
    '&:before, &:after': {
      borderBottom: '0 !important',
    },
    '& > .MuiSelect-select': {
      paddingTop: '6px',
      paddingBottom: '6px',
      paddingLeft: '8px',
    },
    '&:hover': {
      backgroundColor: isLight ? 'rgba(0,0,0,0.045)' : 'rgba(255,255,255,0.055)',
    },
    '&.Mui-focused': {
      backgroundColor: isLight ? 'rgba(0,0,0,0.075)' : 'rgba(255,255,255,0.085)',
    },
  },
})

/**
 * Opt back out of `getFlatSelectOverrides`.
 *
 * The flat look is right for selects that sit inline in toolbars and headers,
 * but in a form a borderless select next to an outlined TextField reads as a
 * broken field. Spread this into the `sx` of a form select to give it the same
 * outline, height and focus ring as the TextFields around it. It has to repeat
 * `!important` because the global rule uses it.
 */
export const getFormSelectSx = (isLight: boolean) => ({
  '& .MuiOutlinedInput-root:has(> .MuiSelect-select)': {
    borderRadius: '4px',
    backgroundColor: 'transparent',
    '&:hover, &.Mui-focused': {
      backgroundColor: 'transparent',
    },
    '& > .MuiSelect-select': {
      padding: '16.5px 32px 16.5px 14px',
    },
    '& .MuiOutlinedInput-notchedOutline': {
      borderWidth: '1px !important',
      borderStyle: 'solid !important',
      borderColor: isLight ? 'rgba(0, 0, 0, 0.23)' : 'rgba(255, 255, 255, 0.23)',
    },
    '&:hover .MuiOutlinedInput-notchedOutline': {
      borderColor: isLight ? 'rgba(0, 0, 0, 0.87)' : '#fff',
    },
    '&.Mui-focused .MuiOutlinedInput-notchedOutline': {
      borderWidth: '2px !important',
      borderColor: 'primary.main',
    },
  },
})

export const ThemeProviderWrapper = ({ children }: { children: ReactNode }) => {
  const themeConfig = useThemeConfig()
  const api = useApi()
  const [mode, setMode] = useState<PaletteMode>(getInitialMode)

  // Live OS preference sync while the browser has no explicit override.
  useEffect(() => {
    // A pinned ?theme= must stay pinned. Otherwise an embedder's dark panel
    // would flip to light the moment the VIEWER's OS changed — a setting the
    // embedding site has no control over and no way to observe.
    if (themePinnedByQuery()) return

    const mql = window.matchMedia('(prefers-color-scheme: light)')
    const handler = (e: MediaQueryListEvent) => {
      if (storedThemeMode()) return
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

    const fontSmoothingStyles = TYPOGRAPHY.smoothing
      ? { WebkitFontSmoothing: 'antialiased', MozOsxFontSmoothing: 'grayscale' }
      : { WebkitFontSmoothing: 'auto', MozOsxFontSmoothing: 'auto' }

    const menuSurfaceBg = isLight ? 'rgba(255, 255, 255, 0.98)' : 'rgba(27, 27, 27, 0.98)'
    const menuBorder = isLight ? '1px solid rgba(0,0,0,0.12)' : '1px solid rgba(255,255,255,0.12)'
    const menuTextColor = isLight ? '#262626' : '#f1f1f1'
    const menuHoverBg = isLight ? 'rgba(0,0,0,0.045)' : 'rgba(255,255,255,0.055)'
    const menuSelectedBg = isLight ? 'rgba(0,0,0,0.075)' : 'rgba(255,255,255,0.085)'
    const menuDividerColor = isLight ? 'rgba(0,0,0,0.10)' : 'rgba(255,255,255,0.09)'
    const menuShadow = isLight ? '0 10px 30px rgba(0,0,0,0.14)' : '0 10px 30px rgba(0,0,0,0.38)'

    const dialogStyles = getDialogStyleTokens(isLight, bg)
    const dialogColor = isLight ? '#333' : '#F1F1F1'

    return createTheme({
      palette: {
        primary: {
          main: themeConfig.primary,
        },
        secondary: {
          // Brand cyan #00d5ff is illegible on white, so light mode uses a
          // darker teal that still reads as the same brand family.
          main: isLight ? themeConfig.lightSecondary : themeConfig.secondary,
        },
        mode: mode,
        background: {
          default: bg,
        },
      },
      typography: {
        fontFamily: APP_FONT_FAMILY,
        fontFamilyMono: APP_MONO_FONT_FAMILY,
        fontSize: TYPOGRAPHY.bodyFontSize,
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
        // MUI floats an outlined input's label into a notch cut out of the
        // border. That reads fine on a text field you type into, but on a
        // select — where the value is already a full sentence and the control
        // is denser — the label lands half inside the box and looks crooked.
        // Site-wide, selects get a static left-aligned label above the control
        // and no notch. Scoped with :has() so text fields keep floating labels.
        MuiInputLabel: {
          styleOverrides: {
            root: {
              '.MuiFormControl-root:has(.MuiSelect-select) &': {
                position: 'static',
                transform: 'none',
                maxWidth: '100%',
                fontSize: '0.75rem',
                marginBottom: 4,
                // The floating label relies on shrink state for colour; pinned
                // above the field it should read as a quiet field label instead.
                '&.Mui-focused': {
                  color: 'inherit',
                },
              },
            },
          },
        },
        MuiSwitch: {
          styleOverrides: {
            root: {
              width: 40,
              height: 22,
              padding: 0,
              display: 'flex',
              overflow: 'visible',
            },
            switchBase: {
              padding: 3,
              color: '#fff',
              '&.Mui-checked': {
                transform: 'translateX(18px)',
                color: '#fff',
                '& + .MuiSwitch-track': {
                  opacity: 1,
                  backgroundColor: '#2563eb',
                },
              },
              '&.Mui-disabled': {
                color: '#fff',
                opacity: 0.5,
              },
              '&.Mui-disabled + .MuiSwitch-track': {
                opacity: isLight ? 0.2 : 0.25,
              },
              '&:hover': { backgroundColor: 'transparent' },
            },
            thumb: {
              width: 16,
              height: 16,
              boxShadow: 'none',
            },
            track: {
              borderRadius: 11,
              opacity: 1,
              backgroundColor: isLight ? 'rgba(0,0,0,0.25)' : 'rgba(255,255,255,0.22)',
              transition: 'background-color 150ms ease',
            },
          },
        },
        MuiCssBaseline: {
          styleOverrides: {
            // Typography tokens live in styles/typography.ts; they are mirrored
            // onto :root so plain-CSS surfaces can read them too.
            ':root': typographyCssVariables(),
            html: {
              fontFamily: APP_FONT_FAMILY,
              fontSize: TYPOGRAPHY.rootFontSize,
              ...fontSmoothingStyles,
            },
            body: {
              fontFamily: APP_FONT_FAMILY,
              ...fontSmoothingStyles,
              backgroundColor: bg,
              ...scrollbarStyles,
            },
            'button, input, optgroup, select, textarea': {
              fontFamily: 'inherit',
            },
            '*': scrollbarStyles,
          },
        },
        MuiMenu: {
          defaultProps: {
            elevation: 0,
          },
          styleOverrides: {
            root: {
              zIndex: 100003,
            },
            paper: {
              minWidth: '144px',
              padding: '5px',
              overflow: 'hidden auto',
              backgroundColor: menuSurfaceBg,
              backgroundImage: 'none',
              backdropFilter: 'blur(12px)',
              border: menuBorder,
              borderRadius: '10px',
              boxShadow: menuShadow,
              '& .MuiMenuItem-root': {
                color: menuTextColor,
                fontSize: '0.8rem',
                fontWeight: 500,
                lineHeight: 1.25,
                padding: '5px 7px',
                minHeight: '28px',
                borderRadius: '6px',
                transition: 'background-color 0.1s, color 0.1s',
                '&:hover': {
                  backgroundColor: menuHoverBg,
                },
                '&.Mui-selected': {
                  backgroundColor: menuSelectedBg,
                  '&:hover': {
                    backgroundColor: menuSelectedBg,
                  },
                },
              },
              '& .MuiListItemIcon-root': {
                minWidth: '24px',
                color: isLight ? 'rgba(0,0,0,0.58)' : 'rgba(255,255,255,0.58)',
                '& svg': {
                  width: 15,
                  height: 15,
                },
              },
              '& .MuiListItemText-primary': {
                fontSize: 'inherit',
                lineHeight: 'inherit',
              },
              '& .MuiListItemText-secondary': {
                marginTop: '2px',
                color: isLight ? 'rgba(0,0,0,0.5)' : 'rgba(255,255,255,0.48)',
                fontSize: '0.7rem',
                lineHeight: 1.25,
              },
              '& .MuiListSubheader-root': {
                minHeight: 0,
                padding: '5px 7px 3px',
                backgroundColor: 'transparent',
                color: isLight ? 'rgba(0,0,0,0.5)' : 'rgba(255,255,255,0.45)',
                fontSize: '0.7rem',
                fontWeight: 500,
                lineHeight: 1.35,
              },
              '& .MuiDivider-root': {
                borderColor: menuDividerColor,
                margin: '5px 0',
              },
            },
            list: {
              padding: 0,
            },
          },
        },
        MuiPaper: {
          styleOverrides: {
            root: {
              '&.MuiMenu-paper, &.MuiPopover-paper': {
                backgroundColor: menuSurfaceBg,
                backgroundImage: 'none',
                backdropFilter: 'blur(12px)',
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
              backgroundColor: dialogStyles.surfaceFallback,
              background: dialogStyles.surface,
              backgroundImage: 'none',
              WebkitBackdropFilter: dialogStyles.surfaceFilter,
              backdropFilter: dialogStyles.surfaceFilter,
              border: dialogStyles.border,
              color: dialogColor,
              borderRadius: 16,
              boxShadow: dialogStyles.shadow,
              transition: 'all 0.2s ease-in-out',
            },
            container: {
              // A full-screen dialog has to be bounded by the VISIBLE viewport,
              // not the layout one. With the on-screen keyboard open the layout
              // viewport is the taller of the two, so the sheet overflows it and
              // the whole thing pans — header and all — instead of its list
              // scrolling inside a fixed frame. `--app-height` tracks the visual
              // viewport (see index.html); the 100% fallback is what every
              // desktop browser resolves to anyway.
              height: 'var(--app-height, 100%)',
            },
            root: {
              zIndex: 100002, // Above floating windows (z-index 9999); tooltips (100004) render above
              transition: 'all 0.2s ease-in-out',
              '& .MuiBackdrop-root': {
                backgroundColor: dialogStyles.backdropFallback,
                background: dialogStyles.backdrop,
                WebkitBackdropFilter: 'blur(4px)',
                backdropFilter: 'blur(4px)',
                transition: 'background 0.2s ease-in-out, backdrop-filter 0.2s ease-in-out',
              },
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
          styleOverrides: getTooltipStyleOverrides(isLight),
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
            MenuProps: SELECT_MENU_PROPS,
          },
        },
        MuiOutlinedInput: {
          styleOverrides: {
            root: getFlatSelectOverrides(isLight),
            notchedOutline: {
              // The label is pinned above the control for selects, so there is
              // nothing to cut a notch for — and on a flat select there is no
              // border to cut it out of anyway.
              '.MuiFormControl-root:has(.MuiSelect-select) & legend': {
                display: 'none',
              },
            },
          },
        },
        MuiInput: {
          styleOverrides: {
            root: getFlatSelectOverrides(isLight),
          },
        },
        MuiFilledInput: {
          styleOverrides: {
            root: getFlatSelectOverrides(isLight),
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
      try { localStorage.setItem(THEME_MODE_KEY, next) } catch { /* ignore */ }
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
