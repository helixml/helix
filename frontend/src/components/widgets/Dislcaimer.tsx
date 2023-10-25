import React, { FC } from 'react'
import Container from '@mui/material/Container'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import useThemeConfig from '../../hooks/useThemeConfig'

const Disclaimer: FC<{
  
}> = ({
  
}) => {
  const themeConfig = useThemeConfig()

  return (
    <Container maxWidth={'xl'} sx={{ height: '5vh' }}>
      <Typography variant="body2" color="text.secondary" align="center">
        {'Open source models may produce inaccurate information about people, places, or facts. Created by '}
        <Link color="inherit" href={ themeConfig.url }>
          { themeConfig.company }
        </Link>{' '}
        {new Date().getFullYear()}
        {'.'}
      </Typography>
    </Container>
  )
}

export default Disclaimer
