import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Avatar from '@mui/material/Avatar'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import { Edit } from 'lucide-react'

import {
  IApp,
} from '../../types'

import {
  getAppAvatarUrl,
  getAppName,
  getAppDescription,
} from '../../utils/apps'

const CreateHeader: FC<{
  app: IApp,
  avatarSx?: any,
  showEditButton?: boolean,
  onEditClick?: () => void,
}> = ({
  app,
  avatarSx = {},
  showEditButton = false,
  onEditClick,
}) => {
  const avatar = getAppAvatarUrl(app)
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
      <Box
        sx={{
          position: 'relative',
          display: 'flex',
          alignItems: 'center',
          gap: 2,
        }}
      >
        <Tooltip title={tooltipContent} placement="right">
          <Box
            sx={{
              position: 'relative',
              display: 'inline-block',
              cursor: showEditButton && onEditClick ? 'pointer' : 'default',
              width: 40,
              height: 40,
              '&:hover .edit-avatar-icon': {
                opacity: 1,
              },
            }}
            onClick={showEditButton && onEditClick ? onEditClick : undefined}
          >
            <Avatar
              src={avatar}
              sx={{
                width: 40,
                height: 40,
                border: '1px solid #fff',
                ...avatarSx,
              }}
            />
            {showEditButton && onEditClick && (
              <Box
                sx={{
                  position: 'absolute',
                  top: 0,
                  left: 0,
                  width: '100%',
                  height: '100%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                }}
              >
                <Edit
                  size={22}
                  color="white"
                  style={{
                    opacity: 0,
                    transition: 'opacity 0.2s',
                  }}
                  className="edit-avatar-icon"
                />
              </Box>
            )}
          </Box>
        </Tooltip>
      </Box>
    </Box>
  )
}

export default CreateHeader
