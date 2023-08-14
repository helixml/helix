import React, { ReactElement } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

const DEFAULT_THEME_NAME = 'lilypad'

export interface ITheme {
  company: string,
  url: string,
  primary: string,
  secondary: string,
  activeSections: string[],
  logo: {
    (): ReactElement,
  },
}

export const THEMES: Record<string, ITheme> = {
  lilypad: {
    company: 'Lilypad',
    url: 'https://lilypad.tech/',
    primary: '#2D4182',
    secondary: '#1FC3CD',
    // this means ALL
    activeSections: [],
    logo: () => (
      <Box sx={{
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
      }}>
        <Box
          component="img"
          src="/img/logo.png"
          alt="Lilypad" 
          sx={{
            height: 40,
          }}
        />
        <Typography variant="h6" sx={{
          ml: 1,
        }}>
          Demos
        </Typography>
      </Box>
    ),
  },
}

export const THEME_DOMAINS: Record<string, string> = {
  'lilypad.tech': 'lilypad',
}


export const getThemeName = (): string => {
  if (typeof document !== "undefined") {
    const params = new URLSearchParams(new URL(document.URL).search);
    const queryValue = params.get('theme');
    if(queryValue) {
      localStorage.setItem('theme', queryValue)
    }
  }
  const localStorageValue = localStorage.getItem('theme')
  if(localStorageValue) {
    if(THEMES[localStorageValue]) {
      return localStorageValue
    }
    else {
      localStorage.removeItem('theme')
      return DEFAULT_THEME_NAME
    }
  }
  if (typeof document !== "undefined") {
    const domainName = THEME_DOMAINS[document.location.hostname]
    if(domainName && THEMES[domainName]) return THEME_DOMAINS[domainName]
    return DEFAULT_THEME_NAME
  }
  return DEFAULT_THEME_NAME
}