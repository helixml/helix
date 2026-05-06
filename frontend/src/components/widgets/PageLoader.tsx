import { FC } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'

interface PageLoaderProps {
  message?: string
  // minHeight controls how much vertical space the loader claims. Default is
  // tall enough to feel centered on a typical detail page; pass a smaller value
  // for use inside a tab.
  minHeight?: number | string
}

// PageLoader is a centered spinner used while a page or tab is fetching its
// initial payload. Shared so loading states feel consistent across the app.
const PageLoader: FC<PageLoaderProps> = ({ message, minHeight = '60vh' }) => {
  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        gap: 2,
        minHeight,
        width: '100%',
      }}
    >
      <CircularProgress size={36} thickness={4} />
      {message && (
        <Typography variant="body2" color="text.secondary">
          {message}
        </Typography>
      )}
    </Box>
  )
}

export default PageLoader
