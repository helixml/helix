import React, { FC } from 'react'
import { styled } from '@mui/system'
import Typography from '@mui/material/Typography'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import Link from '@mui/material/Link'
import Progress from '../widgets/Progress'
import {
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_CREATOR_SYSTEM,
  SESSION_CREATOR_USER,
} from '../../types'

import {
  ISessionType,
  ISessionMode,
  IInteraction,
} from '../../types'

const GeneratedImage = styled('img')()

export const Interaction: FC<{
  type: ISessionType,
  interaction: IInteraction,
  isLast?: boolean,
}> = ({
  type,
  interaction,
  isLast = false,
}) => {

  let displayMessage = ''
  let progress = 0
  let imageURLs: string[] = []
  let isLoading = isLast && interaction.creator == SESSION_CREATOR_SYSTEM && !interaction.finished

  if(type == SESSION_TYPE_TEXT) {
    displayMessage = interaction.message
    if(!displayMessage && isLoading) {
      displayMessage = '🤔'
    }
  } else if(type == SESSION_TYPE_IMAGE) {
    if(interaction.creator == SESSION_CREATOR_USER) {
      displayMessage = interaction.message
    }
    else {
      if(isLoading) {
        if(interaction.progress > 0) {
          progress = interaction.progress
        } else {
          displayMessage = '🤔'
        }
      } else if(interaction.files && interaction.files.length > 0) {
        imageURLs = interaction.files
      }
    }
  }

  return (
    <Box key={interaction.id} sx={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem', mb:2 }}>
      <Avatar sx={{ width: 24, height: 24 }}>{interaction.creator.charAt(0)}</Avatar>
      <Box sx={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>{interaction.creator.charAt(0).toUpperCase() + interaction.creator.slice(1)}</Typography>
        {
          displayMessage && (
            <Typography dangerouslySetInnerHTML={{__html: displayMessage.replace(/\n/g, '<br/>')}}></Typography>
          )
        }
        {
          progress > 0 && (
            <Progress
              progress={ progress }
            />
          )
        }
        {
          imageURLs.map((imageURL: string) => {
            return (
              <Box
                sx={{
                  mt: 2,
                }}
                key={ imageURL }
              >
                <Link
                  href={ imageURL }
                  target="_blank"
                >
                  <GeneratedImage
                    sx={{
                      height: '600px',
                      maxHeight: '600px',
                      border: '1px solid #000000',
                      filter: 'drop-shadow(5px 5px 10px rgba(0, 0, 0, 0.5))',
                    }}
                    src={ imageURL }
                  />  
                </Link>
              </Box>
            )
            
          })
        }
      </Box>
    </Box>
  )   
}

export default Interaction