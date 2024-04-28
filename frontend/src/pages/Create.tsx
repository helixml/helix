import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'

import BackgroundImageWrapper from '../components/widgets/BackgroundImageWrapper'
import Page from '../components/system/Page'

import useRouter from '../hooks/useRouter'
import useSessionConfig from '../hooks/useSessionConfig'

import CreateToolbar from '../components/create/Toolbar'
import ConfigWindow from '../components/create/ConfigWindow'

import {
  ISessionMode,
  ISessionType,
  ICreateSessionConfig,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
} from '../types'

const DEFAULT_SESSION_CONFIG: ICreateSessionConfig = {
  activeToolIDs: [],
  finetuneEnabled: true,
  ragEnabled: false,
  ragDistanceFunction: 'cosine', 
  ragThreshold: 0.2,
  ragResultsCount: 3,
  ragChunkSize: 1024,
  ragChunkOverflow: 20,
}

const Create: FC = () => {
  const router = useRouter()

  const [ sessionConfig, setSessionConfig ] = useState<ICreateSessionConfig>(DEFAULT_SESSION_CONFIG)
  const [ showConfigWindow, setShowConfigWindow ] = useState(false)

  const mode = (router.params.mode as ISessionMode) || SESSION_MODE_INFERENCE
  const type = (router.params.type as ISessionType) || SESSION_TYPE_TEXT

  return (
    <Page
      topbarContent={(
        <CreateToolbar
          mode={ mode }
          onOpenConfig={ () => setShowConfigWindow(true) }
          onSetMode={ mode => router.setParams({mode}) }
        />
      )}
    >
      <BackgroundImageWrapper>
        <Box sx={{m:20}}>hello4</Box>
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
      </BackgroundImageWrapper>
    </Page>
  )
}

export default Create
