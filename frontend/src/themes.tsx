import React, { ReactElement } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'

const DEFAULT_THEME_NAME = 'helix'

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
  helix: {
    company: 'Helix',
    url: 'https://tryhelix.ai/',
    primary: '#7db6c7',
    secondary: '#7db6c7',
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
          alt="Helix" 
          sx={{
            height: 30,
            ml: 1,
          }}
        />
        <Typography variant="h6" sx={{
          ml: 2,
          mr: 1,
        }}>
          Helix
        </Typography>
      </Box>
    ),
  },
}

export const THEME_DOMAINS: Record<string, string> = {
  'helix.ml': 'helix',
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