import React, { useContext } from 'react'
import { act, fireEvent, render, screen } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

import {
  getDialogStyleTokens,
  getFlatSelectOverrides,
  getTooltipStyleOverrides,
  ThemeContext,
  ThemeProviderWrapper,
} from './theme'

const { updateColorScheme, api } = vi.hoisted(() => {
  const updateColorScheme = vi.fn(() => Promise.resolve())
  return {
    updateColorScheme,
    api: {
      getApiClient: () => ({ v1UsersMeColorSchemeUpdate: updateColorScheme }),
    },
  }
})

vi.mock('../hooks/useApi', () => ({ default: () => api }))

let systemThemeHandler: ((event: MediaQueryListEvent) => void) | undefined

function setSystemLightMode(matches: boolean) {
  vi.stubGlobal('matchMedia', vi.fn(() => ({
    matches,
    addEventListener: (_event: string, handler: (event: MediaQueryListEvent) => void) => {
      systemThemeHandler = handler
    },
    removeEventListener: vi.fn(),
  })))
}

function ThemeToggle() {
  const { mode, toggleMode } = useContext(ThemeContext)
  return React.createElement('button', { onClick: toggleMode }, mode)
}

beforeEach(() => {
  localStorage.clear()
  window.history.replaceState({}, '', '/')
  systemThemeHandler = undefined
  updateColorScheme.mockClear()
  setSystemLightMode(true)
})

describe('theme mode preference', () => {
  it('persists a manual toggle and ignores later OS changes', () => {
    const { unmount } = render(React.createElement(
      ThemeProviderWrapper,
      null,
      React.createElement(ThemeToggle),
    ))

    fireEvent.click(screen.getByRole('button', { name: 'light' }))
    expect(screen.getByRole('button', { name: 'dark' })).toBeInTheDocument()
    expect(localStorage.getItem('themeMode')).toBe('dark')

    act(() => systemThemeHandler?.({ matches: true } as MediaQueryListEvent))
    expect(screen.getByRole('button', { name: 'dark' })).toBeInTheDocument()
    expect(updateColorScheme).toHaveBeenCalledTimes(1)
    expect(updateColorScheme).toHaveBeenCalledWith({ color_scheme: 'dark' })

    unmount()
    render(React.createElement(
      ThemeProviderWrapper,
      null,
      React.createElement(ThemeToggle),
    ))
    expect(screen.getByRole('button', { name: 'dark' })).toBeInTheDocument()
  })

  it('goes back to following the OS when toggled to the OS mode', () => {
    render(React.createElement(ThemeProviderWrapper, null, React.createElement(ThemeToggle)))

    fireEvent.click(screen.getByRole('button', { name: 'light' }))
    fireEvent.click(screen.getByRole('button', { name: 'dark' }))
    expect(localStorage.getItem('themeMode')).toBeNull()

    act(() => systemThemeHandler?.({ matches: false } as MediaQueryListEvent))
    expect(screen.getByRole('button', { name: 'dark' })).toBeInTheDocument()
  })

  it('drops an override once the OS reaches it, then follows the OS again', () => {
    render(React.createElement(ThemeProviderWrapper, null, React.createElement(ThemeToggle)))
    fireEvent.click(screen.getByRole('button', { name: 'light' }))
    expect(localStorage.getItem('themeMode')).toBe('dark')

    act(() => systemThemeHandler?.({ matches: false } as MediaQueryListEvent))
    expect(localStorage.getItem('themeMode')).toBeNull()
    expect(screen.getByRole('button', { name: 'dark' })).toBeInTheDocument()

    act(() => systemThemeHandler?.({ matches: true } as MediaQueryListEvent))
    expect(screen.getByRole('button', { name: 'light' })).toBeInTheDocument()
  })

  it('drops a stored mode that matches the OS on load', () => {
    localStorage.setItem('themeMode', 'light')
    render(React.createElement(ThemeProviderWrapper, null, React.createElement(ThemeToggle)))

    expect(localStorage.getItem('themeMode')).toBeNull()
    act(() => systemThemeHandler?.({ matches: false } as MediaQueryListEvent))
    expect(screen.getByRole('button', { name: 'dark' })).toBeInTheDocument()
  })

  it('gives an explicit query mode precedence over the stored mode', () => {
    localStorage.setItem('themeMode', 'dark')
    window.history.replaceState({}, '', '/?theme=light')

    render(React.createElement(
      ThemeProviderWrapper,
      null,
      React.createElement(ThemeToggle),
    ))

    expect(screen.getByRole('button', { name: 'light' })).toBeInTheDocument()
  })
})

describe('Helix for Mac theme bridge', () => {
  function renderFramed(parent: { postMessage: ReturnType<typeof vi.fn> } | Window) {
    Object.defineProperty(window, 'parent', { value: parent, configurable: true })
    return render(React.createElement(ThemeProviderWrapper, null, React.createElement(ThemeToggle)))
  }

  it('reports the OS mode, a toggled override, and the OS catching up to the embedding app', () => {
    const parent = { postMessage: vi.fn() }
    renderFramed(parent)
    expect(parent.postMessage).toHaveBeenLastCalledWith(
      { type: 'helix:theme', mode: 'light', explicit: false }, '*')

    fireEvent.click(screen.getByRole('button', { name: 'light' }))
    expect(parent.postMessage).toHaveBeenLastCalledWith(
      { type: 'helix:theme', mode: 'dark', explicit: true }, '*')

    act(() => systemThemeHandler?.({ matches: false } as MediaQueryListEvent))
    expect(parent.postMessage).toHaveBeenLastCalledWith(
      { type: 'helix:theme', mode: 'dark', explicit: false }, '*')
    Object.defineProperty(window, 'parent', { value: window, configurable: true })
  })

  it('posts nothing when not embedded', () => {
    const postMessage = vi.spyOn(window, 'postMessage')
    renderFramed(window)
    expect(postMessage).not.toHaveBeenCalled()
    postMessage.mockRestore()
  })
})

describe('tooltip theme', () => {
  it('uses the compact bordered surface in dark mode', () => {
    expect(getTooltipStyleOverrides(false)).toMatchObject({
      tooltip: {
        backgroundColor: '#1b1b1b',
        border: '1px solid #3a3a3a',
        borderRadius: '7px',
        color: '#f5f5f5',
        fontSize: '0.75rem',
        padding: '5px 8px',
      },
      arrow: {
        color: '#1b1b1b',
      },
    })
  })

  it('leaves light-mode tooltip styling to the MUI defaults', () => {
    expect(getTooltipStyleOverrides(true)).toEqual({
      tooltip: {},
      arrow: {},
    })
  })
})

describe('dialog theme', () => {
  it('lifts the dark dialog above the app background while preserving the glass treatment', () => {
    expect(getDialogStyleTokens(false, '#0a0a0a')).toEqual({
      surfaceFallback: '#191919',
      surface: 'color-mix(in srgb, color-mix(in srgb, #0a0a0a 94%, white) 80%, transparent)',
      surfaceFilter: 'blur(16px) saturate(1.08)',
      border: '1px solid rgba(255, 255, 255, 0.08)',
      shadow: 'inset 0 1px rgba(255, 255, 255, 0.04), 0 24px 72px -20px rgba(0, 0, 0, 0.90)',
      backdropFallback: 'rgba(0, 0, 0, 0.64)',
      backdrop: 'color-mix(in srgb, #0a0a0a 64%, transparent)',
    })
  })

  it('keeps the same separation treatment in light mode', () => {
    expect(getDialogStyleTokens(true, '#ffffff')).toMatchObject({
      surfaceFallback: '#ffffff',
      surface: 'color-mix(in srgb, #ffffff 80%, transparent)',
      surfaceFilter: 'blur(12px) saturate(1.14)',
      border: '1px solid rgba(0, 0, 0, 0.10)',
      backdropFallback: 'rgba(255, 255, 255, 0.60)',
      backdrop: 'color-mix(in srgb, #ffffff 60%, transparent)',
    })
  })
})

describe('select theme', () => {
  it('renders outlined selects as flat controls', () => {
    expect(getFlatSelectOverrides(false)).toMatchObject({
      '&:has(> .MuiSelect-select)': {
        backgroundColor: 'transparent',
        '& .MuiOutlinedInput-notchedOutline': {
          border: '0 !important',
        },
        '&:before, &:after': {
          borderBottom: '0 !important',
        },
        '&:hover': {
          backgroundColor: 'rgba(255,255,255,0.055)',
        },
      },
    })
  })
})
