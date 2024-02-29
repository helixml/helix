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
  darkText: string,
  darkPanel: string,
  lightIcon: string,
  lightIconHover: string,
  lightHighlight: string,
  lightBackgroundColor: string,
  lightBackgroundImage: string,
  lightBorder: string,
  lightText: string,
  lightPanel: string,
  neutral900: string,
  neutral800: string,
  neutral700: string,
  neutral600: string,
  neutral500: string,
  neutral400: string,
  neutral300: string,
  neutral200: string,
  neutral100: string,
  magentaRoot: string,
  magentaLight: string,
  magentaDark: string,
  tealRoot: string,
  tealLight: string,
  tealDark: string,
  redRoot: string,
  redLight: string,
  yellowRoot: string,
  yellowLight: string,
  greenRoot: string,
  greenLight: string,
  deepPurple: string,
  deepBlue: string,
  deepGreen: string,
  // active sections
  activeSections: string[],
  logo: {
    (): ReactElement,
  },
}

export const THEMES: Record<string, ITheme> = {
  helix: {
    company: 'Helix',
    url: 'https://tryhelix.ai/',
    primary: '#5d5d7b',
    secondary: '#00d5ff',
    darkIcon: '#5d5d7b',
    darkIconHover: '#00d5ff',
    darkHighlight: '#00d5ff',
    darkScrollbar: '#1a1a1d',
    darkScrollbarThumb: '#2b2b2f',
    darkScrollbarHover: '#3c3c40',
    darkBackgroundColor: "#070714",
    darkBackgroundImage: "url('/img/nebula-dark.png')",
    darkBorder: "0.1rem solid #303047",
    darkText: "#ffffff",
    darkPanel: "#10101e",
    lightIcon: '#5d5d7b',
    lightIconHover: '#00d5ff',
    lightHighlight: '#00d5ff',
    lightBackgroundColor: "#ffffff",
    lightBackgroundImage: "url('/img/nebula-light.png')",
    lightBorder: "1px solid #aeaeae",
    lightText: "#333",
    lightPanel: "#f4f4f4",
    // colors
    neutral900: '#000000',
    neutral800: '#070714',
    neutral700: '#10101e',
    neutral600: '#1A1A2F',
    neutral500: '#303047',
    neutral400: '#505D7B',
    neutral300: '#B1B1D1',
    neutral200: '#E0E0F1',
    neutral100: '#FFFFFF',
    magentaRoot: '#EF2EC6',
    magentaLight: '#FBDEF5',
    magentaDark: '#9A0C95',
    tealRoot: '#00D5FF',
    tealLight: '#D5F4FA',
    tealDark: '#17839A',
    redRoot: '#FC3600',
    redLight: '#F0BEB0',
    yellowRoot: '#FCDB05',
    yellowLight: '#FBE286',
    greenRoot: '#3BF959',
    greenLight: '#B4FDC0',
    deepPurple: '#250B1A',
    deepBlue: '#1F2236',
    deepGreen: '#193533',
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