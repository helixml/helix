import React, { FC, useMemo } from 'react'
import { SxProps } from '@mui/system'
import Divider from '@mui/material/Divider'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Card from '@mui/material/Card'
import CardActions from '@mui/material/CardActions'
import CardActionArea from '@mui/material/CardActionArea'
import CardContent from '@mui/material/CardContent'
import CardMedia from '@mui/material/CardMedia'
import Avatar from '@mui/material/Avatar'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'

import useRouter from '../../hooks/useRouter'

import {
  IApp,
} from '../../types'

import {
  getAppImage,
  getAppAvatar,
  getAppName,
  getAppDescription,
} from '../../utils/apps'

const AppStoreCard: FC<{
  app: IApp,
}> = ({
  app,
}) => {
  const router = useRouter()

  const avatar = getAppAvatar(app)
  const image = getAppImage(app)
  const name = getAppName(app)
  const description = getAppDescription(app)

  return (
    <Card>
      <CardActionArea
        onClick={() => {
          router.navigate('new', {app_id: app.id})
        }}
      >
        {
          image && (
            <CardMedia
              sx={{ height: 140 }}
              image={ image }
              title={ name }
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
              avatar && (
                <Cell
                  sx={{
                    mr: 2,
                    pt: 1,
                  }}
                >
                  <Avatar
                    src={ avatar }
                  />
                </Cell>
              )
            }
            <Cell grow sx={{
              minHeight: '80px'
            }}>
              <Typography gutterBottom variant="h5" component="div">
                { name }
              </Typography>
              <Typography variant="body2" color="text.secondary">
                { description }
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
  const router = useRouter()

  const launchApp = async (appID: string) => {
    router.navigate('create', {app_id: appID})
  }

  const globalApps = useMemo(() => {
    return apps.filter(app => app.global)
  }, [
    apps,
  ])

  const userApps = useMemo(() => {
    return apps.filter(app => app.global ? false : true)
  }, [
    apps,
  ])

  return (
    <>
      <AppStoreSection
        title="Your Apps"
        apps={ userApps }
        sx={{
          mb: 4,
        }}
      />

      <AppStoreSection
        title="Featured Apps"
        apps={ globalApps }
        sx={{
          mb: 4,
        }}
      />

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