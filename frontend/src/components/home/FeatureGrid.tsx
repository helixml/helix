import React, { FC, useCallback, MouseEvent } from 'react'
import { SxProps } from '@mui/system'
import Divider from '@mui/material/Divider'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Container from '@mui/material/Container'
import Card from '@mui/material/Card'
import CardActions from '@mui/material/CardActions'
import CardContent from '@mui/material/CardContent'
import CardMedia from '@mui/material/CardMedia'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'

import {
  IFeature,
} from '../../types'


const CHAT_FEATURE: IFeature = {
  title: 'Chat',
  description: 'Talk to Helix',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const IMAGE_GEN_FEATURE: IFeature = {
  title: 'Image Gen',
  description: 'Generate Images',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const APPS_FEATURE: IFeature = {
  title: 'Apps',
  description: 'View Apps',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const RAG_FEATURE: IFeature = {
  title: 'RAG',
  description: 'Add your own documents',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const FINETUNE_TEXT_FEATURE: IFeature = {
  title: 'Finetune Text',
  description: 'Finetune on text',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const FINETUNE_IMAGES_FEATURE: IFeature = {
  title: 'Finetune Images',
  description: 'Finetune on images',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const JS_APP_FEATURE: IFeature = {
  title: 'JS App',
  description: 'Create a Javascript AI App',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const API_FEATURE: IFeature = {
  title: 'Integrate w/ API',
  description: 'Use the REST API',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const GPTSCRIPT_FEATURE: IFeature = {
  title: 'GPTScript',
  description: 'Run GPTScripts',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const DASHBOARD_FEATURE: IFeature = {
  title: 'Dashboard',
  description: 'Show the platform dashboard',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const USERS_FEATURE: IFeature = {
  title: 'Users',
  description: 'Show Users',
  image: '/img/servers.png',
  openAction: () => {},
  docsAction: () => {},
}

const SETTINGS_FEATURE: IFeature = {
  title: 'Settings',
  description: 'Show Settings (coming soon)',
  image: '/img/servers.png',
  disabled: true,
  openAction: () => {},
  docsAction: () => {},
}

const HomeFeatureCard: FC<{
  feature: IFeature,
}> = ({
  feature,
}) => {
  const router = useRouter()
  return (
    <Card sx={{ maxWidth: 345 }}>
      <CardMedia
        sx={{ height: 140 }}
        image={ feature.image }
        title={ feature.title }
      />
      <CardContent>
        <Typography gutterBottom variant="h5" component="div">
          { feature.title }
        </Typography>
        <Typography variant="body2" color="text.secondary">
          { feature.description }
        </Typography>
      </CardContent>
      <CardActions>
        <Row>
        <Cell grow>
          <Button
            size="small"
            sx={{
              color: 'secondary.main',
            }}
            onClick={ () => feature.docsAction(router.navigate) }
          >
            Docs
          </Button>
        </Cell>
        <Cell>
          <Button
            size="small"
            variant="outlined"
            sx={{
              color: 'secondary.main',
              borderColor: 'secondary.main',
            }}
            onClick={ () => feature.openAction(router.navigate) }
          >
            View
          </Button>
        </Cell>
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
      <Grid container spacing={ 2 }>
        { features.map((feature, index) => (
          <Grid item xs={ 12 } sm={ 6 } md={ 4 } key={ index }>
            <HomeFeatureCard
              feature={ feature }
            />
          </Grid>
        )) }
      </Grid>
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
          />
        )
      }
    </>
  )
}

export default HomeFeatureGrid