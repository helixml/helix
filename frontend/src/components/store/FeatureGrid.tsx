import React, { FC } from 'react'
import { SxProps } from '@mui/system'
import Divider from '@mui/material/Divider'
import Box from '@mui/material/Box'
import Avatar from '@mui/material/Avatar'
import Grid from '@mui/material/Grid'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Card from '@mui/material/Card'
import CardActions from '@mui/material/CardActions'
import CardActionArea from '@mui/material/CardActionArea'
import CardContent from '@mui/material/CardContent'
import CardMedia from '@mui/material/CardMedia'
import ChatIcon from '@mui/icons-material/Chat'
import ImageIcon from '@mui/icons-material/Image'
import AppsIcon from '@mui/icons-material/Apps'
import DocumentScannerIcon from '@mui/icons-material/DocumentScanner'
import ModelTrainingIcon from '@mui/icons-material/ModelTraining'
import JavascriptIcon from '@mui/icons-material/Javascript'
import DashboardIcon from '@mui/icons-material/Dashboard'
import WebIcon from '@mui/icons-material/Web'
import ApiIcon from '@mui/icons-material/Api'
import SettingsIcon from '@mui/icons-material/Settings'
import GroupIcon from '@mui/icons-material/Group'
import PermMediaIcon from '@mui/icons-material/PermMedia'
import PlagiarismIcon from '@mui/icons-material/Plagiarism'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'
import useTracking from '../../hooks/useTracking'
import useSessions from '../../hooks/useSessions'
import useApi from '../../hooks/useApi'

import {
  IApp,
  ISessionChatRequest,
} from '../../types'

import {
  IFeature,
} from '../../types'

const HomeFeatureCard: FC<{
  feature: IFeature,
}> = ({
  feature,
}) => {
  const router = useRouter()

  return (
    <Card>
      <CardActionArea
        disabled={feature.disabled}
        onClick={() => {
          feature.actions[0].handler(router.navigate)
        }}
      >
        {
          feature.image && (
            <CardMedia
              sx={{ height: 140 }}
              image={ feature.image }
              title={ feature.title }
            />
          )
        }
        <CardContent
          sx={{
            cursor: 'pointer',
          }}
        >
          <Row
            sx={{
              alignItems: 'flex-start',
            }}
          >
            {
              feature.icon && (
                <Cell
                  sx={{
                    mr: 2,
                    pt: 1,
                  }}
                >
                  <Avatar
                    sx={{
                      // width: 48,
                      // height: 48,
                      backgroundColor: 'black',
                    }}
                  >
                    { feature.icon }
                  </Avatar>
                </Cell>
              )
            }
            <Cell grow sx={{
              minHeight: '80px'
            }}>
              <Typography gutterBottom variant="h5" component="div">
                { feature.title }
              </Typography>
              <Typography variant="body2" color="text.secondary">
                { feature.description }
              </Typography>
            </Cell>
          </Row>
        </CardContent>
      </CardActionArea>
      <CardActions>
        <Row>
          {
            feature.actions.map((action, index) => (
              <Cell key={ index }>
                <Button
                  size="small"
                  variant={ action.variant }
                  color={ action.color }
                  onClick={ () => action.handler(router.navigate) }
                  disabled={ feature.disabled }
                >
                  { action.title }
                </Button>
              </Cell>
            ))
          }
        </Row>
      </CardActions>
    </Card>
  )
}

const StoreFeatureSection: FC<{
  title: string,
  features: IFeature[],
  sx?: SxProps,
}> = ({
  title,
  features,
  sx = {},
}) => {
  return (
    <Box sx={sx}>
      <Typography
        variant="h4"
        sx={{
          textAlign: 'left',
        }}
      >
        { title }
      </Typography>
      <Divider
        sx={{
          my: 2,
        }}
      />
      <Box sx={{
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
      }}>
        <Grid container spacing={ 4 }>
          { features.map((feature, index) => (
            <Grid item xs={ 12 } sm={ 12 } md={ 6 } lg={ 4 } key={ index } sx={{ p: 0, m: 0 }}>
              <HomeFeatureCard
                feature={ feature }
              />
            </Grid>
          )) }
        </Grid>
      </Box>
    </Box>
  )
}


const StoreFeatureGrid: FC<{
  apps: IApp[]
}> = ({
  apps 
}) => {

  console.log(apps)
  const account = useAccount()
  const router = useRouter()
  const tracking = useTracking()
  const sessions = useSessions()
  const api = useApi()

  // TODO: maybe we should create new session from an app on the backend?

  const launchApp = async (appID: string) => {
    // create a new session with the app id

    // TODO: pull out the type from the app's 0'th assistant
    // TODO: do we actually want to create a set of sessions, one per assistant in the app?


    const app = await api.get<IApp>(`/api/v1/apps/${appID}`)
    if(!app) return

    if (!app.config.helix?.assistants || app.config.helix.assistants.length === 0) {
      return
    }

    const model = app.config.helix.assistants[0].model

    // TODO: add type field to assistant throughout (e.g. assistants can be fine
    // tuned image models)
    const type = model.includes("sdxl") ? "image" : "text";

    const req: ISessionChatRequest = {
      type: type,
      model: model,
      stream: true,
      legacy: true,
      app_id: appID,
      // no messages, just ready to receive one from user
      messages: []
    }

    const session = await api.post('/api/v1/sessions/chat', req)

    if(!session) return
    tracking.emitEvent({
      name: 'app_launch',
      app: appID,
      session,
    })
    await sessions.loadSessions()
    router.navigate('session', {session_id: session.id})
  }

  const APP_1: IFeature = {
    title: 'Sarcastic Bob',
    description: "It's an AI chatbot that's mean to you. Meet Sarcastic Bob. He won't be nice, but it might be funny.",
    image: 'https://www.dictionary.com/e/wp-content/uploads/2018/03/sideshow-bob.jpg',
    // icon: <ChatIcon sx={{color: '#fcdb05'}} />,
    actions: [{
      title: "I'm ready",
      color: 'secondary',
      variant: 'outlined',
      // TODO: get this from apps data
      handler: () => launchApp("app_01hyx25hdae1a3bvexs6dc2qhk"),
    }]
  }

  const APP_2: IFeature = {
    title: 'Waitrose Demo',
    description: "Personalized recipe recommendations, based on your purchase history and our recipe database. Yummy.",
    image: 'https://waitrose-prod.scene7.com/is/image/waitroseprod/cp-essential-everyday?uuid=0845d10c-ed0d-4961-bc85-9e571d35cd63&$Waitrose-Image-Preset-95$',
    // icon: <ChatIcon sx={{color: '#fcdb05'}} />,
    actions: [{
      title: "Get Recipes",
      color: 'secondary',
      variant: 'outlined',
      handler: (navigate) => navigate('new'),
    }]
  }

  const APP_3: IFeature = {
    title: 'Searchbot',
    description: "Website search your customers will love. Answer questions, surface hidden content and analyse customer intent.",
    image: 'https://tryhelix.ai/assets/img/FGesgz7rGY-900.webp',
    // icon: <ChatIcon sx={{color: '#fcdb05'}} />,
    actions: [{
      title: "Create Bot",
      color: 'secondary',
      variant: 'outlined',
      handler: (navigate) => navigate('new'),
    }]
  }

  return (
    <>
      <StoreFeatureSection
        title="Your Apps"
        features={[
          APP_1,
          APP_2,
          APP_3,
        ]}
        sx={{
          mb: 4,
        }}
      />

      <StoreFeatureSection
        title="Featured Apps"
        features={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>

      <StoreFeatureSection
        title="API Integrations"
        features={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>

      <StoreFeatureSection
        title="GPTScript Demos"
        features={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>


      <StoreFeatureSection
        title="Fine tuned image models"
        features={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>


      <StoreFeatureSection
        title="Fine tuned text models"
        features={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>



      <StoreFeatureSection
        title="RAG enabled apps"
        features={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>


    </>
  )
}

export default StoreFeatureGrid