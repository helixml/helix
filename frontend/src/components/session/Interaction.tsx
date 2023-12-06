import React, { FC, useState, useMemo } from 'react'
import { styled, useTheme } from '@mui/system'
import Typography from '@mui/material/Typography'
import Avatar from '@mui/material/Avatar'
import Alert from '@mui/material/Alert'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Link from '@mui/material/Link'
import Stepper from '@mui/material/Stepper'
import Step from '@mui/material/Step'
import StepLabel from '@mui/material/StepLabel'
import ReplayIcon from '@mui/icons-material/Replay'
import ArrowForwardIcon from '@mui/icons-material/ArrowForward'
import VisibilityIcon from '@mui/icons-material/Visibility'
import TerminalWindow from '../widgets/TerminalWindow'
import ClickLink from '../widgets/ClickLink'
import EditTextFineTunedQuestions from './EditTextFineTunedQuestions'
import ForkFineTunedInteration from './ForkFineTunedInteraction'
import LiveInteraction from './LiveInteraction'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
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
  getTextDataPrepStageIndex,
  getTextDataPrepErrors,
  getTextDataPrepStats,
} from '../../utils/session'

const GeneratedImage = styled('img')({})

export const Interaction: FC<{
  session_id: string,
  session_name: string,
  type: ISessionType,
  mode: ISessionMode,
  interaction: IInteraction,
  serverConfig: IServerConfig,
  error?: string,
  isLast?: boolean,
  retryFinetuneErrors?: {
    (): void
  },
  onMessageChange?: {
    (message: string): void,
  },
  onClone?: {
    (interactionID: string): void,
  },
}> = ({
  session_id,
  session_name,
  type,
  mode,
  interaction,
  serverConfig,
  error = '',
  isLast = false,
  retryFinetuneErrors,
  onMessageChange,
  onClone,
}) => {
  const [ viewingError, setViewingError ] = useState(false)
  const theme = useTheme()
  let displayMessage: string = ''
  let imageURLs: string[] = []
  let isLoading = isLast && interaction.creator == SESSION_CREATOR_SYSTEM && !interaction.finished
  
  const isImageFinetune = interaction.creator == SESSION_CREATOR_USER && type == SESSION_TYPE_IMAGE
  const isTextFinetune = interaction.creator == SESSION_CREATOR_USER && type == SESSION_TYPE_TEXT
  const dataPrepStage = interaction.data_prep_stage

  const isEditingConversations = interaction.state == INTERACTION_STATE_EDITING ? true : false
  const hasFineTuned = interaction.lora_dir ? true : false
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
      if(!interaction.lora_dir) {
        displayMessage = interaction.message
      }
    } else if(type == SESSION_TYPE_IMAGE) {
      if(interaction.creator == SESSION_CREATOR_USER) {
        displayMessage = interaction.message
      }
      else {
        if(mode == SESSION_MODE_INFERENCE && interaction.files && interaction.files.length > 0) {
          imageURLs = interaction.files.filter(isImage)
        }
      }
    }
  }

  const useSystemName = session_name || 'System'
  const useName = interaction.creator == SESSION_CREATOR_SYSTEM ? useSystemName : interaction.creator

  const dataPrepErrors = useMemo(() => {
    return getTextDataPrepErrors(interaction)
  }, [
    interaction,
  ])

  const dataPrepStats = useMemo(() => {
    return getTextDataPrepStats(interaction)
  }, [
    interaction,
  ])

  if(!serverConfig || !serverConfig.filestore_prefix) return null

  return (
    <Box key={interaction.id} sx={{ display: 'flex', alignItems: 'flex-start', gap: '0.5rem', mb:2 }}>
      <Avatar sx={{ width: 24, height: 24 }}>{useName.charAt(0).toUpperCase()}</Avatar>
      <Box sx={{ display: 'flex', flexDirection: 'column', width: '100%' }}>
        <Row>
          <Cell flexGrow={1}>
            <Typography
              variant="subtitle2"
              sx={{
                fontWeight: 'bold'
              }}
            >
              { useName.charAt(0).toUpperCase() + useName.slice(1) }
            </Typography>
          </Cell>
          {/* {
            interaction.creator == SESSION_CREATOR_SYSTEM && onClone && (
              <Cell>
                <Link
                  href={`/copy/${session_id}/${interaction.id}`}
                  onClick={(e) => {
                    e.preventDefault()
                    onClone(interaction.id)
                  }}
                >
                  <Typography
                    sx={{
                      fontSize: "small",
                      flexGrow: 0,
                      textDecoration: 'underline',
                    }}
                  >
                    Clone
                  </Typography>
                </Link>
              </Cell>
            )
          } */}
        </Row>
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
                              overflow: "hidden"
                            }}
                            onClick={ () => {
                              window.open(useURL)
                            }}
                          >
                            <span className={`fiv-viv fiv-size-md fiv-icon-${mapFileExtension(filename)}`}></span>
                            <Typography variant="caption" sx={{
                              textAlign: 'center',
                              color: theme.palette.mode == "light" ? 'blue' : 'lightblue',
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
          type == SESSION_TYPE_TEXT && dataPrepStage != TEXT_DATA_PREP_STAGE_NONE && (
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
                <Step>
                  <StepLabel>Fine Tune</StepLabel>
                </Step>
              </Stepper>
            </Box>
          )
        }
        {
          isEditingConversations && session_id && dataPrepErrors.length == 0 && (
            <Box
              sx={{
                mt: 2,
              }}
            >
              <EditTextFineTunedQuestions
                sessionID={ session_id }
                interactionID={ interaction.id }
              />
            </Box>
          )
        }
        {
          hasFineTuned && (
            <ForkFineTunedInteration

            />
          )
        }
        {
          isLoading && (
            <LiveInteraction
              session_id={ session_id }
              interaction={ interaction }
              onMessageChange={ onMessageChange }
            />
          )
        }
        {
          displayMessage && (
            <Typography className="interactionMessage">{ displayMessage }</Typography>
          )
        }
        {
          useErrorText && (
            <Alert severity="error">
              The system has encountered an error - 
              <ClickLink onClick={ () => {
                setViewingError(true)
              }}>
                click here
              </ClickLink>
              to view the details.
            </Alert>
          ) 
        }
        {
          isEditingConversations && session_id && dataPrepErrors.length > 0 && (
            <Box
              sx={{
                mt: 2,
              }}
            >
              <Alert
                severity="success"
                sx={{
                  mb: 2,
                }}
              >
                From <strong>{ dataPrepStats.total_files }</strong> file{ dataPrepStats.total_files == 1 ? '' : 's' } we created <strong>{ dataPrepStats.total_chunks }</strong> text chunk{ dataPrepStats.total_chunks == 1 ? '' : 's' } and converted <strong>{ dataPrepStats.converted }</strong> of those into <strong>{ dataPrepStats.total_questions }</strong> questions.
              </Alert>
              <Alert
                severity="error"
                sx={{
                  mb: 2,
                }}
              >
                However, we encountered <strong>{ dataPrepStats.errors }</strong> error{ dataPrepStats.errors == 1 ? '' : 's' }, please choose how you want to proceed:
              </Alert>
              <Row>
                {
                  retryFinetuneErrors && (
                    <Button
                      variant="contained"
                      color="primary"
                      sx={{
                        mr: 1,
                      }}
                      endIcon={<ReplayIcon />}
                      onClick={ retryFinetuneErrors }
                    >
                      Retry
                    </Button>
                  )
                }
                
                <Button
                  variant="contained"
                  color="primary"
                  sx={{
                    mr: 1,
                  }}
                  endIcon={<VisibilityIcon />}
                  onClick={ () => {
                    alert('coming soon')
                  }}
                >
                  View Errors
                </Button>
                <Button
                  variant="contained"
                  color="primary"
                  sx={{
                    mr: 1,
                  }}
                  endIcon={<ArrowForwardIcon />}
                  onClick={ () => {
                    window.location.href = `/session/${session_id}/edit`
                  }}
                >
                  Ignore Errors
                </Button>
              </Row>
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