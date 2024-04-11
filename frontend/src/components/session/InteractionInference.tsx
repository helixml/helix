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
import Markdown from './Markdown'

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'

import {
  emitEvent,
} from '../../utils/analytics'

import {
  ISession,
  IServerConfig,
} from '../../types'

import {
  replaceMessageText,
} from '../../utils/session'

const GeneratedImage = styled('img')({})

export const InteractionInference: FC<{
  imageURLs?: string[],
  message?: string,
  error?: string,
  serverConfig?: IServerConfig,
  session: ISession,
  // if the session is shared then we don't enforce needing an access token to see the files
  isShared?: boolean,
  onRestart?: () => void,
  upgrade?: boolean,
  isFromSystem?: boolean,
}> = ({
  imageURLs = [],
  message,
  error,
  serverConfig,
  session,
  isShared,
  onRestart,
  upgrade,
  isFromSystem,
}) => {
  const account = useAccount()
  const router = useRouter()
  const [ viewingError, setViewingError ] = useState(false)
  if(!serverConfig || !serverConfig.filestore_prefix) return null

  const getFileURL = (url: string) => {
    return `${serverConfig.filestore_prefix}/${url}?access_token=${account.token}&redirect_urls=true`
  }

  const sourceText = replaceMessageText(message || '', session, getFileURL)

  return (
    <>
      {
        message && (
          <Markdown
            text={ sourceText }
          />
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
              !upgrade && onRestart && (
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
            {
              upgrade && (
                <Cell
                  sx={{
                    ml: 2,
                  }}
                >
                  <Button                    
                    variant="contained"
                    color="secondary"
                    size="small"                    
                    onClick={() => {
                      emitEvent({
                        name: 'queue_upgrade_clicked'
                      })
                      router.navigate('account')
                    }}
                  >
                    Upgrade
                  </Button>
                </Cell>
              )
            }
          </Row>
        ) 
      }
      {
        serverConfig?.filestore_prefix && imageURLs
          .filter(file => {
            return isShared || account.token ? true : false
          })
          .map((imageURL: string) => {
            const useURL = getFileURL(imageURL)
            return (
              <Box
                sx={{
                  mt: 2,
                  maxWidth: '600px',
                }}
                key={ useURL }
              >
                <Link
                  href={ useURL }
                  target="_blank"
                >
                  <GeneratedImage
                    sx={{
                      maxHeight: '600px',
                      width: '100%',
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

export default InteractionInference