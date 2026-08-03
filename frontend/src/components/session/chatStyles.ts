import { Theme } from '@mui/material/styles'

export const CHAT_FONT_FAMILY = '"DM Sans Variable", "DM Sans", Inter, system-ui, sans-serif'
export const CHAT_MONO_FONT_FAMILY = '"JetBrains Mono Variable", "SFMono-Regular", Consolas, monospace'

export const getChatColors = (theme: Theme) => {
  const dark = theme.palette.mode === 'dark'

  return {
    canvas: dark ? '#0a0a0a' : '#fafafa',
    surface: dark ? '#18181b' : '#ffffff',
    surfaceRaised: dark ? '#1f1f23' : '#f4f4f5',
    userBubble: dark ? 'rgba(255, 255, 255, 0.045)' : '#f0f0f2',
    border: dark ? 'rgba(255, 255, 255, 0.07)' : 'rgba(0, 0, 0, 0.09)',
    borderStrong: dark ? 'rgba(255, 255, 255, 0.13)' : 'rgba(0, 0, 0, 0.16)',
    foreground: dark ? '#f5f5f5' : '#18181b',
    assistantForeground: dark ? 'rgba(245, 245, 245, 0.8)' : 'rgba(24, 24, 27, 0.8)',
    muted: dark ? '#a1a1aa' : '#71717a',
  }
}
