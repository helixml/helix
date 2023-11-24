import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import JsonWindowLink from '../widgets/JsonWindowLink'

import {
  ISession,
} from '../../types'

import useRouter from '../../hooks/useRouter'

export const SessionHeader: FC<{
  session: ISession,
}> = ({
  session,
}) => {
  const {navigate} = useRouter()

  return (
    <Box
      sx={{
        width: '100%',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
        mb: 2,
      }}
    >
      <Typography
        sx={{
          fontSize: "small",
          color: "gray",
          flexGrow: 1,
        }}
      >
        {session?.name}
      </Typography>
      <Typography
        sx={{
          fontSize: "small",
          color: "gray",
          flexGrow: 0,
        }}
      >
        <Link href="/files?path=%2Fsessions"
          onClick={(e) => {
            e.preventDefault()
            navigate('files', {
              path: `/sessions/${session?.id}`
            })
          }}
        >
          View Files
        </Link>
      </Typography>
      <Typography
        sx={{
          fontSize: "small",
          color: "gray",
          flexGrow: 0,
          pl: 1,
          pr: 1,
        }}
      >
        |
      </Typography>
      <JsonWindowLink
        data={ session } 
      >
        <Typography
          sx={{
            fontSize: "small",
            flexGrow: 0,
            textDecoration: 'underline',
          }}
        >
          Info
        </Typography>
      </JsonWindowLink>
    </Box>
  )
}

export default SessionHeader