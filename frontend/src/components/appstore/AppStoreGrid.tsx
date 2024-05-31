import React, { FC, useMemo } from 'react'
import { SxProps } from '@mui/system'
import Divider from '@mui/material/Divider'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'

import AppStoreCard from './AppStoreCard'

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

const AppStoreSection: FC<{
  title: string,
  apps: IApp[],
  sx?: SxProps,
  onClick: (id: string) => void,
}> = ({
  title,
  apps,
  sx = {},
  onClick,
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
          {
            apps.map((app, index) => {
              return (
                <Grid item xs={ 12 } sm={ 12 } md={ 6 } lg={ 4 } key={ index } sx={{ p: 0, m: 0 }}>
                  <AppStoreCard
                    avatar={ getAppAvatar(app) }
                    image={ getAppImage(app) }
                    name={ getAppName(app) }
                    description={ getAppDescription(app) }
                    onClick={ () => onClick(app.id) }
                  />
                </Grid>
              )
            })
          }
        </Grid>
      </Box>
    </Box>
  )
}

const AppStoreGrid: FC<{
  apps: IApp[],
  onClick: (id: string) => void,
}> = ({
  apps,
  onClick,
}) => {
  const router = useRouter()

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
        onClick={ onClick }
      />

      <AppStoreSection
        title="Featured Apps"
        apps={ globalApps }
        sx={{
          mb: 4,
        }}
        onClick={ onClick }
      />

      <AppStoreSection
        title="API Integrations"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
        onClick={ onClick }
      />

      Coming soon.<br/><br/><br/>

      <AppStoreSection
        title="GPTScript Demos"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
        onClick={ onClick }
      />

      Coming soon.<br/><br/><br/>


      <AppStoreSection
        title="Fine tuned image models"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
        onClick={ onClick }
      />

      Coming soon.<br/><br/><br/>


      <AppStoreSection
        title="Fine tuned text models"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
        onClick={ onClick }
      />

      Coming soon.<br/><br/><br/>



      <AppStoreSection
        title="RAG enabled apps"
        apps={[
        ]}
        sx={{
          mb: 4,
        }}
        onClick={ onClick }
      />

      Coming soon.<br/><br/><br/>


    </>
  )
}

export default AppStoreGrid