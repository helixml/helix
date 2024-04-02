import React, { FC, useState, useCallback } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'

import NavigateNextIcon from '@mui/icons-material/NavigateNext'
import AddIcon from '@mui/icons-material/Add'

import AddFilesWindow from './AddFilesWindow'

import useFinetuneInputs from '../../hooks/useFinetuneInputs'
import useApi from '../../hooks/useApi'

import {
  ISession,
} from '../../types'

export const FineTuneAddFiles: FC<{
  session: ISession,
  interactionID?: string,
  onReloadSession: () => void,
}> = ({
  session,
  interactionID,
  onReloadSession,
}) => {
  const api = useApi()
  const inputs = useFinetuneInputs()
  const [ addFilesMode, setAddFilesMode ] = useState(false)

  // this is for text finetune
  const onStartDataPrep = async () => {
    inputs.setUploadProgress({
      percent: 0,
      totalBytes: 0,
      uploadedBytes: 0,
    })
    try {
      let formData = new FormData()
      formData = inputs.setFormData(formData)
      await api.put(`/api/v1/sessions/${session.id}/finetune/documents`, formData, {
        onUploadProgress: inputs.uploadProgressHandler,
        params: {
          interactionID: interactionID || '',
        }
      })
      inputs.setUploadProgress(undefined)
    } catch(e: any) {}

    inputs.setUploadProgress(undefined)
  }

  return (
    <>
      <Grid container spacing={ 0 }>
        <Grid item sm={ 12 } md={ 6 } sx={{pr:2}}>
          <Typography gutterBottom>
            You can add files to this stage or begin the data prep right away.
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
              mr: 2,
            }}
            endIcon={<AddIcon />}
            onClick={ () => setAddFilesMode(true) }
          >
            Add Files
          </Button>
          <Button
            variant="contained"
            color="secondary"
            size="small"
            endIcon={<NavigateNextIcon />}
            onClick={ onStartDataPrep }
          >
            Start Data Prep
          </Button>
        </Grid>

      </Grid>
      {
        addFilesMode && (
          <AddFilesWindow
            session={ session }
            interactionID={ interactionID }
            onClose={ (filesAdded) => {
              setAddFilesMode(false)
              if(filesAdded) {
                onReloadSession()
              }
            } }
          />
        )
      }
      
    </>
  )  
}

export default FineTuneAddFiles