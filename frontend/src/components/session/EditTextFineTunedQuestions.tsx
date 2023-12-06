import React, { FC, useState, useEffect, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'

import NavigateNextIcon from '@mui/icons-material/NavigateNext'
import EditIcon from '@mui/icons-material/Edit'

import useApi from '../../hooks/useApi'
import Window from '../widgets/Window'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import ConversationEditor from './ConversationEditor'

import useSnackbar from '../../hooks/useSnackbar'
import useInteractionQuestions from '../../hooks/useInteractionQuestions'

export const EditTextFineTunedQuestions: FC<{
  sessionID: string,
  interactionID: string,
}> = ({
  sessionID,
  interactionID,
}) => {
  const snackbar = useSnackbar()
  const api = useApi()
  const interactionQuestions = useInteractionQuestions()
  const [ editMode, setEditMode ] = useState(false)
  
  const startFinetuning = useCallback(async () => {
    await api.post(`/api/v1/sessions/${sessionID}/finetune/start`, undefined, {}, {
      loading: true,
    })
    snackbar.success('Fine tuning started')
  }, [
    sessionID,
  ])

  useEffect(() => {
    interactionQuestions.loadQuestions(sessionID, interactionID)
  }, [
    sessionID,
    interactionID,
  ])

  return (
    <Box>
      <Row>
        <Cell grow>
          <Typography gutterBottom>
            Your documents have been turned into question answer pairs ready for fine tuning.
          </Typography>
          {
            editMode ? (
              <Typography gutterBottom>
                Please edit the questions and answers below and click the <strong>Save</strong> button to continue.
              </Typography>
            ) : (
              <Typography gutterBottom>
                You can start training now or edit the questions and answers.
              </Typography>
            )
          }
        </Cell>
        <Cell>
          <Button
            variant="contained"
            color="primary"
            sx={{
              mr: 2,
            }}
            endIcon={<EditIcon />}
            onClick={ () => setEditMode(true) }
          >
            Edit Questions
          </Button>
          <Button
            variant="contained"
            color="secondary"
            endIcon={<NavigateNextIcon />}
            onClick={ startFinetuning }
          >
            Start Training
          </Button>
        </Cell>

      </Row>
          
      {
        editMode && (
          <Window
            title="Edit Questions"
            size="lg"
            fullHeight
            open
            withCancel
            cancelTitle="Close"
            onCancel={ () => setEditMode(false) }
          >
            <ConversationEditor
              initialQuestions={ interactionQuestions.questions }
              onSubmit={ async (questions) => {
                const saved = await interactionQuestions.saveQuestions(sessionID, interactionID, questions)
                if(saved) {
                  setEditMode(false)
                }
              }}
              onCancel={ () => setEditMode(false) }
            />
          </Window>
        )
      }
    
    </Box>
  )  
}

export default EditTextFineTunedQuestions