import { FC, useState, useEffect } from 'react'
import Page from '../components/system/Page'
import CreateContent from '../components/create/CreateContent'
import useRouter from '../hooks/useRouter'
import useApps from '../hooks/useApps'
import { useAccount } from '../contexts/account'
import useLightTheme from '../hooks/useLightTheme'  
import useIsBigScreen from '../hooks/useIsBigScreen'
import { getNewSessionBreadcrumbs } from '../utils/session'
import { ISessionMode, ISessionType } from '../types'

const Create: FC = () => {
  const router = useRouter()
  const apps = useApps()
  const account = useAccount()
  const lightTheme = useLightTheme()
  const isBigScreen = useIsBigScreen()

  const mode = router.params.mode as ISessionMode
  const type = router.params.type as ISessionType
  const appID = router.params.app_id || ''
  const model = router.params.model || ''

  const [isLoadingApp, setIsLoadingApp] = useState(false)

  useEffect(() => {
    if (!appID) {
      account.orgNavigate('chat')
      return
    }
    setIsLoadingApp(true)
    apps.loadApp(appID).finally(() => setIsLoadingApp(false))
    return () => apps.setApp(undefined)
  }, [appID, router])

  if (appID && (isLoadingApp || !apps.app)) {
    return null
  }

  const pageSX = apps.app ? {} : {
    backgroundImage: lightTheme.isLight ? 'url(/img/nebula-light.png)' : 'url(/img/nebula-dark.png)',
    backgroundSize: '80%',
    backgroundPosition: (mode == "inference" && isBigScreen) ? 'center center' : `center ${window.innerHeight - 280}px`,
    backgroundRepeat: 'no-repeat',
  }

  return (
    <Page
      orgBreadcrumbs={true}
      breadcrumbs={
        getNewSessionBreadcrumbs({
          mode,
          type,
          ragEnabled: router.params.rag ? true : false,
          finetuneEnabled: router.params.finetune ? true : false,
          app: apps.app,
        })
      }
      px={isBigScreen ? 6 : 4}
      sx={{
        ...pageSX,
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
      }}
    >
      <CreateContent 
        mode={mode}
        type={type}
        appID={appID}
        model={model}
        app={apps.app}
      />
    </Page>
  )
}

export default Create