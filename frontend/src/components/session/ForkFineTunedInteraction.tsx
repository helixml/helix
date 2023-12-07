import React, { FC, useState, useEffect } from 'react'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import FileCopyIcon from '@mui/icons-material/FileCopy'
import ViewIcon from '@mui/icons-material/Visibility'

import Window from '../widgets/Window'
import ConversationEditor from './ConversationEditor'

import useInteractionQuestions from '../../hooks/useInteractionQuestions'

export const ForkFineTunedInteraction: FC<{
  sessionID: string,
  interactionID: string,
}> = ({
  sessionID,
  interactionID,
}) => {
  const interactionQuestions = useInteractionQuestions()
  const [ viewMode, setViewMode ] = useState(false)
  
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
            You have completed a fine tuning session on these documents.
          </Typography>
          <Typography gutterBottom>
            You can "Clone" from this point in time to create a new session and continue training from here.
          </Typography>
        </Grid>
        <Grid item sm={ 12 } md={ 6 } sx={{
          textAlign: 'right',
          pt: 2,
        }}>
          <Button
            variant="contained"
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
          <Button
            variant="contained"
            color="secondary"
            size="small"
            sx={{
              mr: 1,
              mb: 1,
            }}
            endIcon={<FileCopyIcon />}
            onClick={ () => {} }
          >
            Clone From Here
          </Button>
        </Grid>

      </Grid>

      {
        viewMode && (
          <Window
            title="View Questions"
            size="lg"
            fullHeight
            open
            withCancel
            cancelTitle="Close"
            onCancel={ () => setViewMode(false) }
          >
            {
              interactionQuestions.questions.length > 0 && (
                <ConversationEditor
                  readOnly
                  initialQuestions={ interactionQuestions.questions }
                  onCancel={ () => setViewMode(false) }
                />
              )
            }
            
          </Window>
        )
      }
    </>
  )  
}

export default ForkFineTunedInteraction