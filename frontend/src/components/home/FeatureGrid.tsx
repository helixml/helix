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
import AppleIcon from '@mui/icons-material/Apple'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'

import {
  IFeature,
} from '../../types'

const CHAT_FEATURE: IFeature = {
  title: 'Chat with Helix',
  description: 'Everything you can do with ChatGPT, with open models.',
  image: '/img/servers.png',
  icon: <AppleIcon sx={{color: '#fff'}} />,
  actions: [{
    title: 'Chat',
    color: 'secondary',
    variant: 'outlined',
    handler: (navigate) => navigate('new'),
  }, {
    title: 'Docs',
    color: 'primary',
    variant: 'text',
    handler: () => {},
  }]
}

const IMAGE_GEN_FEATURE: IFeature = {
  title: 'Image Generation',
  description: 'Prompt an image model to generate drawings, photos and logos',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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

const APPS_FEATURE: IFeature = {
  title: 'Browse Apps',
  description: 'AI Assistants and apps that you or other users have created',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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
  description: 'Train your own LLM on the knowledge in your text (coming soon: finetune on style or complex tools use)',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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

const FINETUNE_IMAGES_FEATURE: IFeature = {
  title: 'Finetune on Images',
  description: 'Train your own image model on the style of a set of images (e.g. visual style, objects, faces)',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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

const JS_APP_FEATURE: IFeature = {
  title: 'JS AI-Powered Apps',
  description: 'Create a Javascript Frontend App with AI powered UI or embed a chatbot',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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

const API_FEATURE: IFeature = {
  title: 'Integrate LLM with REST API',
  description: 'Provide a Swagger/OpenAPI spec and enable users to call the API via natural language',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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

const GPTSCRIPT_FEATURE: IFeature = {
  title: 'GPTScript',
  description: 'Run GPTScripts',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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

const DASHBOARD_FEATURE: IFeature = {
  title: 'Admin Dashboard',
  description: 'View connected GPU runners and check queue length',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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

const USERS_FEATURE: IFeature = {
  title: 'Users',
  description: 'Show Users',
  image: '/img/servers.png',
  actions: [{
    title: 'Chat',
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

const SETTINGS_FEATURE: IFeature = {
  title: 'Settings',
  description: 'Show Settings (coming soon)',
  image: '/img/servers.png',
  disabled: true,
  actions: [{
    title: 'Chat',
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
            <Cell grow>
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
          JS_APP_FEATURE,
          API_FEATURE,
          GPTSCRIPT_FEATURE,
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