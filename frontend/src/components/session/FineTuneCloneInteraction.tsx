import React, { FC, useState, useEffect } from 'react'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import CardActions from '@mui/material/CardActions'

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
  ICloneTextMode,
  CLONE_TEXT_TYPE_JUST_DATA,
  CLONE_TEXT_TYPE_WITH_QUESTIONS,
  CLONE_TEXT_TYPE_ALL,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../../types'

export const FineTuneCloneInteraction: FC<{
  type: ISessionType,
  sessionID: string,
  interactionID: string,
  onClone: (mode: ICloneTextMode) => void,
}> = ({
  type,
  sessionID,
  interactionID,
  onClone,
}) => {
  const interactionQuestions = useInteractionQuestions()
  const [ viewMode, setViewMode ] = useState(false)
  const [ cloneMode, setCloneMode ] = useState(false)

  const colSize = type == SESSION_TYPE_IMAGE ? 6 : 4
  
  useEffect(() => {
    if(!viewMode) {
      interactionQuestions.setQuestions([])
      return
    }
    interactionQuestions.loadQuestions(sessionID, interactionID)
  }, [
    viewMode,
    sessionID,
    interactionID,
  ])

  return (
    <>
      <Grid container spacing={ 0 }>
        <Grid item sm={ 12 } md={ 6 } sx={{pr:2}}>
          <Typography gutterBottom>
            You have completed a fine tuning session on these { type == SESSION_TYPE_IMAGE ? 'images' : 'documents' }.
          </Typography>
          <Typography gutterBottom>
            You can now chat to your model or you can "Clone" from this point in time to create a new session and continue training from here.
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
                  mr: 1,
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
              mr: 1,
              mb: 1,
            }}
            endIcon={<FileCopyIcon />}
            onClick={ () => setCloneMode(true) }
          >
            Clone From Here
          </Button>
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
                        <Button size="small" variant="contained" onClick={() => onClone(CLONE_TEXT_TYPE_JUST_DATA)}>Clone</Button>
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
                            <Button size="small" variant="contained" onClick={() => onClone(CLONE_TEXT_TYPE_WITH_QUESTIONS)}>Clone</Button>
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
                        <Button size="small" variant="contained" onClick={() => onClone(CLONE_TEXT_TYPE_ALL)}>Clone</Button>
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