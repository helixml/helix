import React, { FC } from 'react'
import Grid from '@mui/material/Grid'

import AppStoreCard from './AppStoreCard'

import {
  IApp,
} from '../../types'

const AssistantPicker: FC<{
  app: IApp,
  activeAssistant?: number,
  onClick: (index: number) => void,
}> = ({
  app,
  activeAssistant = 0,
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
          return (
            <Grid item xs={ 12 } sm={ 12 } md={ 6 } lg={ 3 } key={ index } sx={{ p: 0, m: 0 }}>
              <AppStoreCard
                avatar={ assistant.avatar }
                image={ assistant.image }
                name={ assistant.name }
                description={ assistant.description }
                clickTitle="Chat"
                disabled={ activeAssistant == index }
                selected={ activeAssistant == index }
                sx={{
                  opacity: activeAssistant == index ? '1' : '0.7',
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
