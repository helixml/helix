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
  APPS,
} from '../../fixtures'

const APP_1 = APPS[0]

import {
  IFeature,
} from '../../types'

const AppStoreCard: FC<{
  app: IApp,
}> = ({
  app,
}) => {
  const router = useRouter()

  return (
    <Card>
      <CardActionArea
        onClick={() => {
          router.navigate('new', {app_id: app.id})
        }}
      >
        {
          app.config.helix?.avatar && (
            <CardMedia
              sx={{ height: 140 }}
              image={ app.config.helix?.avatar }
              title={ app.config.helix?.name }
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
            {/* {
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
            } */}
            <Cell grow sx={{
              minHeight: '80px'
            }}>
              <Typography gutterBottom variant="h5" component="div">
                { app.config.helix?.name }
              </Typography>
              <Typography variant="body2" color="text.secondary">
                { app.config.helix?.description }
              </Typography>
            </Cell>
          </Row>
        </CardContent>
      </CardActionArea>
      <CardActions>
        <Button
          size="small"
          onClick={ () => {
            router.navigate('new', {app_id: app.id}) 
          }}
        >
          Launch
        </Button>
      </CardActions>
    </Card>
  )
}

const AppStoreSection: FC<{
  title: string,
  apps: IApp[],
  sx?: SxProps,
}> = ({
  title,
  apps,
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
          { apps.map((app, index) => (
            <Grid item xs={ 12 } sm={ 12 } md={ 6 } lg={ 4 } key={ index } sx={{ p: 0, m: 0 }}>
              <AppStoreCard
                app={ app }
              />
            </Grid>
          )) }
        </Grid>
      </Box>
    </Box>
  )
}


const AppStoreGrid: FC<{
  apps: IApp[]
}> = ({
  apps 
}) => {
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

    // TODO: make the create page into our "app launcher"
    router.navigate('create', {app_id: appID})
  }

  return (
    <>
      <AppStoreSection
        title="Your Apps"
        apps={[
          APP_1,
        ]}
        sx={{
          mb: 4,
        }}
      />

      <AppStoreSection
        title="Featured Apps"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>

      <AppStoreSection
        title="API Integrations"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>

      <AppStoreSection
        title="GPTScript Demos"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>


      <AppStoreSection
        title="Fine tuned image models"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>


      <AppStoreSection
        title="Fine tuned text models"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>



      <AppStoreSection
        title="RAG enabled apps"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
      />

      Coming soon.<br/><br/><br/>


    </>
  )
}

export default AppStoreGrid