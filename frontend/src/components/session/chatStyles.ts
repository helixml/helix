import { Theme } from '@mui/material/styles'

export const getChatColors = (theme: Theme) => {
  const dark = theme.palette.mode === 'dark'

  return {
    canvas: dark ? theme.palette.background.default : '#fafafa',
    surface: dark ? '#18181b' : '#ffffff',
    composerSurface: dark ? '#111113' : '#ffffff',
    surfaceRaised: dark ? '#1f1f23' : '#f4f4f5',
    userBubble: dark ? 'rgba(255, 255, 255, 0.045)' : '#f0f0f2',
    border: dark ? 'rgba(255, 255, 255, 0.07)' : 'rgba(0, 0, 0, 0.09)',
    borderStrong: dark ? 'rgba(255, 255, 255, 0.13)' : 'rgba(0, 0, 0, 0.16)',
    foreground: dark ? '#f5f5f5' : '#18181b',
    assistantForeground: dark ? '#e8e8e8' : 'rgba(24, 24, 27, 0.8)',
    muted: dark ? '#b8b8b8' : '#52525b',
    subtle: dark ? '#969696' : '#71717a',
  }
}
