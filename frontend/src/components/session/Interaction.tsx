import React, { FC, useState } from 'react'
import { styled } from '@mui/system'
import Typography from '@mui/material/Typography'
import Avatar from '@mui/material/Avatar'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Link from '@mui/material/Link'
import Stepper from '@mui/material/Stepper'
import Step from '@mui/material/Step'
import StepLabel from '@mui/material/StepLabel'
import TerminalWindow from '../widgets/TerminalWindow'
import ClickLink from '../widgets/ClickLink'
import ConversationEditor from './ConversationEditor'
import LiveInteraction from './LiveInteraction'
import {
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_MODE_INFERENCE,
  SESSION_CREATOR_SYSTEM,
  SESSION_CREATOR_USER,
  INTERACTION_STATE_EDITING,
  TEXT_DATA_PREP_STAGE_NONE,
} from '../../types'

import {
  ISessionType,
  ISessionMode,
  IInteraction,
  IServerConfig,
} from '../../types'

import {
  mapFileExtension,
  isImage,
} from '../../utils/filestore'

import {
  getTextDataPrepStage,
  getTextDataPrepStageIndex,
} from '../../utils/session'

const GeneratedImage = styled('img')({})

export const Interaction: FC<{
  session_id: string,
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
  let displayMessage: string = ''
  let imageURLs: string[] = []
  let isLoading = isLast && interaction.creator == SESSION_CREATOR_SYSTEM && !interaction.finished
  
  const isImageFinetune = interaction.creator == SESSION_CREATOR_USER && type == SESSION_TYPE_IMAGE
  const isTextFinetune = interaction.creator == SESSION_CREATOR_USER && type == SESSION_TYPE_TEXT

  const dataPrepStage = getTextDataPrepStage(interaction)

  const isEditingConversations = interaction.state == INTERACTION_STATE_EDITING ? true : false
  const useErrorText = interaction.error || (isLast ? error : '')

  // in this state the last interaction is not yet "finished"
  if(isEditingConversations) {
    isLoading = false
  }

  if(isLoading) {
    // we don't display the message here - we render a LiveInteraction which handles the websockets
    // without reloading the entire app
  } else {
    if(type == SESSION_TYPE_TEXT) {
      displayMessage = interaction.message
      if(interaction.lora_dir) {
        displayMessage = 'Fine tuning complete - you can now ask the model questions...'
      }
    } else if(type == SESSION_TYPE_IMAGE) {
      if(interaction.creator == SESSION_CREATOR_USER) {
        displayMessage = interaction.message
      }
      else {
        if(interaction.lora_dir) {
          displayMessage = 'Fine tuning complete - you can now ask the model to create images...'
        } else if(mode == SESSION_MODE_INFERENCE && interaction.files && interaction.files.length > 0) {
          imageURLs = interaction.files.filter(isImage)
        }
      }
    }
  }

  if(!serverConfig || !serverConfig.filestore_prefix) return null

  console.log(interaction)
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
                            <span className={`fiv-viv fiv-size-md fiv-icon-${mapFileExtension(filename)}`}></span>
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
          dataPrepStage != TEXT_DATA_PREP_STAGE_NONE && (
            <Box
              sx={{
                mt: 4,
                mb: 4,
              }}
            >
              <Stepper activeStep={getTextDataPrepStageIndex(dataPrepStage)}>
                <Step>
                  <StepLabel>Extract Text</StepLabel>
                </Step>
                <Step>
                  <StepLabel>Generate Questions</StepLabel>
                </Step>
                <Step>
                  <StepLabel>Edit Questions</StepLabel>
                </Step>
              </Stepper>
            </Box>
          )
        }
        {
          isLoading && (
            <LiveInteraction
              session_id={ session_id }
              interaction={ interaction }             
            />
          )
        }
        {
          displayMessage && (
            <Typography>{ displayMessage }</Typography>
          )
        }
        {
          useErrorText && (
            <Alert severity="error">The system has encountered an error - <ClickLink onClick={ () => {
              setViewingError(true)
            }}>click here</ClickLink> to view the details.</Alert>
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