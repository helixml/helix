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
        <Tooltip title={showEditButton ? "Edit agent settings" : tooltipContent} placement="right">
          <Box
            sx={{
              position: 'relative',
              display: 'inline-flex',
              alignItems: 'center',
              gap: 1,
              cursor: showEditButton && onEditClick ? 'pointer' : 'default',
              padding: showEditButton ? '4px 12px 4px 4px' : 0,
              borderRadius: showEditButton ? '24px' : 0,
              backgroundColor: showEditButton ? 'rgba(255, 255, 255, 0.1)' : 'transparent',
              border: showEditButton ? '1px solid rgba(255, 255, 255, 0.2)' : 'none',
              transition: 'all 0.2s ease',
              '&:hover': showEditButton ? {
                backgroundColor: 'rgba(255, 255, 255, 0.2)',
                borderColor: 'rgba(255, 255, 255, 0.4)',
              } : {},
            }}
            onClick={showEditButton && onEditClick ? onEditClick : undefined}
          >
            <Avatar
              src={avatar}
              sx={{
                width: 32,
                height: 32,
                border: '1px solid rgba(255, 255, 255, 0.5)',
                ...avatarSx,
              }}
            />
            {showEditButton && onEditClick && (
              <Box
                sx={{
                  display: 'flex',
                  alignItems: 'center',
                  gap: 0.5,
                  color: 'rgba(255, 255, 255, 0.9)',
                  fontSize: '0.8rem',
                  fontWeight: 500,
                }}
              >
                <Edit size={14} />
                <span>Edit</span>
              </Box>
            )}
          </Box>
        </Tooltip>
      </Box>
    </Box>
  )
}

export default CreateHeader
