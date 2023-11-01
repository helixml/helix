import React, { FC, useState } from 'react'
import { styled } from '@mui/system'
import Typography from '@mui/material/Typography'
import Avatar from '@mui/material/Avatar'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Link from '@mui/material/Link'
import Progress from '../widgets/Progress'
import TerminalWindow from '../widgets/TerminalWindow'
import ClickLink from '../widgets/ClickLink'
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
  IServerConfig,
} from '../../types'

const GeneratedImage = styled('img')({})

export const Interaction: FC<{
  type: ISessionType,
  interaction: IInteraction,
  serverConfig: IServerConfig,
  isLast?: boolean,
}> = ({
  type,
  interaction,
  serverConfig,
  isLast = false,
}) => {

  const [ viewingError, setViewingError ] = useState(false)
  let displayMessage = ''
  let progress = 0
  let imageURLs: string[] = []
  let isLoading = isLast && interaction.creator == SESSION_CREATOR_SYSTEM && !interaction.finished
  const isImageFinetune = interaction.creator == SESSION_CREATOR_USER && type == SESSION_TYPE_IMAGE

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

  if(!serverConfig || !serverConfig.filestore_prefix) return null

  return (
    <Box key={interaction.id} sx={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem', mb:2 }}>
      <Avatar sx={{ width: 24, height: 24 }}>{interaction.creator.charAt(0)}</Avatar>
      <Box sx={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
        <Typography variant="subtitle2" sx={{ fontWeight: 'bold' }}>{interaction.creator.charAt(0).toUpperCase() + interaction.creator.slice(1)}</Typography>
        {
          isImageFinetune && interaction.files && interaction.files.length > 0 && (
            <Box
              sx={{
                maxHeight: '400px',
                overflowY: 'auto'
              }}
            >
              <Grid container spacing={3} direction="row" justifyContent="flex-start">
                {
                  interaction.files.length > 0 && interaction.files.map((file) => {
                    const useURL = `${serverConfig.filestore_prefix}/${file}`

                    return (
                      <Grid item xs={4} md={4} key={file}>
                        <Box
                          sx={{
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: 'center',
                            justifyContent: 'center',
                            color: '#999'
                          }}
                        >
                          <Box
                            component="img"
                            src={useURL}
                            sx={{
                              height: '50px',
                              border: '1px solid #000000',
                              filter: 'drop-shadow(3px 3px 5px rgba(0, 0, 0, 0.2))',
                              mb: 1,
                            }}
                          />
                        </Box>
                      </Grid>
                    )
                  })
                    
                }
              </Grid>
            </Box>
          )
        }
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
            const useURL = `${serverConfig.filestore_prefix}/${imageURL}`

            return (
              <Box
                sx={{
                  mt: 2,
                }}
                key={ useURL }
              >
                <Link
                  href={ useURL }
                  target="_blank"
                >
                  <GeneratedImage
                    sx={{
                      height: '600px',
                      maxHeight: '600px',
                      border: '1px solid #000000',
                      filter: 'drop-shadow(5px 5px 10px rgba(0, 0, 0, 0.5))',
                    }}
                    src={ useURL }
                  />  
                </Link>
              </Box>
            )
            
          })
        }
        {
          interaction.error && (
            <Alert severity="error">The system has encountered an error - <ClickLink onClick={ () => {
              setViewingError(true)
            }}>click here</ClickLink> to view the details.</Alert>
          ) 
        }
        {
          viewingError && (
            <TerminalWindow
              open
              title="Error"
              data={ interaction.error }
              onClose={ () => {
                setViewingError(false)
              }}
            /> 
          )
        }
      </Box>
    </Box>
  )   
}

export default Interaction