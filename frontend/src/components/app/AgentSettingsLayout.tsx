import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { alpha } from '@mui/material/styles'

export const AgentSettingsPage: FC<{ children: ReactNode }> = ({ children }) => (
  <Stack sx={{ width: '100%', maxWidth: 896, mx: 'auto', pt: 2, pb: 10 }} spacing={6}>
    {children}
  </Stack>
)

export const AgentSettingsSection: FC<{
  title: string
  description: string
  children: ReactNode
}> = ({ title, description, children }) => (
  <Box component="section">
    <Box sx={{ px: { xs: 1.5, sm: 2 }, mb: 1.5 }}>
      <Typography
        component="h2"
        sx={{
          color: 'text.primary',
          fontSize: '1.125rem',
          fontWeight: 650,
          lineHeight: 1.35,
          letterSpacing: '-0.025em',
        }}
      >
        {title}
      </Typography>
      <Typography
        color="text.secondary"
        sx={{ mt: 0.5, maxWidth: 640, fontSize: '0.8125rem', lineHeight: 1.45 }}
      >
        {description}
      </Typography>
    </Box>
    <Stack spacing={0.5}>{children}</Stack>
  </Box>
)

export const AgentSettingsRow: FC<{ children: ReactNode }> = ({ children }) => (
  <Box
    sx={(theme) => ({
      borderRadius: 3,
      px: { xs: 1.5, sm: 2 },
      py: 2,
      backgroundColor: alpha(theme.palette.text.primary, theme.palette.mode === 'dark' ? 0.018 : 0.025),
      transition: 'background-color 120ms ease',
      '&:hover': {
        backgroundColor: alpha(theme.palette.text.primary, theme.palette.mode === 'dark' ? 0.032 : 0.045),
      },
    })}
  >
    {children}
  </Box>
)
