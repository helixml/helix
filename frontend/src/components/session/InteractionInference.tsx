import React, { FC, useState } from 'react'
import { styled } from '@mui/system'
import Typography from '@mui/material/Typography'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'
import ReplayIcon from '@mui/icons-material/Replay'

import TerminalWindow from '../widgets/TerminalWindow'
import ClickLink from '../widgets/ClickLink'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'


import {
  IServerConfig,
} from '../../types'

const GeneratedImage = styled('img')({})

export const InteractionMessage: FC<{
  imageURLs?: string[],
  message?: string,
  error?: string,
  serverConfig?: IServerConfig,
  onRestart?: () => void,
}> = ({
  imageURLs = [],
  message,
  error,
  serverConfig,
  onRestart,
}) => {
  const [ viewingError, setViewingError ] = useState(false)
  if(!serverConfig || !serverConfig.filestore_prefix) return null

  return (
    <>
      {
        message && (
          <Typography className="interactionMessage" dangerouslySetInnerHTML={{__html: message.trim().replace(/</g, '&lt;').replace(/\n/g, '<br/>')}}></Typography>
        )
      }
      {
        error && (
          <Row>
            <Cell grow>
              <Alert severity="error">
                The system has encountered an error -
                <ClickLink
                  sx={{
                    pl: 0.5,
                    pr: 0.5,
                  }}
                  onClick={ () => {
                    setViewingError(true)
                  }}
                >
                  click here
                </ClickLink>
                to view the details.
              </Alert>
            </Cell>
            {
              onRestart && (
                <Cell
                  sx={{
                    ml: 2,
                  }}
                >
                  <Button                    
                    variant="contained"
                    color="secondary"
                    size="small"
                    endIcon={<ReplayIcon />}
                    onClick={ onRestart }
                  >
                    Retry
                  </Button>
                </Cell>
              )
            }
          </Row>
        ) 
      }
      {
        serverConfig?.filestore_prefix && imageURLs.map((imageURL: string) => {
          const useURL = `${serverConfig.filestore_prefix}/${imageURL}`
          return (
            <Box
              sx={{
                mt: 2,
              }}
              key={ useURL }
            >
              <Link
                href={ useURL }
                target="_blank"
              >
                <GeneratedImage
                  sx={{
                    height: '600px',
                    maxHeight: '600px',
                    border: '1px solid #000000',
                    filter: 'drop-shadow(5px 5px 10px rgba(0, 0, 0, 0.5))',
                  }}
                  src={ useURL }
                />  
              </Link>
            </Box>
          )
          
        })
      }
      {
        viewingError && (
          <TerminalWindow
            open
            title="Error"
            data={ error }
            onClose={ () => {
              setViewingError(false)
            }}
          /> 
        )
      }
    </>
  )   
}

export default InteractionMessage