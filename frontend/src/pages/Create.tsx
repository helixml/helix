import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Link from '@mui/material/Link'
import { SxProps } from '@mui/material/styles'
import Typography from '@mui/material/Typography'
import { FC, useEffect, useState, useMemo } from 'react'
import AssistantPicker from '../components/appstore/AssistantPicker'
import AppCreateHeader from '../components/appstore/CreateHeader'
import CenterMessage from '../components/create/CenterMessage'
import ConfigWindow from '../components/create/ConfigWindow'
import ExamplePrompts from '../components/create/ExamplePrompts'
import InferenceTextField from '../components/create/InferenceTextField'
import SessionTypeButton from '../components/create/SessionTypeButton'
import SessionTypeTabs from '../components/create/SessionTypeTabs'
import Toolbar from '../components/create/Toolbar'
import AddDocumentsForm from '../components/finetune/AddDocumentsForm'
import AddImagesForm from '../components/finetune/AddImagesForm'
import FileDrawer from '../components/finetune/FileDrawer'
import LabelImagesForm from '../components/finetune/LabelImagesForm'
import Page from '../components/system/Page'
import Cell from '../components/widgets/Cell'
import Disclaimer from '../components/widgets/Disclaimer'
import Row from '../components/widgets/Row'
import UploadingOverlay from '../components/widgets/UploadingOverlay'
import useAccount from '../hooks/useAccount'
import useApi from '../hooks/useApi'
import useApps from '../hooks/useApps'
import useCreateInputs from '../hooks/useCreateInputs'
import useIsBigScreen from '../hooks/useIsBigScreen'
import useLightTheme from '../hooks/useLightTheme'
import useRouter from '../hooks/useRouter'
import useSessions from '../hooks/useSessions'
import useSnackbar from '../hooks/useSnackbar'
import useTracking from '../hooks/useTracking'
import { useStreaming } from '../contexts/streaming'

import {
  IDataEntity,
  ISessionMode,
  ISessionType,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_IMAGE,
  SESSION_TYPE_TEXT,
} from '../types'

import {
  COLORS,
} from '../config'

import {
  getNewSessionBreadcrumbs,
} from '../utils/session'

import {
  getAssistant,
  getAssistantAvatar,
  getAssistantName,
} from '../utils/apps'

// First, we need to import the necessary components
import EditIcon from '@mui/icons-material/Edit'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'

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
  const apps = useApps()
  const { NewInference } = useStreaming()

  const [showConfigWindow, setShowConfigWindow] = useState(false)
  const [showFileDrawer, setShowFileDrawer] = useState(false)
  const [showImageLabelsEmptyError, setShowImageLabelsEmptyError] = useState(false)
  const [focusInput, setFocusInput] = useState(false)
  const [loading, setLoading] = useState(false)

  const mode = (router.params.mode as ISessionMode) || SESSION_MODE_INFERENCE
  const type = (router.params.type as ISessionType) || SESSION_TYPE_TEXT
  const appID = router.params.app_id || ''
  const model = router.params.model || ''

  const activeAssistantID = router.params.assistant_id || '0'
  const activeAssistant = apps.app && getAssistant(apps.app, activeAssistantID)

  const imageFineTuneStep = router.params.imageFineTuneStep || 'upload'
  const PADDING_X = isBigScreen ? PADDING_X_LARGE : PADDING_X_SMALL

  // Then, in the Create component, we'll add a check to see if the current user owns the app
  // This should be added near the top of the component, after the existing useEffect hooks

  const userOwnsApp = useMemo(() => {
    if (!apps.app || !account.user) return false
    return apps.app.owner === account.user.id
  }, [apps.app, account.user])

  /*
   *
   *
   * 
  
    CALLBACKS
  
   *
   * 
   *  
  */

  // we are about to do a funetune, check if the user is logged in
  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      inputs.serializePage()
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }

  const onInference = async () => {
    if (!checkLoginStatus()) return
    setLoading(true)

    const urlParams = new URLSearchParams(window.location.search)
    const appID = urlParams.get('app_id') || ''
    let assistantID = urlParams.get('assistant_id') || ''
    const ragSourceID = urlParams.get('rag_source_id') || ''

    // if we have an app but no assistant ID let's default to the first one
    if(appID && !assistantID) {
      assistantID = '0'
    }

    try {
      const session = await NewInference({
        type: type as ISessionType,
        message: inputs.inputValue,
        appId: appID,
        assistantId: assistantID,
        ragSourceId: ragSourceID,
        modelName: model,
        loraDir: '',
      });

      if (!session) return
      tracking.emitEvent({
        name: 'inference',
        session,
      })
      await sessions.loadSessions()
      setLoading(false)
      router.navigate('session', { session_id: session.id })
    } catch (error) {
      console.error('Error in onInference:', error);
      snackbar.error('Failed to start inference');
      setLoading(false);
    }
  }

  const onStartFinetune = async (eventName: string) => {
    try {
      inputs.setUploadProgress({
        percent: 0,
        totalBytes: 0,
        uploadedBytes: 0,
      })

      const dataEntity = await api.post<any, IDataEntity>('/api/v1/data_entities', inputs.getUploadedFiles(), {
        onUploadProgress: inputs.uploadProgressHandler,
      })

      if (!dataEntity) {
        snackbar.error('Failed to upload data entity')
        throw new Error('Failed to upload data entity')
      }

      const sessionLearnRequest = inputs.getSessionLearnRequest(type, dataEntity.id)
      const session = await api.post('/api/v1/sessions/learn', sessionLearnRequest, {
        onUploadProgress: inputs.uploadProgressHandler,
      })
      inputs.setUploadProgress(undefined)
      if (!session) {
        snackbar.error('Failed to get new session')
        throw new Error('Failed to get new session')
      }
      tracking.emitEvent({
        name: eventName,
        session,
      })
      await sessions.loadSessions()
      router.navigate('session', { session_id: session.id })

    } catch (e) {
      inputs.setUploadProgress(undefined)
    }
  }

  const onStartTextFinetune = async () => {
    if (!checkLoginStatus()) return
    await onStartFinetune('finetune:text')
  }

  const onStartImageFunetune = async () => {
    const emptyLabel = inputs.finetuneFiles.find(file => {
      return inputs.labels[file.file.name] ? false : true
    })
    if (emptyLabel) {
      setShowImageLabelsEmptyError(true)
      snackbar.error('Please label all images before continuing')
      return
    } else {
      setShowImageLabelsEmptyError(false)
    }

    if (!checkLoginStatus()) return
    await onStartFinetune('finetune:image')
  }

  /*
   *
   *
   * 
  
    EFFECTS
  
   *
   * 
   *  
  */

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

  useEffect(() => {
    if (!account.user) return
    if (!appID) return
    apps.loadApp(appID)
    return () => apps.setApp(undefined)
  }, [
    account.user,
    appID,
  ])

  /*
   *
   *
   * 
  
    COMPONENTS
  
   *
   * 
   *  
  */

  const topbar = (
    <Toolbar
      mode={mode}
      type={type}
      model={model}
      app={apps.app}
      onOpenConfig={() => setShowConfigWindow(true)}
      onSetMode={mode => {
        if (mode == "finetune") {
          // default rag true in case user clicks on the toggle
          router.setParams({ mode: mode, rag: "true" })
        } else {
          router.setParams({ mode })
        }
      }}
      onSetModel={model => {
        router.setParams({ model })
        // Trigger focus on the text box after setting the model
        setFocusInput(true)
      }}
    />
  )

  const activeAssistantAvatar = activeAssistant && apps.app ? getAssistantAvatar(apps.app, activeAssistantID) : ''
  const activeAssistantName = activeAssistant && apps.app ? getAssistantName(apps.app, activeAssistantID) : ''

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
          loading={loading}
          type={type}
          focus={focusInput ? 'true' : activeAssistantID}
          value={inputs.inputValue}
          disabled={mode == SESSION_MODE_FINETUNE}
          startAdornment={isBigScreen && (
            activeAssistant ? (
              activeAssistantAvatar ? (
                <Avatar
                  src={activeAssistantAvatar}
                  sx={{
                    width: '30px',
                    height: '30px',
                  }}
                />
              ) : null
            ) : (
              <SessionTypeButton
                type={type}
                onSetType={type => router.setParams({ type })}
              />
            )
          )}
          promptLabel={activeAssistant ? `Chat with ${activeAssistantName || ''}` : undefined}
          onUpdate={inputs.setInputValue}
          onInference={onInference}
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
      <Row sx={{ height: '100%' }}>
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
              if (type == SESSION_TYPE_TEXT) {
                onStartTextFinetune()
              } else if (type == SESSION_TYPE_IMAGE) {
                if (imageFineTuneStep == 'upload') {
                  router.setParams({ imageFineTuneStep: 'label' })
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
        files={inputs.finetuneFiles}
        onAddFiles={newFiles => inputs.setFinetuneFiles(files => files.concat(newFiles))}
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
        files={inputs.finetuneFiles}
        onAddFiles={newFiles => inputs.setFinetuneFiles(files => files.concat(newFiles))}
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
        files={inputs.finetuneFiles}
        labels={inputs.labels}
        showEmptyErrors={showImageLabelsEmptyError}
        onSetLabels={inputs.setLabels}
      />
    </Box>
  )

  const inferenceHeaderNormal = (
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
          type={type}
          onSetType={type => router.setParams({ type })}
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
          type={type}
          onChange={(prompt) => {
            inputs.setInputValue(prompt)
          }}
        />
      </Cell>
    </Row>
  )

  const inferenceHeaderApp = apps.app && (
    <Row
      id="HEADER"
      vertical
      center
      sx={{
        position: 'relative',
        backgroundImage: `url(${apps.app.config.helix.image || '/img/app-editor-swirl.webp'})`,
        backgroundPosition: 'top',
        backgroundRepeat: 'no-repeat',
        backgroundSize: apps.app.config.helix.image ? 'cover' : 'auto',
        p: 2,
      }}
    >
      {apps.app.config.helix.image && (
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.8)',
            zIndex: 1,
          }}
        />
      )}
      {userOwnsApp && (
        <Box
          sx={{
            position: 'absolute',
            top: 16,
            right: 16,
            zIndex: 3,
          }}
        >
          <Tooltip title="Edit App">
            <IconButton
              onClick={() => router.navigate('app', { app_id: apps.app?.id })}
              sx={{
                color: 'white',
                backgroundColor: 'rgba(255, 255, 255, 0.1)',
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.2)',
                },
              }}
            >
              <EditIcon />
            </IconButton>
          </Tooltip>
        </Box>
      )}
      <Cell
        sx={{
          pt: 4,
          px: PADDING_X,
          textAlign: 'center',
          position: 'relative',
          zIndex: 2,
        }}
      >
        <AppCreateHeader
          app={apps.app}
        />
      </Cell>
      <Cell
        sx={{
          px: PADDING_X,
          py: 2,
          pt: 4,
          width: '100%',
          position: 'relative',
          zIndex: 2,
        }}
      >
        <AssistantPicker
          app={apps.app}
          activeAssistantID={activeAssistantID}
          onClick={(index) => {
            router.setParams({ assistant_id: index.toString() })
          }}
        />
      </Cell>
    </Row>
  )

  const inferenceHeader = apps.app ? inferenceHeaderApp : inferenceHeaderNormal
  const pageSX: SxProps = apps.app ? {

  } : {
    backgroundImage: lightTheme.isLight ? 'url(/img/nebula-light.png)' : 'url(/img/nebula-dark.png)',
    backgroundSize: '80%',
    backgroundPosition: (mode == SESSION_MODE_INFERENCE && isBigScreen) ? 'center center' : `center ${window.innerHeight - 280}px`,
    backgroundRepeat: 'no-repeat',
  }

  // Reset focusInput after it's been used
  useEffect(() => {
    if (focusInput) {
      setFocusInput(false)
    }
  }, [focusInput])

  return (
    <Page
      breadcrumbs={
        getNewSessionBreadcrumbs({
          mode,
          type,
          ragEnabled: router.params.rag ? true : false,
          finetuneEnabled: router.params.finetune ? true : false,
          app: apps.app,
        })
      }
      topbarContent={topbar}
      footerContent={mode == SESSION_MODE_INFERENCE ? inferenceFooter : finetuneFooter}
      px={PADDING_X}
      sx={pageSX}
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
              type={type}
              onSetType={type => router.setParams({ type })}
            />
          </Box>
        )
      }

      {
        mode == SESSION_MODE_INFERENCE && inferenceHeader
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
            mode={mode}
            type={type}
            sessionConfig={inputs.sessionConfig}
            onSetSessionConfig={inputs.setSessionConfig}
            onClose={() => setShowConfigWindow(false)}
          />
        )
      }

      {
        showFileDrawer && (
          <FileDrawer
            open
            files={inputs.finetuneFiles}
            onUpdate={inputs.setFinetuneFiles}
            onClose={() => setShowFileDrawer(false)}
          />
        )
      }

      {
        inputs.uploadProgress && (
          <UploadingOverlay
            percent={inputs.uploadProgress.percent}
          />
        )
      }
    </Page>
  )
}

export default Create