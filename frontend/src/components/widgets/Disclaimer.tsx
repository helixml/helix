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
    <Container maxWidth={'xl'}>
      <Typography variant="body2" color="text.secondary" align="center">
        {'Open source models can make mistakes. Consider checking important information. Created by '}
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
