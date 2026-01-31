import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import useThemeConfig from '../../hooks/useThemeConfig'
import useAccount from '../../hooks/useAccount'

const Disclaimer: FC<{
  
}> = ({
  
}) => {
  const themeConfig = useThemeConfig()
  const account = useAccount()

  return (
    <Typography variant="body2" color="text.secondary" align="center">
      {'LLMs can make mistakes. Check facts, dates and events. '}
      <Link color="inherit" href={ themeConfig.url }>
        { themeConfig.company }
      </Link>{' '}
      { account.serverConfig.version }.
    </Typography>
  )
}

export default Disclaimer
