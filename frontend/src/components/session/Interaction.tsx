import React, { FC, useState } from 'react'
import { styled } from '@mui/system'
import Typography from '@mui/material/Typography'
import Avatar from '@mui/material/Avatar'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'
import Link from '@mui/material/Link'
import Progress from '../widgets/Progress'
import NavigateNextIcon from '@mui/icons-material/NavigateNext'
import TerminalWindow from '../widgets/TerminalWindow'
import ClickLink from '../widgets/ClickLink'
import ConversationEditor from './ConversationEditor'
import {
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_CREATOR_SYSTEM,
  SESSION_CREATOR_USER,
  INTERACTION_STATE_EDITING,
} from '../../types'

import {
  ISessionType,
  ISessionMode,
  IInteraction,
  IServerConfig,
} from '../../types'

import {
  getFileExtension,
  isImage,
} from '../../utils/filestore'

const GeneratedImage = styled('img')({})

export const Interaction: FC<{
  session_id?: string,
  type: ISessionType,
  mode: ISessionMode,
  interaction: IInteraction,
  serverConfig: IServerConfig,
  error?: string,
  isLast?: boolean,
}> = ({
  session_id,
  type,
  mode,
  interaction,
  serverConfig,
  error = '',
  isLast = false,
}) => {

  const [ viewingError, setViewingError ] = useState(false)
  let displayMessage = ''
  let progress = 0
  let imageURLs: string[] = []
  let isLoading = isLast && interaction.creator == SESSION_CREATOR_SYSTEM && !interaction.finished
  const isImageFinetune = interaction.creator == SESSION_CREATOR_USER && type == SESSION_TYPE_IMAGE
  const isTextFinetune = interaction.creator == SESSION_CREATOR_USER && type == SESSION_TYPE_TEXT

  const isEditingConversations = interaction.state == INTERACTION_STATE_EDITING && interaction.files.find(f => f.endsWith('.jsonl')) ? true : false
  const useErrorText = interaction.error || (isLast ? error : '')

  if(type == SESSION_TYPE_TEXT) {
    displayMessage = interaction.message
    if(!displayMessage && isLoading) {
      if(interaction.progress > 0) {
        progress = interaction.progress
      } else if (interaction.state != INTERACTION_STATE_EDITING) {
        displayMessage = '🤔'
      }
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
      } else if(interaction.finetune_file) {
        displayMessage = 'Fine fining complete - you can now use the model for inference.'
      } else if(mode == SESSION_MODE_INFERENCE && interaction.files && interaction.files.length > 0) {
        imageURLs = interaction.files.filter(isImage)
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
                  interaction.files.length > 0 && interaction.files
                    .filter(file => {
                      return file.match(/\.txt$/i) ? false : true
                    })
                    .map((file) => {
                      const useURL = `${serverConfig.filestore_prefix}/${file}`
                      const filenameParts = file.split('/')
                      const label = interaction.metadata[filenameParts[filenameParts.length - 1]] || ''

                      return (
                        <Grid item xs={3} md={3} key={file}>
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
                            <Typography variant="caption">{label}</Typography>
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
          isTextFinetune && interaction.files && interaction.files.length > 0 && (
            <Box
              sx={{
                maxHeight: '400px',
                overflowY: 'auto'
              }}
            >
              <Grid container spacing={3} direction="row" justifyContent="flex-start">
                {
                  interaction.files.length > 0 && interaction.files
                    .map((file) => {
                      const useURL = `${serverConfig.filestore_prefix}/${file}`
                      const filenameParts = file.split('/')
                      const filename = filenameParts[filenameParts.length - 1] || ''

                      return (
                        <Grid item xs={3} md={3} key={file}>
                          <Box
                            sx={{
                              display: 'flex',
                              flexDirection: 'column',
                              alignItems: 'center',
                              justifyContent: 'center',
                              color: '#999',
                              cursor: 'pointer',
                            }}
                            onClick={ () => {
                              window.open(useURL)
                            }}
                          >
                            <span className={`fiv-viv fiv-size-md fiv-icon-${getFileExtension(filename)}`}></span>
                            <Typography variant="caption" sx={{
                              textAlign: 'center',
                              color: 'blue',
                              textDecoration: 'underline',
                            }}>{filename}</Typography>
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
          interaction.status && !useErrorText && !isEditingConversations && (
            <Typography variant="caption" dangerouslySetInnerHTML={{__html: interaction.status.replace(/\n/g, '<br/>')}}></Typography>
          )
        }
        {
          isEditingConversations && session_id && (
            <Box
              sx={{
                mt: 2,
              }}
            >
              <ConversationEditor
                session_id={ session_id }
              />
            </Box>
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
          useErrorText && (
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
              data={ useErrorText }
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