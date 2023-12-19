import React, { FC, useState, useEffect, useCallback } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'

import NavigateNextIcon from '@mui/icons-material/NavigateNext'
import EditIcon from '@mui/icons-material/Edit'

import useApi from '../../hooks/useApi'
import Window from '../widgets/Window'
import FineTuneTextQuestionEditor from './FineTuneTextQuestionEditor'

import useSnackbar from '../../hooks/useSnackbar'
import useInteractionQuestions from '../../hooks/useInteractionQuestions'

export const FineTuneTextQuestions: FC<{
  sessionID: string,
  interactionID: string,
}> = ({
  sessionID,
  interactionID,
}) => {
  const snackbar = useSnackbar()
  const api = useApi()
  const interactionQuestions = useInteractionQuestions()
  const [ showEditMode, setShowEditMode ] = useState(false)
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
    if(!editMode) {
      interactionQuestions.setQuestions([])
      setShowEditMode(false)
      return
    }
    
    const doAsync = async () => {
      setShowEditMode(false)
      await interactionQuestions.loadQuestions(sessionID, interactionID)
      setShowEditMode(true)
    }
    doAsync()
    
  }, [
    editMode,
    sessionID,
    interactionID,
  ])

  return (
    <>
      <Grid container spacing={ 0 }>
        <Grid item sm={ 12 } md={ 6 } sx={{pr:2}}>
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
            size="small"
            endIcon={<NavigateNextIcon />}
            onClick={ startFinetuning }
          >
            Start Training
          </Button>
        </Grid>

      </Grid>
          
      {
        showEditMode && (
          <FineTuneTextQuestionEditor
            initialQuestions={ interactionQuestions.questions || [] }
            onSubmit={ async (questions) => {
              const saved = await interactionQuestions.saveQuestions(sessionID, interactionID, questions)
              if(saved) {
                setEditMode(false)
              }
            }}
            onCancel={ () => setEditMode(false) }
          />
        )
      }
    
    </>
  )  
}

export default FineTuneTextQuestions