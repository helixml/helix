import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'

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
        Session {session?.name} in which we {session?.mode.toLowerCase()} {session?.type.toLowerCase()} with {session?.model_name} 
        { session?.lora_dir ? ` finetuned on ${session?.lora_dir.split('/').pop()}` : '' }...
      </Typography>
      <Typography
        sx={{
          fontSize: "small",
          color: "gray",
          flexGrow: 0,
        }}
      >
        <Link href="/files?path=%2Fsessions" onClick={(e) => {
          e.preventDefault()
          navigate('files', {
            path: `/sessions/${session?.id}`
          })
        }}>View Files</Link>
      </Typography>
    </Box>
  )
}

export default SessionHeader