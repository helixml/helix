import ConstructionIcon from '@mui/icons-material/Construction'
import LoginIcon from '@mui/icons-material/Login'
import Button from '@mui/material/Button'
import IconButton from '@mui/material/IconButton'
import { FC, useEffect } from 'react'

import Cell from '../widgets/Cell'
import Row from '../widgets/Row'
import AppPicker from './AppPicker'

import useAccount from '../../hooks/useAccount'
import useIsBigScreen from '../../hooks/useIsBigScreen'
import useApps from '../../hooks/useApps'
import useRouter from '../../hooks/useRouter'

import {
  IApp,
  ISessionMode,
  ISessionType,
  SESSION_MODE_INFERENCE
} from '../../types'

const CreateToolbar: FC<{
  mode: ISessionMode,
  type: ISessionType,
  model?: string,
  app?: IApp,
  provider?: string,
  onOpenConfig: () => void,
  onSetMode: (mode: ISessionMode) => void,
  onSetModel: (model: string) => void,
}> = ({
  mode,
  type,
  model,
  app,
  provider,
  onOpenConfig,
  onSetMode,
  onSetModel,
}) => {
  const bigScreen = useIsBigScreen()
  const account = useAccount()
  const {
    navigate,
    params,
  } = useRouter()
  const {
    apps,
  } = useApps()

  const appRequested = params.app_id

  return (
    <Row>
      {!appRequested && apps.length > 0 && mode == SESSION_MODE_INFERENCE && (
        <Cell>
          <AppPicker
            apps={apps}
            selectedApp={app}
            onSelectApp={(app) => {
              if(!app) return
              navigate('new', {
                app_id: app.id,
              })
            }}
          />
        </Cell>
      )}
      <Cell grow>
        
      </Cell>
      {
        // don't show the tools icon in inference mode since we don't have
        // global tools any more. we still show it in "learn" mode where it
        // controls rag and finetune settings.
        !app && !(mode === SESSION_MODE_INFERENCE) && (
          <Cell>
            <IconButton
              onClick={ onOpenConfig }
            >
              <ConstructionIcon />
            </IconButton>
          </Cell>
        )
      }
      {/* {
        !app && (
          <Cell>
            {
              bigScreen ? (
                <SessionModeSwitch
                  mode={ mode }
                  onSetMode={ onSetMode }
                />
              ) : (
                <SessionModeDropdown
                  mode={ mode }
                  onSetMode={ onSetMode }
                />
              )
            }
          </Cell>
        )
      } */}
      <Cell>
        {
          !account.user && (bigScreen ? (
            <Button
              id='login-button'
              size="medium"
              variant="contained"
              color="primary"
              endIcon={ <LoginIcon /> }
              onClick={ account.onLogin }
              sx={{
                ml: 2,
              }}
            >
              Login / Register
            </Button> 
          ) : (
            <IconButton
              onClick={ () => account.onLogin() }
            >
              <LoginIcon />
            </IconButton>
          ))
        }
      </Cell>
    </Row>
  )
}

export default CreateToolbar
