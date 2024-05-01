import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'

import Page from '../components/system/Page'
import Toolbar from '../components/create/Toolbar'
import ConfigWindow from '../components/create/ConfigWindow'
import SessionTypeTabs from '../components/create/SessionTypeTabs'
import SessionTypeSwitch from '../components/create/SessionTypeSwitch'
import CenterMessage from '../components/create/CenterMessage'
import ExamplePrompts from '../components/create/ExamplePrompts'
import InferenceTextField from '../components/create/InferenceTextField'
import Disclaimer from '../components/widgets/Disclaimer'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import AddDocumentsForm from '../components/finetune/AddDocumentsForm'
import AddImagesForm from '../components/finetune/AddImagesForm'
import FileDrawer from '../components/finetune/FileDrawer'

import useRouter from '../hooks/useRouter'
import useLightTheme from '../hooks/useLightTheme'
import useCreateInputs from '../hooks/useCreateInputs'

import {
  ISessionMode,
  ISessionType,
  ICreateSessionConfig,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
} from '../types'

import {
  DEFAULT_SESSION_CONFIG,
  HELIX_DEFAULT_TEXT_MODEL,
  COLORS,
} from '../config'

const PADDING_X = 6

const Create: FC = () => {
  const router = useRouter()
  const lightTheme = useLightTheme()
  const inputs = useCreateInputs()

  const [ sessionConfig, setSessionConfig ] = useState<ICreateSessionConfig>(DEFAULT_SESSION_CONFIG)
  const [ showConfigWindow, setShowConfigWindow ] = useState(false)
  const [ showFileDrawer, setShowFileDrawer ] = useState(false)

  const mode = (router.params.mode as ISessionMode) || SESSION_MODE_INFERENCE
  const type = (router.params.type as ISessionType) || SESSION_TYPE_TEXT
  const model = router.params.model || HELIX_DEFAULT_TEXT_MODEL

  const topbar = (
    <Toolbar
      mode={ mode }
      type={ type }
      model={ model }
      onOpenConfig={ () => setShowConfigWindow(true) }
      onSetMode={ mode => router.setParams({mode}) }
      onSetModel={ model => router.setParams({model}) }
    />
  )

  const headerContent = mode == SESSION_MODE_FINETUNE && (
    <Box
      sx={{
        mt: 3,
        px: PADDING_X,
      }}
    >
      <SessionTypeTabs
        type={ type }
        onSetType={ type => router.setParams({type}) }
      />
    </Box>
  )

  const footerContent = mode == SESSION_MODE_INFERENCE ? (
    <Box sx={{ px: PADDING_X }}>
      <Box sx={{ mb: 3 }}>
        <ExamplePrompts
          type={ type }
          onChange={ (prompt) => {
            inputs.setInputValue(prompt)
          }}
        />
      </Box>
      <Box sx={{ mb: 1 }}>
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
      <Box sx={{ mb: 1 }}>
        <Disclaimer />
      </Box>
    </Box>
  ) : inputs.finetuneFiles.length > 0 ? (
    <Box
      sx={{
        p: 0,
        px: PADDING_X,
        borderTop: lightTheme.border,
        height: '100px',
        backgroundColor: 'rgba(0,0,0,0.5)',
      }}
    >
      <Row sx={{height:'100%'}}>
        <Cell>
          <Typography sx={{ display: 'inline-flex', textAlign: 'left' }}>
            {inputs.finetuneFiles.length} file{inputs.finetuneFiles.length !== 1 ? 's' : ''} added.
            <Link
              component="button"
              onClick={() => setShowFileDrawer(true)}
              sx={{ ml: 0.5, textDecoration: 'underline', color: COLORS[type] }}
              >
              View or edit files
            </Link>
          </Typography>
        </Cell>
        <Cell grow />
        <Cell>
          <Button
            sx={{
              bgcolor: COLORS[type],
              color: 'black',
              borderRadius: 1,
              fontSize: "medium",
              fontWeight: 800,
              textTransform: 'none',
            }}
            variant="contained"
            onClick={() => {}}
          >
            Continue
          </Button>
        </Cell>
      </Row>
    </Box>
  ) : null

  return (
    <Page
      topbarTitle={ mode == SESSION_MODE_FINETUNE ? 'The start of something beautiful' : '' }
      topbarContent={ topbar }
      headerContent={ headerContent }
      footerContent={ footerContent }
      px={ PADDING_X }
      sx={{
        backgroundImage: lightTheme.isLight ? 'url(/img/nebula-light.png)' : 'url(/img/nebula-dark.png)',
        backgroundSize: '80%',
        backgroundPosition: mode == SESSION_MODE_INFERENCE ? 'center center' : `center ${window.innerHeight - 280}px`,
        backgroundRepeat: 'no-repeat',
      }}
    >
      
      {
        mode == SESSION_MODE_INFERENCE && (
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
        )
      }

      {
        mode == SESSION_MODE_FINETUNE && type == SESSION_TYPE_TEXT && (
          <Box
            sx={{
              pt: 2,
              px: PADDING_X,
            }}
          >
            <AddDocumentsForm
              files={ inputs.finetuneFiles }
              onAddFiles={ newFiles => inputs.setFinetuneFiles(files => files.concat(newFiles)) }
            />
          </Box>
        )
      }

      {
        mode == SESSION_MODE_FINETUNE && type == SESSION_TYPE_IMAGE && (
          <Box
            sx={{
              pt: 2,
              px: PADDING_X,
            }}
          >
            <AddImagesForm
              files={ inputs.finetuneFiles }
              onAddFiles={ newFiles => inputs.setFinetuneFiles(files => files.concat(newFiles)) }
            />
          </Box>
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

      {
        showFileDrawer && (
          <FileDrawer
            open
            files={ inputs.finetuneFiles }
            onUpdate={ inputs.setFinetuneFiles }
            onClose={ () => setShowFileDrawer(false) }
          />
        )
      }
    </Page>
  )
}

export default Create
