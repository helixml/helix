import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import useThemeConfig from '../../hooks/useThemeConfig'

const Disclaimer: FC<{
  
}> = ({
  
}) => {
  const themeConfig = useThemeConfig()

  return (
    <Box>
      <Typography variant="body2" color="text.secondary" align="center">
        {'Open source models can make mistakes. Check facts, dates and events. Created by '}
        <Link color="inherit" href={ themeConfig.url }>
          { themeConfig.company }
        </Link>{' '}
        {new Date().getFullYear()}
        {'.'}
      </Typography>
    </Box>
  )
}

export default Disclaimer
