import React, { FC, useState, useEffect, useRef, useCallback } from 'react'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import TextField from '@mui/material/TextField'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'

import SendIcon from '@mui/icons-material/Send'
import Interaction from '../components/session/Interaction'
import InteractionLiveStream from '../components/session/InteractionLiveStream'

import Disclaimer from '../components/widgets/Disclaimer'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'
import useRouter from '../hooks/useRouter'
import useAccount from '../hooks/useAccount'
import useLoading from '../hooks/useLoading'
import { useTheme } from '@mui/material/styles'
import useThemeConfig from '../hooks/useThemeConfig'
import Tooltip from '@mui/material/Tooltip'
import IconButton from '@mui/material/IconButton'
import RefreshIcon from '@mui/icons-material/Refresh'

import {
  ICloneInteractionMode,
  INTERACTION_STATE_EDITING,
  SESSION_TYPE_TEXT,
  SESSION_MODE_FINETUNE,
} from '../types'

const DemoPage: FC = () => {
  const snackbar = useSnackbar()
  const api = useApi()
  const router = useRouter()
  const account = useAccount()
  const loadingHelpers = useLoading()
  const theme = useTheme()
  const themeConfig = useThemeConfig()

  const textFieldRef = useRef<HTMLTextAreaElement>()
  const divRef = useRef<HTMLDivElement>()

  const [inputValue, setInputValue] = useState('')

  const handleInputChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setInputValue(event.target.value)
  }

  const onSend = useCallback(async (prompt: string) => {
    console.log(`Sending prompt: ${prompt}`)
    setInputValue("")
  }, [])

  const scrollToBottom = useCallback(() => {
    const divElement = divRef.current
    if(!divElement) return
    divElement.scrollTo({
      top: divElement.scrollHeight - divElement.clientHeight,
      behavior: "smooth"
    })
  }, [])

  useEffect(() => {
    textFieldRef.current?.focus()
  }, [])

  return (    
    <Box
      sx={{
        width: '100%',
        height: '100%',
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
      }}
    >
      <Box
        id="demo-scroller"
        ref={ divRef }
        sx={{
          width: '100%',
          flexGrow: 1,
          overflowY: 'auto',
          mt: 10,
          p: 2,
        }}
      >
        <Container maxWidth="lg">
          <Typography variant="h5" gutterBottom>
            Demo Interaction Area
          </Typography>
          <Typography>
            This is a simulated interaction area for demonstration purposes.
          </Typography>
        </Container>
      </Box>
      <Box
        sx={{
          width: '100%',
          flexGrow: 0,
          p: 2,
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        
      </Box>
      <Box
        sx={{
          width: '100%',
          flexGrow: 0,
          p: 2,
          display: 'flex',
          flexDirection: 'row',
          alignItems: 'center',
          justifyContent: 'center',
        }}
      >
        <Container
          maxWidth="lg"
        >
          <Row>
            <Cell flexGrow={1}>
              <TextField
                id="textEntry"
                fullWidth
                inputRef={textFieldRef}
                label="Enter your demo input here..."
                value={inputValue}
                onChange={handleInputChange}
                name="demo_input"
                multiline={true}
              />
            </Cell>
            <Cell>
              <Button
                variant='contained'
                onClick={ () => onSend(inputValue) }
                sx={{
                  ml: 2,
                }}
                endIcon={<SendIcon />}
              >
                Send
              </Button>
            </Cell>
          </Row>
          <Box
            sx={{
              mt: 2,
              mb: {
                xs: 8,
                sm: 8,
                md: 8,
                lg: 4,
                xl: 4,
              }
            }}
          >
            <Disclaimer />
          </Box>
          
        </Container>
        
      </Box>
    </Box>
  )
}

export default DemoPage
