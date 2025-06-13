import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Avatar from '@mui/material/Avatar'
import Tooltip from '@mui/material/Tooltip'

import {
  IApp,
} from '../../types'

import {
  getAppAvatar,
  getAppName,
  getAppDescription,
} from '../../utils/apps'

const CreateHeader: FC<{
  app: IApp,
  avatarSx?: any,
}> = ({
  app,
  avatarSx = {},
}) => {
  const avatar = getAppAvatar(app) || '/img/logo.png'
  const name = getAppName(app)
  const description = getAppDescription(app)

  const tooltipContent = (
    <Box>
      {name && <Box sx={{ fontWeight: 600 }}>{name}</Box>}
      {description && <Box sx={{ fontSize: '0.85rem', color: '#aaa' }}>{description}</Box>}
    </Box>
  )

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'flex-start',
        gap: 2,
        mb: 2,
      }}
    >
      <Tooltip title={tooltipContent} placement="right">
        <Avatar
          src={avatar}
          sx={{
            width: 40,
            height: 40,
            border: '1px solid #fff',
            mr: 2,
            ...avatarSx,
          }}
        />
      </Tooltip>
    </Box>
  )
}

export default CreateHeader
