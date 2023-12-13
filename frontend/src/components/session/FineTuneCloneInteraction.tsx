import React, { FC, useState, useEffect, useCallback } from 'react'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import CardActions from '@mui/material/CardActions'

import AddIcon from '@mui/icons-material/Add'
import FileCopyIcon from '@mui/icons-material/FileCopy'
import ViewIcon from '@mui/icons-material/Visibility'
import CloneIcon from '@mui/icons-material/ContentCopy'
import DataIcon from '@mui/icons-material/DataUsage'
import QuestionAnswerIcon from '@mui/icons-material/QuestionAnswer'

import Window from '../widgets/Window'
import FineTuneTextQuestionEditor from './FineTuneTextQuestionEditor'

import useInteractionQuestions from '../../hooks/useInteractionQuestions'

import {
  ISessionType,
  ICloneInteractionMode,
  CLONE_INTERACTION_MODE_JUST_DATA,
  CLONE_INTERACTION_MODE_WITH_QUESTIONS,
  CLONE_INTERACTION_MODE_ALL,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../../types'

export const FineTuneCloneInteraction: FC<{
  type: ISessionType,
  sessionID: string,

  // this is used to load the questions
  userInteractionID: string,

  // this is used to target the clone action
  systemInteractionID: string,
  onClone: (mode: ICloneInteractionMode, interactionID: string) => Promise<boolean>,
  onAddDocuments?: () => void,
}> = ({
  type,
  sessionID,
  userInteractionID,
  systemInteractionID,
  onClone,
  onAddDocuments,
}) => {
  const interactionQuestions = useInteractionQuestions()
  const [ viewMode, setViewMode ] = useState(false)
  const [ cloneMode, setCloneMode ] = useState(false)

  const colSize = type == SESSION_TYPE_IMAGE ? 6 : 4

  const handleClone = useCallback(async (mode: ICloneInteractionMode, interactionID: string) => {
    const result = await onClone(mode, interactionID)
    if(!result) return
    setCloneMode(false)
  }, [
    onClone,
  ])
  
  useEffect(() => {
    if(!viewMode) {
      interactionQuestions.setQuestions([])
      return
    }
    interactionQuestions.loadQuestions(sessionID, userInteractionID)
  }, [
    viewMode,
    sessionID,
    userInteractionID,
    systemInteractionID,
  ])

  return (
    <>
      <Grid container spacing={ 0 }>
        <Grid item sm={ 12 } md={ 6 } sx={{pr:2}}>
          <Typography gutterBottom>
            You have completed a fine tuning session on these { type == SESSION_TYPE_IMAGE ? 'images' : 'documents' }.
          </Typography>
          <Typography gutterBottom>
            You can now chat to your model, add some more documents and re-train or you can "Clone" from this point in time.
          </Typography>
        </Grid>
        <Grid item sm={ 12 } md={ 6 } sx={{
          textAlign: 'right',
          pt: 2,
        }}>
          {
            type == SESSION_TYPE_TEXT && (
              <Button
                variant="outlined"
                color="primary"
                size="small"
                sx={{
                  ml: 1,
                  mb: 1,
                }}
                endIcon={<ViewIcon />}
                onClick={ () => setViewMode(true) }
              >
                View Questions
              </Button>
            )
          }
          <Button
            variant="outlined"
            color="secondary"
            size="small"
            sx={{
              ml: 1,
              mb: 1,
            }}
            endIcon={<FileCopyIcon />}
            onClick={ () => setCloneMode(true) }
          >
            Clone
          </Button>

          {
            onAddDocuments && (
              <Button
                variant='outlined'
                size="small"
                sx={{
                  ml: 1,
                  mb: 1,
                }}
                onClick={ onAddDocuments }
                endIcon={<AddIcon />}
              >
                Add { type == SESSION_TYPE_TEXT ? 'Documents' : 'Images' }
              </Button>
            )
          }
          
        </Grid>

      </Grid>

      {
        viewMode && interactionQuestions.loaded && (
          <FineTuneTextQuestionEditor
            title="View Questions"
            cancelTitle="Close"
            readOnly
            initialQuestions={ interactionQuestions.questions }
            onCancel={ () => setViewMode(false) }
          />
        )
      }
      {
        cloneMode && (
          <Window
            title="Clone"
            size="lg"
            open
            withCancel
            onCancel={ () => setCloneMode(false) }
          >
            <Box
              sx={{
                p: 2,
              }}
            >
              <Grid container spacing={ 2 }>
                <Grid item xs={ 12 } md={ colSize }>
                  <Card
                    sx={{
                      height: '100%',
                      display: 'flex',
                      flexDirection: 'column',
                    }}
                  >
                    <CardContent sx={{
                      flexGrow: 1,
                    }}>
                      <QuestionAnswerIcon fontSize="large" />
                      <Typography gutterBottom variant="h5" component="div">
                          Just Data
                      </Typography>
                      <Typography gutterBottom variant="body2" color="text.secondary">
                          Start again with the original data.
                      </Typography>
                      {
                        type == SESSION_TYPE_TEXT ? (
                          <Typography variant="body2" color="text.secondary">
                            Both the trained model and question answer pairs will be removed.
                          </Typography>
                        ) : (
                          <Typography variant="body2" color="text.secondary">
                            The trained model will be removed.
                          </Typography>
                        )
                      }
                    </CardContent>
                    <CardActions
                      sx={{
                        flexGrow: 0,
                        justifyContent: 'flex-end',
                      }}
                    >
                        <Button size="small" variant="contained" onClick={() => {
                          handleClone(CLONE_INTERACTION_MODE_JUST_DATA, systemInteractionID)
                        }}>Clone</Button>
                    </CardActions>
                  </Card>
                </Grid>
                {
                  type == SESSION_TYPE_TEXT && (
                    <Grid item xs={ 12 } md={ colSize }>
                      <Card
                        sx={{
                          height: '100%',
                          display: 'flex',
                          flexDirection: 'column',
                        }}
                      >
                        <CardContent sx={{
                          flexGrow: 1,
                        }}>
                          <DataIcon fontSize="large" />
                          <Typography gutterBottom variant="h5" component="div">
                              With Questions
                          </Typography>
                          <Typography variant="body2" color="text.secondary">
                              The question & answer pairs will be retained but the trained model will be removed.
                          </Typography>
                        </CardContent>
                        <CardActions
                          sx={{
                            justifyContent: 'flex-end',
                          }}
                        >
                            <Button size="small" variant="contained" onClick={() => handleClone(CLONE_INTERACTION_MODE_WITH_QUESTIONS, systemInteractionID)}>Clone</Button>
                        </CardActions>
                      </Card>
                    </Grid>
                  )
                }
                <Grid item xs={ 12 } md={ colSize }>
                  <Card
                    sx={{
                      height: '100%',
                      display: 'flex',
                      flexDirection: 'column',
                    }}
                  >
                    <CardContent sx={{
                      flexGrow: 1,
                    }}>
                      <CloneIcon fontSize="large" />
                      <Typography gutterBottom variant="h5" component="div">
                          With Training
                      </Typography>
                      <Typography variant="body2" color="text.secondary">
                          Clone everything including the trained model.
                      </Typography>
                    </CardContent>
                    <CardActions
                      sx={{
                        justifyContent: 'flex-end',
                      }}
                    >
                        <Button size="small" variant="contained" onClick={() => handleClone(CLONE_INTERACTION_MODE_ALL, systemInteractionID)}>Clone</Button>
                    </CardActions>
                  </Card>
                  
                </Grid>
              </Grid>
            </Box>
          </Window>
        )
      }
      
    </>
  )  
}

export default FineTuneCloneInteraction