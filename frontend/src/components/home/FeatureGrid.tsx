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

import {
  IFeature,
} from '../../types'

const CHAT_FEATURE: IFeature = {
  title: 'Chat with Helix',
  description: 'Everything you can do with ChatGPT, with open models',
  // image: '/img/servers.png',
  icon: <ChatIcon sx={{color: '#fff'}} />,
  actions: [{
    title: 'Chat',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => navigate('new'),
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => window.open("https://docs.helix.ml/helix/using-helix/text-inference/"),
  }]
}

const IMAGE_GEN_FEATURE: IFeature = {
  title: 'Image Generation',
  description: 'Prompt an image model to generate photos, illustrations or logos',
  // image: '/img/servers.png',
  icon: <ImageIcon sx={{color: '#fff'}} />,
  actions: [{
    title: 'Create',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => {navigate('new', {mode:"inference", type:"image"})},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => window.open("https://docs.helix.ml/helix/using-helix/image-inference/"),
  }]
}

const APPS_FEATURE: IFeature = {
  title: 'Browse Apps',
  description: 'AI Assistants and apps that you or other users have created (coming soon)',
  disabled: true,
  icon: <AppsIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'Browse',
    color: 'secondary',
    variant: 'outlined',
    handler: () => {},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => {},
  }]
}

const RAG_FEATURE: IFeature = {
  title: 'RAG over Documents',
  description: 'Upload documents and index them, then chat to your documents',
  disabled: true,
  // icon: <DocumentScannerIcon sx={{color: '#fff'}} />,
  icon: <PlagiarismIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'Upload',
    color: 'secondary',
    variant: 'outlined',
    handler: () => {},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => {},
  }]
}

const FINETUNE_TEXT_FEATURE: IFeature = {
  title: 'Finetune on Text',
  description: 'Train your own LLM on knowledge in your docs (soon: style and complex tools)',
  icon: <ModelTrainingIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'Finetune',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => {navigate('new', {mode:"finetune", type:"text"})},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => window.open("https://docs.helix.ml/helix/using-helix/text-finetuning/"),
  }]
}

// { session.mode == SESSION_MODE_INFERENCE &&  session.type == SESSION_TYPE_IMAGE && <ImageIcon color="primary" /> }
// { session.mode == SESSION_MODE_INFERENCE && session.type == SESSION_TYPE_TEXT && <DescriptionIcon color="primary" /> }
// { session.mode == SESSION_MODE_FINETUNE &&  session.type == SESSION_TYPE_IMAGE && <PermMediaIcon color="primary" /> }
// { session.mode == SESSION_MODE_FINETUNE && session.type == SESSION_TYPE_TEXT && <ModelTrainingIcon color="primary" /> }
const FINETUNE_IMAGES_FEATURE: IFeature = {
  title: 'Finetune on Images',
  description: 'Train your own image model on the style of a set of images (e.g. style, objects)',
  icon: <PermMediaIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'Finetune',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => {navigate('new', {mode:"finetune", type:"image"})},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => window.open("https://docs.helix.ml/helix/using-helix/image-finetuning/"),
  }]
}

const JS_APP_FEATURE: IFeature = {
  title: 'AI-Powered JS Apps',
  description: 'Create a Frontend App with AI powered UI or embed a chatbot, deploy with git push',
  icon: <WebIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'Apps',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => {navigate('apps')},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => window.open("https://docs.helix.ml/helix/develop/apps/"),
  }]
}

const API_FEATURE: IFeature = {
  title: 'API Integration with LLM',
  description: 'Enable natural language interface to REST APIs with Swagger/OpenAPI specs',
  icon: <ApiIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'API Tools',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => {navigate('tools')},
  }, {
    title: 'Docs (coming soon)',
    color: 'primary',
    variant: 'text',
    handler: () => {},
  }]
}

const GPTSCRIPT_FEATURE: IFeature = {
  title: 'GPTScript on the Server',
  description: 'Write backend logic in natural language, integrate with frontends or via API',
  icon: <img src="/img/gptscript.png" style={{width:"30px", filter: "grayscale(1)"}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'GPTScript Tools',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => {navigate('tools')},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => window.open("https://docs.helix.ml/helix/develop/apps/"),
  }]
}

const DASHBOARD_FEATURE: IFeature = {
  title: 'Dashboard',
  description: 'View connected GPU runners and check queue length',
  icon: <DashboardIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'Dashboard',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => {navigate('dashboard')},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => window.open("https://docs.helix.ml/helix/private-deployment/"),
  }]
}

const USERS_FEATURE: IFeature = {
  title: 'User Management',
  description: 'Manage Users, Auth, SSO and OAuth, ActiveDirectory or LDAP integration',
  icon: <GroupIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  actions: [{
    title: 'Keycloak',
    color: 'secondary',
    variant: 'outlined',
    handler: () => {window.open("/auth")},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => window.open("https://www.keycloak.org/docs/latest/server_admin/index.html"),
  }]
}

const SETTINGS_FEATURE: IFeature = {
  title: 'Settings',
  description: 'Manage which models are enabled and other configuration (coming soon)',
  icon: <SettingsIcon sx={{color: '#fff'}} />,
  // image: '/img/servers.png',
  disabled: true,
  actions: [{
    title: 'Settings',
    color: 'secondary',
    variant: 'outlined',
    handler: () => {},
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => {},
  }]
}

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
                      backgroundColor: 'primary.main',
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

const HomeFeatureSection: FC<{
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


const HomeFeatureGrid: FC = ({
  
}) => {

  const account = useAccount()

  return (
    <>
      <HomeFeatureSection
        title="Use"
        features={[
          CHAT_FEATURE,
          IMAGE_GEN_FEATURE,
          APPS_FEATURE,
        ]}
        sx={{
          mb: 4,
        }}
      />

      <HomeFeatureSection
        title="Customize"
        features={[
          RAG_FEATURE,
          FINETUNE_TEXT_FEATURE,
          FINETUNE_IMAGES_FEATURE,
        ]}
        sx={{
          mb: 4,
        }}
      />

      <HomeFeatureSection
        title="Develop"
        features={[
          API_FEATURE,
          GPTSCRIPT_FEATURE,
          JS_APP_FEATURE,
        ]}
        sx={{
          mb: 4,
        }}
      />

      {
        account.admin && (
          <HomeFeatureSection
            title="Admin"
            features={[
              DASHBOARD_FEATURE,
              USERS_FEATURE,
              SETTINGS_FEATURE,
            ]}
            sx={{
              pb: 4,
            }}
          />
        )
      }
    </>
  )
}

export default HomeFeatureGrid