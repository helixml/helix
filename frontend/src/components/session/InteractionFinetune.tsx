import React, { FC, useMemo, useCallback } from 'react'
import { useTheme } from '@mui/system'
import useMediaQuery from '@mui/material/useMediaQuery' 
import Typography from '@mui/material/Typography'
import Alert from '@mui/material/Alert'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Stepper from '@mui/material/Stepper'
import Step from '@mui/material/Step'
import StepLabel from '@mui/material/StepLabel'
import ReplayIcon from '@mui/icons-material/Replay'
import ArrowForwardIcon from '@mui/icons-material/ArrowForward'
import VisibilityIcon from '@mui/icons-material/Visibility'
import FineTuneTextQuestions from './FineTuneTextQuestions'
import FineTuneAddFiles from './FineTuneAddFiles'
import FineTuneCloneInteraction from './FineTuneCloneInteraction'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import AttachFileIcon from '@mui/icons-material/AttachFile';

import {
  ICloneInteractionMode,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_CREATOR_USER,
  INTERACTION_STATE_EDITING,
  TEXT_DATA_PREP_STAGE_NONE,
  TEXT_DATA_PREP_STAGE_EDIT_FILES,
  TEXT_DATA_PREP_STAGE_EDIT_QUESTIONS,
  SESSION_CREATOR_ASSISTANT,
} from '../../types'

import {
  ISession,
  IInteraction,
  IServerConfig,
} from '../../types'

import {
  mapFileExtension,
} from '../../utils/filestore'

import {
  getTextDataPrepStageIndex,
  getTextDataPrepStageIndexDisplay,
  getTextDataPrepErrors,
  getTextDataPrepStats,
} from '../../utils/session'

export const InteractionFinetune: FC<{
  serverConfig: IServerConfig,
  interaction: IInteraction,
  session: ISession,
  highlightAllFiles?: boolean,
  retryFinetuneErrors?: () => void,
  onReloadSession?: () => void,
  onClone?: (mode: ICloneInteractionMode, interactionID: string) => Promise<boolean>,
  onAddDocuments?: () => void,
}> = ({
  serverConfig,
  interaction,
  session,
  highlightAllFiles = false,
  retryFinetuneErrors,
  onReloadSession,
  onClone,
  onAddDocuments,
}) => {
  const theme = useTheme()
  const account = useAccount()
  const api = useApi()
  const snackbar = useSnackbar()

  const isAssistantInteraction = interaction.creator == SESSION_CREATOR_ASSISTANT
  const isUserInteraction = interaction.creator == SESSION_CREATOR_USER
  const isImageFinetune = isUserInteraction && session.type == SESSION_TYPE_IMAGE
  const isTextFinetune = isUserInteraction && session.type == SESSION_TYPE_TEXT
  const isEditingConversations = interaction.state == INTERACTION_STATE_EDITING && interaction.data_prep_stage == TEXT_DATA_PREP_STAGE_EDIT_QUESTIONS ? true : false
  const isAddingFiles = interaction.state == INTERACTION_STATE_EDITING && interaction.data_prep_stage == TEXT_DATA_PREP_STAGE_EDIT_FILES ? true : false
  const hasFineTuned = interaction.lora_dir ? true : false

  const dataPrepErrors = useMemo(() => {
    return getTextDataPrepErrors(interaction)
  }, [
    interaction,
  ])

  // in the case where we are a assistant interaction that is showing buttons
  // to edit the dataset in the previous user interaction
  // we need to know what that previous user interaction was
  const userFilesInteractionID = useMemo(() => {
    const currentInteractionIndex = session.interactions.findIndex((i) => i.id == interaction.id)
    if(currentInteractionIndex == 0) return ''
    const previousInteraction = session.interactions[currentInteractionIndex - 1]
    return previousInteraction.id
  }, [
    session,
    interaction,
  ])

  const isShared = useMemo(() => {
    return session.config.shared ? true : false
  }, [
    session,
  ])

  const dataPrepStats = useMemo(() => {
    return getTextDataPrepStats(interaction)
  }, [
    interaction,
  ])

  const startFinetuning = useCallback(async () => {
    await api.post(`/api/v1/sessions/${session.id}/finetune/start`, undefined, {}, {
      loading: true,
    })
    snackbar.success('Fine tuning started')
  }, [
    session,
  ])

  if(!serverConfig || !serverConfig.filestore_prefix || (!isShared && !account.token)) return null

  const matches = useMediaQuery(theme.breakpoints.down('md'))

  return (
    <>
      {
        isImageFinetune && interaction.files && interaction.files.length > 0 && (
          <Box
            sx={{
              maxHeight: '400px',
              overflowY: 'auto',
            }}
          >
            <Grid container spacing={2} direction="row" justifyContent="flex-start">
              {
                interaction.files.length > 0 && interaction.files
                  .filter(file => {
                    if(!isShared && !account.token) return false
                    return file.match(/\.txt$/i) ? false : true
                  })
                  .map((file) => {
                    const useURL = `${serverConfig.filestore_prefix}/${file}?access_token=${account.tokenUrlEscaped}`
                    const filenameParts = file.split('/')
                    const label = interaction.metadata[filenameParts[filenameParts.length - 1]] || ''

                    return (
                      <Grid item xs={4} md={3} key={file}>
                        <Box
                          sx={{
                            display: 'flex',
                            flexDirection: 'column',
                            alignItems: 'center',
                            justifyContent: 'center',
                            color: '#999',
                            p: 0,
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
      {/* {
        isTextFinetune && interaction.files && interaction.files.length > 0 && (
          <Box
            sx={{
              maxHeight: '400px',
              overflowY: 'auto',
              padding: 2,
              backgroundColor: highlightAllFiles ? 'rgba(255,255,255,0.5)' : 'transparent',
              transition: 'all 0.3s ease',
            }}
          >
            <Grid container spacing={3} direction="row" justifyContent="flex-start">
              {
                interaction.files.length > 0 && interaction.files
                  .filter(file => {
                    if(!isShared && !account.token) return false
                    return true
                  })
                  .map((file) => {
                    const useURL = `${serverConfig.filestore_prefix}/${file}?access_token=${account.tokenUrlEscaped}`
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
                        { <AttachFileIcon /> }
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
        session.type == SESSION_TYPE_TEXT && interaction.data_prep_stage != TEXT_DATA_PREP_STAGE_NONE && getTextDataPrepStageIndexDisplay(interaction.data_prep_stage) > 0 && (
          <Box
            sx={{
              mt: 1.5,
              mb: 3,
            }}
          >
            <Stepper activeStep={getTextDataPrepStageIndexDisplay(interaction.data_prep_stage)} orientation={matches ? "vertical" : "horizontal"}>
              <Step>
                <StepLabel>Extract Text</StepLabel>
              </Step>
              <Step>
                <StepLabel>Index Documents</StepLabel>
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
      } */}
      {
        isEditingConversations && dataPrepErrors.length == 0 && (
          <Box
            sx={{
              mt: 2,
            }}
          >
            <FineTuneTextQuestions
              sessionID={ session.id }
              interactionID={ userFilesInteractionID }
            />
          </Box>
        )
      }
      {
        isAddingFiles && onReloadSession && (
          <Box
            sx={{
              mt: 2,
            }}
          >
            <FineTuneAddFiles
              session={ session }
              interactionID={ userFilesInteractionID }
              onReloadSession={ onReloadSession }
            />
          </Box>
        )
      }
      {
        isAssistantInteraction && hasFineTuned && onClone && (
          <FineTuneCloneInteraction
            type={ session.type }
            sessionID={ session.id }
            assistantInteractionID={ interaction.id }
            userInteractionID={ userFilesInteractionID }
            onClone={ onClone }
            onAddDocuments={ onAddDocuments }
          />
        )
      }
      {
        isEditingConversations && session.id && dataPrepErrors.length > 0 && (
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
            <Row
              sx={{
                flexDirection: {
                  xs: 'column',
                  sm: 'column',
                  md: 'row'
                }
              }}
            >
              {
                retryFinetuneErrors && (
                  <Cell
                    sx={{
                      width: '100%',
                      m: 1,
                    }}
                  >
                    <Button
                      variant="contained"
                      color="primary"
                      sx={{
                        width: '100%'
                      }}
                      endIcon={<ReplayIcon />}
                      onClick={ retryFinetuneErrors }
                    >
                      Retry
                    </Button>
                  </Cell>
                )
              }
              <Cell
                sx={{
                  width: '100%',
                  m: 1,
                }}
              >
                <FineTuneTextQuestions
                  onlyShowEditMode
                  sessionID={ session.id }
                  interactionID={ userFilesInteractionID }  
                />
              </Cell>
              <Cell
                sx={{
                  width: '100%',
                  m: 1,
                }}
              >
                <Button
                  variant="contained"
                  color="primary"
                  sx={{
                    width: '100%'
                  }}
                  endIcon={<ArrowForwardIcon />}
                  onClick={ () => {
                    startFinetuning()
                  }}
                >
                  Ignore Errors And Start Fine Tuning
                </Button>
              </Cell>
            </Row>
          </Box>
        )
      }
    </>
  )   
}

export default InteractionFinetune