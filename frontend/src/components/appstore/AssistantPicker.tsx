import React, { FC } from 'react'
import Grid from '@mui/material/Grid'

import AppStoreCard from './AppStoreCard'

import {
  IApp,
} from '../../types'

const AssistantPicker: FC<{
  app: IApp,
  activeAssistantID?: string,
  onClick: (index: number) => void,
}> = ({
  app,
  activeAssistantID = '0',
  onClick,
}) => {
  if(!app.config.helix?.assistants || app.config.helix?.assistants?.length <= 1) {
    return null
  }

  return (
    <Grid
      container
      spacing={ 4 }
      justifyContent="center"
    >
      {
        app.config.helix?.assistants?.map((assistant, index) => {
          const useID = assistant.id || index.toString()
          return (
            <Grid item xs={ 12 } sm={ 12 } md={ 6 } lg={ 3 } key={ index } sx={{ p: 0, m: 0 }}>
              <AppStoreCard
                avatar={ assistant.avatar }
                image={ assistant.image }
                name={ assistant.name || `Assistant ${index + 1}` }
                description={ assistant.description }
                clickTitle="Chat"
                disabled={ activeAssistantID == useID }
                selected={ activeAssistantID == useID }
                sx={{
                  opacity: activeAssistantID == useID ? '1' : '0.7',
                }}
                onClick={ () => onClick(index) }
              />
            </Grid>
          )
        })
      }
    </Grid>
  )
}

export default AssistantPicker
