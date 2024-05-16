import React, { FC, useState, useEffect, useCallback } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Link from '@mui/material/Link'
import Button from '@mui/material/Button'

import ConstructionIcon from '@mui/icons-material/Construction'

import Page from '../components/system/Page'
import Toolbar from '../components/create/Toolbar'
import ConfigWindow from '../components/create/ConfigWindow'

import CenterMessage from '../components/create/CenterMessage'
import ExamplePrompts from '../components/create/ExamplePrompts'
import InferenceTextField from '../components/create/InferenceTextField'
import Disclaimer from '../components/widgets/Disclaimer'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import AddDocumentsForm from '../components/finetune/AddDocumentsForm'
import AddImagesForm from '../components/finetune/AddImagesForm'
import LabelImagesForm from '../components/finetune/LabelImagesForm'
import FileDrawer from '../components/finetune/FileDrawer'
import UploadingOverlay from '../components/widgets/UploadingOverlay'
import ModelPicker from '../components/create/ModelPicker'

import SessionModeSwitch from '../components/create/SessionModeSwitch'
import SessionTypeSwitch from '../components/create/SessionTypeSwitch'
import SessionTypeTabs from '../components/create/SessionTypeTabs'
import SessionTypeButton from '../components/create/SessionTypeButton'
import SessionModeButton from '../components/create/SessionModeButton'

import useRouter from '../hooks/useRouter'
import useLightTheme from '../hooks/useLightTheme'
import useCreateInputs from '../hooks/useCreateInputs'
import useSnackbar from '../hooks/useSnackbar'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useTracking from '../hooks/useTracking'
import useSessions from '../hooks/useSessions'
import useIsBigScreen from '../hooks/useIsBigScreen'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
} from '../types'

import {
  HELIX_DEFAULT_TEXT_MODEL,
  COLORS,
} from '../config'

const PADDING_X_LARGE = 6
const PADDING_X_SMALL = 4

const Create: FC = () => {
  const router = useRouter()
  const lightTheme = useLightTheme()
  const inputs = useCreateInputs()
  const snackbar = useSnackbar()
  const account = useAccount()
  const api = useApi()
  const tracking = useTracking()
  const sessions = useSessions()
  const isBigScreen = useIsBigScreen()

  const [ showConfigWindow, setShowConfigWindow ] = useState(false)
  const [ showFileDrawer, setShowFileDrawer ] = useState(false)
  const [ showImageLabelsEmptyError, setShowImageLabelsEmptyError ] = useState(false)

  const mode = (router.params.mode as ISessionMode) || SESSION_MODE_INFERENCE
  const type = (router.params.type as ISessionType) || SESSION_TYPE_TEXT
  const model = router.params.model || HELIX_DEFAULT_TEXT_MODEL
  const imageFineTuneStep = router.params.imageFineTuneStep || 'upload'
  const PADDING_X = isBigScreen ? PADDING_X_LARGE : PADDING_X_SMALL

  // we are about to do a funetune, check if the user is logged in
  const checkLoginStatus = (): boolean => {
    if(!account.user) {
      inputs.serializePage()
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }

  const onInference = async () => {
    if(!checkLoginStatus()) return
    const formData = inputs.getFormData(mode, type, model)
    const session = await api.post('/api/v1/sessions', formData)
    if(!session) return
    tracking.emitEvent({
      name: 'inference',
      session,
    })
    await sessions.loadSessions()
    router.navigate('session', {session_id: session.id})
  }

  const onStartFinetune = async (eventName: string) => {
    inputs.setUploadProgress({
      percent: 0,
      totalBytes: 0,
      uploadedBytes: 0,
    })

    const uploadedFiles = inputs.getUploadedFiles()
    const formData = inputs.getFormData(mode, type, model)

    const dataEntity = await api.post('/api/v1/data_entities', uploadedFiles, {
      onUploadProgress: inputs.uploadProgressHandler,
    })

    console.log('--------------------------------------------')
    console.dir(dataEntity)

    // const session = await api.post('/api/v1/sessions', formData, {
    //   onUploadProgress: inputs.uploadProgressHandler,
    // })
    // inputs.setUploadProgress(undefined)
    // if(!session) {
    //   return
    // }
    // tracking.emitEvent({
    //   name: eventName,
    //   session,
    // })
    // await sessions.loadSessions()
    // router.navigate('session', {session_id: session.id})
  }

  const onStartTextFinetune = async () => {
    if(!checkLoginStatus()) return
    await onStartFinetune('finetune:text')
  }

  const onStartImageFunetune = async () => {
    const emptyLabel = inputs.finetuneFiles.find(file => {
      return inputs.labels[file.file.name] ? false : true
    })
    if(emptyLabel) {
      setShowImageLabelsEmptyError(true)
      snackbar.error('Please label all images before continuing')
      return
    } else {
      setShowImageLabelsEmptyError(false)
    }
    
    if(!checkLoginStatus()) return
    await onStartFinetune('finetune:image')
  }

  useEffect(() => {
    inputs.loadFromLocalStorage()
  }, [])

  useEffect(() => {
    inputs.setFinetuneFiles([])
    inputs.setLabels({})
    router.removeParams(['imageFineTuneStep'])
  }, [
    mode,
    type,
  ])

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

  const inferenceFooter = (
    <Box
      sx={{
        px: PADDING_X,
        pt: 3,
        borderTop: isBigScreen ? '' : lightTheme.border,
      }}
    >
      <Box sx={{ mb: 1 }}>
        <InferenceTextField
          type={ type }
          value={ inputs.inputValue }
          disabled={ mode == SESSION_MODE_FINETUNE }
          startAdornment={ isBigScreen && (
            <SessionTypeButton
              type={ type }
              onSetType={ type => router.setParams({type}) }
            />
          )}
          onUpdate={ inputs.setInputValue }
          onInference={ onInference }
        />
      </Box>
      <Box sx={{ mb: 1 }}>
        <Disclaimer />
      </Box>
    </Box>
  )

  const finetuneFooter = inputs.finetuneFiles.length > 0 && (
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
          {
            type == SESSION_TYPE_IMAGE && imageFineTuneStep == 'label' ? (
              <Typography sx={{ display: 'inline-flex', textAlign: 'left' }}>
                <Link
                  component="button"
                  onClick={() => router.removeParams(['imageFineTuneStep'])}
                  sx={{ ml: 0.5, textDecoration: 'underline', color: COLORS[type] }}
                >
                  Return to upload images
                </Link>
              </Typography>
            ) : (
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
            )
          }
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
            onClick={() => {
              if(type == SESSION_TYPE_TEXT) {
                onStartTextFinetune()
              } else if (type == SESSION_TYPE_IMAGE) {
                if(imageFineTuneStep == 'upload') {
                  router.setParams({imageFineTuneStep: 'label'})
                } else {
                  onStartImageFunetune()
                }
              }
            }}
          >
            {
              type == SESSION_TYPE_IMAGE && imageFineTuneStep == 'label' ? 'Start training' : 'Continue'
            }
          </Button>
        </Cell>
      </Row>
    </Box>
  )

  const finetuneAddDocumentsForm = mode == SESSION_MODE_FINETUNE && type == SESSION_TYPE_TEXT && (
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

  const finetuneAddImagesForm = mode == SESSION_MODE_FINETUNE && type == SESSION_TYPE_IMAGE && imageFineTuneStep == 'upload' && (
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

  const finetuneLabelImagesForm = mode == SESSION_MODE_FINETUNE && type == SESSION_TYPE_IMAGE && imageFineTuneStep == 'label' && (
    <Box
      sx={{
        pt: 2,
        px: PADDING_X,
      }}
    >
      <LabelImagesForm
        files={ inputs.finetuneFiles }
        labels={ inputs.labels }
        showEmptyErrors={ showImageLabelsEmptyError }
        onSetLabels={ inputs.setLabels }
      />
    </Box>
  )
  
  return (
    <Page
      breadcrumbTitle={ mode == SESSION_MODE_FINETUNE ? "Create" : "" }
      topbarContent={ topbar }
      footerContent={ mode == SESSION_MODE_INFERENCE ? inferenceFooter : finetuneFooter }
      px={ PADDING_X }
      sx={{
        backgroundImage: lightTheme.isLight ? 'url(/img/nebula-light.png)' : 'url(/img/nebula-dark.png)',
        backgroundSize: '80%',
        backgroundPosition: (mode == SESSION_MODE_INFERENCE && isBigScreen) ? 'center center' : `center ${window.innerHeight - 280}px`,
        backgroundRepeat: 'no-repeat',
      }}
    >

      {
        mode == SESSION_MODE_FINETUNE && (
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
      }

      {
        mode == SESSION_MODE_INFERENCE && (
          <Row
            vertical
            center
          >
            <Cell
              sx={{
                pt: 4,
                px: PADDING_X,
              }}
            >
              <CenterMessage
                type={ type }
                onSetType={ type => router.setParams({type}) }
              />
            </Cell>
            <Cell grow />
            <Cell
              sx={{
                px: PADDING_X,
                py: 2,
                maxWidth: '900px'
              }}
            >
              <ExamplePrompts
                type={ type }
                onChange={ (prompt) => {
                  inputs.setInputValue(prompt)
                }}
              />
            </Cell>
          </Row>
        )
      }

      {
        finetuneAddDocumentsForm
      }

      {
        finetuneAddImagesForm
      }

      {
        finetuneLabelImagesForm
      }
      
      {
        showConfigWindow && (
          <ConfigWindow
            mode={ mode }
            type={ type }
            sessionConfig={ inputs.sessionConfig }
            onSetSessionConfig={ inputs.setSessionConfig }
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

      {
        inputs.uploadProgress && (
          <UploadingOverlay
            percent={ inputs.uploadProgress.percent }
          />
        )
      }
    </Page>
  )
}

export default Create
