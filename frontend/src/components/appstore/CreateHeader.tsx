import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Avatar from '@mui/material/Avatar'

import {
  IApp,
} from '../../types'

import {
  getAppImage,
  getAppAvatar,
  getAppName,
  getAppDescription,
} from '../../utils/apps'

const CreateHeader: FC<{
  app: IApp,
}> = ({
  app,
}) => {
  const avatar = getAppAvatar(app)
  const image = getAppImage(app)
  const name = getAppName(app)
  const description = getAppDescription(app)

  return (
    <>
      {
        avatar && (
          <Box
            sx={{
              display: 'flex',
              flexDirection: 'row',
              alignItems: 'center',
              justifyContent: 'center',
            }}
          >
            <Avatar
              src={ avatar }
              sx={{
                width: '200px',
                height: '200px'
              }}
            />
          </Box>
          
        )
      }
      {
        !avatar && image && (
          <Box
            component="img"
            src={ image }
            sx={{
              maxWidth: '800px',
              maxHeight: '200px',
            }}
          />
        )
      }
      {
        name && (
          <Typography
            variant="h4"
          >
            { name }
          </Typography>
        )
      }
      {
        description && (
          <Typography
            variant="body1"
          >
            { description }
          </Typography>
        )
      }
    </>
  )
}

export default CreateHeader
