import React, { FC, useState, useCallback, useEffect } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Grid from '@mui/material/Grid'
import TextField from '@mui/material/TextField'

import ArrowCircleRightIcon from '@mui/icons-material/ArrowCircleRight'

import useAccount from '../../hooks/useAccount'
import Interaction from './Interaction'
import ImageFineTuneLabel from './ImageFineTuneLabel'

import {
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
} from '../../types'

import {
  getSystemMessage,
} from '../../utils/session'

export const ImageFineTuneLabels: FC<{
  showImageLabelErrors: boolean,
  initialLabels?: Record<string, string>,
  files: File[],
  onChange: {
    (labels: Record<string, string>): void
  },
  onDone: {
    (): void
  },
}> = ({
  showImageLabelErrors,
  initialLabels,
  files,
  onChange,
  onDone,
}) => {
  const account = useAccount()

  const [labels, setLabels] = useState<Record<string, string>>(initialLabels || {})

  return (
    <Box
      sx={{
        mt: 2,
      }}
    >
      <Box
        sx={{
          mt: 4,
          mb: 4,
        }}
      >
        <Interaction
          session_id=""
          session_name=""
          interaction={ getSystemMessage('Now, add a label to each of your images.  Try to add as much detail as possible to each image:') }
          type={ SESSION_TYPE_TEXT }
          mode={ SESSION_MODE_INFERENCE }
          serverConfig={ account.serverConfig }
        />
      </Box>
    
      <Grid container spacing={3} direction="row" justifyContent="flex-start">
        {
          files.length > 0 && files.map((file) => {
            const objectURL = URL.createObjectURL(file)
            return (
              <Grid item xs={4} md={4} key={file.name}>
                <Box
                  sx={{
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    color: '#999'
                  }}
                >
                  <Box
                    component="img"
                    src={objectURL}
                    alt={file.name}
                    sx={{
                      height: '100px',
                      border: '1px solid #000000',
                      filter: 'drop-shadow(3px 3px 5px rgba(0, 0, 0, 0.2))',
                      mb: 1,
                    }}
                  />
                  <ImageFineTuneLabel
                    value={ labels[file.name] || '' }
                    filename={ file.name }
                    error={ showImageLabelErrors && !labels[file.name] }
                    onChange={ (value) => {
                      const newLabels = {...labels}
                      newLabels[file.name] = value
                      setLabels(newLabels)
                    }}
                  />
                </Box>
              </Grid>
            )
          })
            
        }
      </Grid>
      {
        files.length > 0 && (
          <Button
            sx={{
              width: '100%',
              mt: 4,
            }}
            variant="contained"
            color="secondary"
            endIcon={<ArrowCircleRightIcon />}
            onClick={ onDone }
          >
            Start Training
          </Button>
        )
      }
    </Box>
  )   
}

export default ImageFineTuneLabels