import React, { FC, useState } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'

import NavigateNextIcon from '@mui/icons-material/NavigateNext'
import AddIcon from '@mui/icons-material/Add'

import AddFilesWindow from './AddFilesWindow'

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
  const [ addFilesMode, setAddFilesMode ] = useState(false)
  
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
            onClick={ () => {} }
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