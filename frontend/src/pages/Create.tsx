import React, { FC, useState, useEffect } from 'react'

import Page from '../components/system/Page'
import CreateToolbar from '../components/create/Toolbar'
import ConfigWindow from '../components/create/ConfigWindow'

import useRouter from '../hooks/useRouter'
import useLightTheme from '../hooks/useLightTheme'
import useTools from '../hooks/useTools'
import useAccount from '../hooks/useAccount'

import {
  ISessionMode,
  ISessionType,
  ICreateSessionConfig,
  SESSION_MODE_INFERENCE,
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
    >
      {
        showConfigWindow && (
          <ConfigWindow
            mode={ mode }
            type={ type }
            sessionConfig={ sessionConfig }
            setSessionConfig={ setSessionConfig }
            onClose={ () => setShowConfigWindow(false) }
          />
        )
      }
    </Page>
  )
}

export default Create
