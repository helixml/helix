import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'

import Page from '../components/system/Page'
import CreateToolbar from '../components/create/Toolbar'
import ConfigWindow from '../components/create/ConfigWindow'
import SessionTypeTabs from '../components/create/SessionTypeTabs'
import SessionTypeSwitch from '../components/create/SessionTypeSwitch'
import CenterMessage from '../components/create/CenterMessage'
import ExamplePrompts from '../components/create/ExamplePrompts'
import InferenceTextField from '../components/create/InferenceTextField'

import useRouter from '../hooks/useRouter'
import useLightTheme from '../hooks/useLightTheme'
import useTools from '../hooks/useTools'
import useAccount from '../hooks/useAccount'
import useCreateInputs from '../hooks/useCreateInputs'

import {
  ISessionMode,
  ISessionType,
  ICreateSessionConfig,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
} from '../types'

import {
  DEFAULT_SESSION_CONFIG,
  HELIX_DEFAULT_TEXT_MODEL,
} from '../config'

const Create: FC = () => {
  const router = useRouter()
  const lightTheme = useLightTheme()
  const tools = useTools()
  const account = useAccount()
  const inputs = useCreateInputs()

  const [ sessionConfig, setSessionConfig ] = useState<ICreateSessionConfig>(DEFAULT_SESSION_CONFIG)
  const [ showConfigWindow, setShowConfigWindow ] = useState(false)

  const mode = (router.params.mode as ISessionMode) || SESSION_MODE_INFERENCE
  const type = (router.params.type as ISessionType) || SESSION_TYPE_TEXT
  const model = router.params.model || HELIX_DEFAULT_TEXT_MODEL

  useEffect(() => {
    if(!account.user) return
    tools.loadData()
  }, [
    account.user,
  ])

  return (
    <Page
      topbarTitle={ mode == SESSION_MODE_FINETUNE ? 'The start of something beautiful' : '' }
      topbarContent={(
        <CreateToolbar
          mode={ mode }
          type={ type }
          model={ model }
          onOpenConfig={ () => setShowConfigWindow(true) }
          onSetMode={ mode => router.setParams({mode}) }
          onSetModel={ model => router.setParams({model}) }
        />
      )}
      sx={{
        backgroundImage: lightTheme.isLight ? 'url(/img/nebula-light.png)' : 'url(/img/nebula-dark.png)',
        backgroundSize: '80%',
        backgroundPosition: 'center center',
        backgroundRepeat: 'no-repeat',
      }}
      footerContent={(
        <Box
          sx={{
            p: 2,
          }}
        >
          <ExamplePrompts
            type={ type }
            onPrompt={ (prompt) => {
              console.log('--------------------------------------------')
              console.log(prompt)
            }}
          />
          <InferenceTextField
            type={ type }
            value={ inputs.inputValue }
            disabled={ mode == SESSION_MODE_FINETUNE }
            startAdornment={(
              <SessionTypeSwitch
                type={ type }
                onSetType={ type => router.setParams({type}) }
              />
            )}
            onUpdate={ inputs.setInputValue }
            onInference={ () => {
              console.log('--------------------------------------------')
              console.log(inputs.inputValue)
            }}
          />
        </Box>
      )}
    >
      {
        mode == SESSION_MODE_FINETUNE && (
          <>
            <SessionTypeTabs
              type={ type }
              onSetType={ type => router.setParams({type}) }
            />
          </>
        )
      }
      {
        mode == SESSION_MODE_INFERENCE && (
          <>
            <Box
              sx={{
                display: 'flex',
                flexDirection: 'column',
                alignItems: 'center',
                justifyContent: 'center',
                pt: 10,
              }}
            >
              <CenterMessage
                type={ type }
                onSetType={ type => router.setParams({type}) }
              />
            </Box>
          </>
        )
      }
      {
        showConfigWindow && (
          <ConfigWindow
            mode={ mode }
            type={ type }
            sessionConfig={ sessionConfig }
            onSetSessionConfig={ setSessionConfig }
            onClose={ () => setShowConfigWindow(false) }
          />
        )
      }
    </Page>
  )
}

export default Create
