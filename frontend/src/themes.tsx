import React, { ReactElement } from 'react'
import Box from '@mui/material/Box'

const DEFAULT_THEME_NAME = 'helix'

export interface ITheme {
  company: string,
  url: string,
  primary: string,
  secondary: string,
  darkIcon: string,
  darkIconHover: string,
  darkHighlight: string,
  darkScrollbar: string,
  darkScrollbarThumb: string,
  darkScrollbarHover: string,
  darkBackgroundColor: string,
  darkBackgroundImage: string,
  darkBorder: string,
  lightIcon: string,
  lightIconHover: string,
  lightBackgroundColor: string,
  lightBackgroundImage: string,
  lightBorder: string,
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
    darkIcon: '#5d5d7b',
    darkIconHover: '#00d5ff',
    darkHighlight: '#00d5ff',
    darkScrollbar: '#1a1a1d',
    darkScrollbarThumb: '#2b2b2f',
    darkScrollbarHover: '#3c3c40',
    darkBackgroundColor: "#070714",
    darkBackgroundImage: "url('/img/nebula-dark.png')",
    darkBorder: "0.1rem solid #303047",
    lightIcon: '#5d5d7b',
    lightIconHover: '#00d5ff',
    lightBackgroundColor: "#ffffff",
    lightBackgroundImage: "url('/img/nebula-light.png')",
    lightBorder: "1px solid #aeaeae",
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
            mx: 1,
          }}
        />
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