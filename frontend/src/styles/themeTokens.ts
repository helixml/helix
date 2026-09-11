export const DARK_APP_BACKGROUND = '#0a0a0a'

export const LIGHT_SIDEBAR_COLORS = {
  background: '#fafafa',
  foreground: '#27272a',
  primaryLabel: 'rgba(39, 39, 42, 0.92)',
  mutedForeground: '#71717a',
  subtleForeground: 'rgba(113, 113, 122, 0.78)',
  icon: '#a1a1aa',
  controlSurface: '#f4f4f5',
  rowHover: '#fdfdfd',
  rowSelected: '#ffffff',
  border: '#e4e4e7',
} as const

export const DARK_SIDEBAR_COLORS = {
  background: '#000000',
  foreground: '#f1f3f7',
  primaryLabel: 'rgba(241, 243, 247, 0.90)',
  mutedForeground: '#a3a3a3',
  subtleForeground: 'rgba(163, 163, 163, 0.78)',
  icon: '#a1a1aa',
  controlSurface: '#0a0a0a',
  rowHover: 'rgba(241, 243, 247, 0.08)',
  rowSelected: 'rgba(241, 243, 247, 0.11)',
  border: 'rgba(255, 255, 255, 0.08)',
} as const

export const getSidebarColors = (isLight: boolean) => (
  isLight ? LIGHT_SIDEBAR_COLORS : DARK_SIDEBAR_COLORS
)
