import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

interface PageSectionHeaderProps {
  title: string
  description: string
  action?: ReactNode
}

const PageSectionHeader: FC<PageSectionHeaderProps> = ({ title, description, action }) => (
  <Stack
    direction={{ xs: 'column', sm: 'row' }}
    justifyContent="space-between"
    alignItems={{ xs: 'stretch', sm: 'flex-start' }}
    spacing={2}
    sx={{ mb: 4 }}
  >
    <Box sx={{ minWidth: 0 }}>
      <Typography
        variant="h4"
        sx={{
          color: 'text.primary',
          fontWeight: 700,
          letterSpacing: '-0.02em',
        }}
      >
        {title}
      </Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mt: 1 }}>
        {description}
      </Typography>
    </Box>
    {action && <Box sx={{ flexShrink: 0 }}>{action}</Box>}
  </Stack>
)

export default PageSectionHeader
