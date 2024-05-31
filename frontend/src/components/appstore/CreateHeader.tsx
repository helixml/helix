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
              mb: 1,
            }}
          >
            <Avatar
              src={ avatar }
              sx={{
                width: '200px',
                height: '200px',
                border: '1px solid #fff',
              }}
            />
          </Box>
          
        )
      }
      {
        !avatar && image && (
          <Box
            component="img"
            sx={{
              width: '100%',
              height: '250px',
              backgroundImage: `url(${image})`,
              // no repeat and cover and center
              backgroundSize: 'cover',
              backgroundRepeat: 'no-repeat',
              backgroundPosition: 'center',
              border: '1px solid #fff',
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
            sx={{
              mb: 1,
            }}
          >
            { description }
          </Typography>
        )
      }
      
    </>
  )
}

export default CreateHeader
