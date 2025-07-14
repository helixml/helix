import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import { FC, useState, useMemo, useContext } from 'react'
import AppCreateHeader from '../appstore/CreateHeader'
import CenterMessage from './CenterMessage'
import ConfigWindow from './ConfigWindow'
import ExamplePrompts from './ExamplePrompts'
import InferenceTextField from './InferenceTextField'
import SessionTypeButton from './SessionTypeButton'
import Toolbar from './Toolbar'
import FileDrawer from '../finetune/FileDrawer'
import { AccountContext } from '../../contexts/account'
import Cell from '../widgets/Cell'
import Disclaimer from '../widgets/Disclaimer'
import Row from '../widgets/Row'
import UploadingOverlay from '../widgets/UploadingOverlay'
import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useApps from '../../hooks/useApps'
import useCreateInputs from '../../hooks/useCreateInputs'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useSessions from '../../hooks/useSessions'
import useSnackbar from '../../hooks/useSnackbar'
import useTracking from '../../hooks/useTracking'
import useUserAppAccess from '../../hooks/useUserAppAccess'
import { useStreaming } from '../../contexts/streaming'
import ConversationStarters from './ConversationStarters'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_FINETUNE,
  SESSION_MODE_INFERENCE,
  SESSION_TYPE_TEXT,
  IApp,
} from '../../types'


import {
  getAssistant,
  getAssistantAvatar,  
} from '../../utils/apps'



const PADDING_X_LARGE = 6
const PADDING_X_SMALL = 4

interface CreateContentProps {
  mode?: ISessionMode;
  type?: ISessionType;
  appID?: string;
  model?: string;
  assistantID?: string;
  onClose?: () => void;
  isEmbedded?: boolean;
  app?: IApp;
}

const CreateContent: FC<CreateContentProps> = ({
  mode: initialMode,
  type: initialType,
  appID: initialAppID,
  model: initialModel,
  assistantID: initialAssistantID,
  onClose,
  isEmbedded = false,
  app: appProp,
}) => {
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
  const { models } = useContext(AccountContext)

  const [showConfigWindow, setShowConfigWindow] = useState(false)
  const [showFileDrawer, setShowFileDrawer] = useState(false)  
  const [focusInput, setFocusInput] = useState(false)
  const [loading, setLoading] = useState(false)
  const [attachedImages, setAttachedImages] = useState<File[]>([])

  const mode = initialMode || (router.params.mode as ISessionMode) || SESSION_MODE_INFERENCE
  const type = initialType || (router.params.type as ISessionType) || SESSION_TYPE_TEXT
  const appID = initialAppID || router.params.app_id || ''
  const model = initialModel || router.params.model || ''
  const activeAssistantID = initialAssistantID || router.params.assistant_id || '0'

  const userAppAccess = useUserAppAccess(appID)

  const app = appProp || apps.app

  const activeAssistant = app && getAssistant(app, activeAssistantID)

  const PADDING_X = isBigScreen ? PADDING_X_LARGE : PADDING_X_SMALL

  const filteredModels = useMemo(() => {
    return models.filter(m => m.type && m.type === type || (type === "text" && m.type === "chat"))
  }, [models, type])

  const userOwnsApp = useMemo(() => {
    return userAppAccess.canRead
  }, [userAppAccess.canRead])

  const onInference = async (prompt?: string) => {
    if (!checkLoginStatus()) return
    setLoading(true)

    const urlParams = new URLSearchParams(window.location.search)
    const appID = urlParams.get('app_id') || ''
    let assistantID = urlParams.get('assistant_id') || ''
    const ragSourceID = urlParams.get('rag_source_id') || ''
    let useModel = urlParams.get('model') || ''
    let orgId = ''

    // if we have an app but no assistant ID let's default to the first one
    if (appID && !assistantID) {
      assistantID = '0'
    }

    if (!useModel) {
      useModel = filteredModels[0].id
    }

    if (account.organizationTools.organization?.id) {
      orgId = account.organizationTools.organization.id
    }

    prompt = prompt || inputs.inputValue

    try {
      const session = await NewInference({
        type: type as ISessionType,
        message: prompt,
        appId: appID,
        assistantId: assistantID,
        ragSourceId: ragSourceID,
        modelName: useModel,
        loraDir: '',
        orgId,
        attachedImages: attachedImages,
      });

      if (!session) return
      tracking.emitEvent({
        name: 'inference',
        session,
      })
      await sessions.loadSessions()
      setLoading(false)
      account.orgNavigate('session', { session_id: session.id })
    } catch (error) {
      console.error('Error in onInference:', error);
      snackbar.error('Failed to start inference');
      setLoading(false);
    }
  }

  const inferenceHeaderNormal = (
    <Row
      vertical
      center
      sx={{ minHeight: 0, p: 0, m: 0 }}
    >
      <Cell
        sx={{
          pt: 0.5,
          pb: 0.5,
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
          py: 0.5,
          maxWidth: '900px',
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

  const inferenceHeaderApp = app && (
    <Row
      id="HEADER"
      sx={{
        position: 'relative',
        backgroundImage: `url(${app.config.helix.image || '/img/app-editor-swirl.webp'})`,
        backgroundPosition: 'top',
        backgroundRepeat: 'no-repeat',
        backgroundSize: app.config.helix.image ? 'cover' : 'auto',
        p: 0,
        minHeight: 0,
        height: '80px',
        alignItems: 'center',
        justifyContent: 'flex-start',
      }}
    >
      {app.config.helix.image && (
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
      <Cell
        sx={{
          pt: 0.5,
          px: PADDING_X,
          position: 'relative',
          zIndex: 2,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-start',
        }}
      >
        <AppCreateHeader
          app={app}
          showEditButton={userOwnsApp}
          onEditClick={() => account.orgNavigate('app', { app_id: app?.id })}
        />
      </Cell>
    </Row>
  )

  const inferenceHeader = app ? inferenceHeaderApp : inferenceHeaderNormal
  
  const topbar = (
    <Toolbar
      mode={mode}
      type={type}
      model={model}
      app={app}
      onOpenConfig={() => setShowConfigWindow(true)}
      onSetMode={mode => {
        if (mode == "finetune") {
          router.setParams({ mode: mode, rag: "true" })
        } else {
          router.setParams({ mode })
        }
      }}
      onSetModel={model => {
        router.setParams({ model })
        setFocusInput(true)
      }}
    />
  )

  const activeAssistantAvatar = activeAssistant && app ? getAssistantAvatar(app, activeAssistantID) : ''  

  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      inputs.serializePage()
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%', minHeight: 0 }}>
      {!isEmbedded && topbar}
      {mode == SESSION_MODE_INFERENCE && inferenceHeader}
      {/* Main content area, fills available space */}
      <Box sx={{ flexGrow: 1, minHeight: 0, overflow: 'auto', width: '100%' }}>        
        <Container maxWidth="lg" sx={{ height: '100%', display: 'flex', flexDirection: 'column', justifyContent: 'flex-start', minHeight: 0, py: 2 }}>
          {/* This area can be used for additional content if needed */}
          <Box sx={{ flexGrow: 1 }}>
            {/* Reserved space for future content */}
          </Box>
        </Container>       
      </Box>
      {/* 
        Bottom fixed section with conversation starters, input, and disclaimer. This should always
        be at the bottom of the screen
      */}
      {mode == SESSION_MODE_INFERENCE && (
        <Box sx={{ flexShrink: 0 }}>
          <Container maxWidth="lg">
            <Box sx={{ py: 2 }}>
              <Row>
                <Cell flexGrow={1}>
                  <Box
                    sx={{
                      margin: '0 auto',
                      display: 'flex',
                      flexDirection: 'column',
                      gap: 2,
                    }}
                  >
                    <Box sx={{ width: '100%' }}>
                      <Stack direction="row" spacing={2} justifyContent="center">
                        <ConversationStarters
                          conversationStarters={
                            (activeAssistant && activeAssistant.conversation_starters && activeAssistant.conversation_starters.length > 0)
                              ? activeAssistant.conversation_starters
                              : ((app?.config.helix as any)?.conversation_starters || [])
                          }
                          layout="horizontal"
                          header={false}
                          onChange={async (prompt) => {
                            inputs.setInputValue(prompt)
                            onInference(prompt)
                          }}
                        />
                      </Stack>
                    </Box>
                    <Box sx={{ width: '100%' }}>
                      <InferenceTextField
                        appId={appID}
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
                        promptLabel={activeAssistant ? `Chat with ${app?.config.helix.name || ''}` : undefined}
                        onUpdate={inputs.setInputValue}
                        onInference={onInference}
                        attachedImages={attachedImages}
                        onAttachedImagesChange={setAttachedImages}
                      />
                    </Box>
                  </Box>
                </Cell>
              </Row>
              <Box sx={{ mt: 2 }}>
                <Disclaimer />
              </Box>
            </Box>
          </Container>
        </Box>
      )}
      {showConfigWindow && (
        <ConfigWindow
          mode={mode}
          type={type}
          sessionConfig={inputs.sessionConfig}
          onSetSessionConfig={inputs.setSessionConfig}
          onClose={() => setShowConfigWindow(false)}
        />
      )}
      {showFileDrawer && (
        <FileDrawer
          open
          files={inputs.finetuneFiles}
          onUpdate={inputs.setFinetuneFiles}
          onClose={() => setShowFileDrawer(false)}
        />
      )}
      {inputs.uploadProgress && (
        <UploadingOverlay
          percent={inputs.uploadProgress.percent}
        />
      )}
    </Box>
  )
}

export default CreateContent 