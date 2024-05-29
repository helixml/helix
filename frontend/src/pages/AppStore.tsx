import React, { FC, useEffect } from 'react'
import { useTheme } from '@mui/material/styles'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'

import AppStoreGrid from '../components/appstore/AppStoreGrid'
import Page from '../components/system/Page'
import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

import useApps from '../hooks/useApps'

import useIsBigScreen from '../hooks/useIsBigScreen'

const AppStore: FC = () => {
  const apps = useApps()
  const theme = useTheme()
  const isLight = theme.palette.mode === 'light'
  const isBigScreen = useIsBigScreen()
  
  useEffect(() => {
    apps.loadData()
  }, [])
  
  return (
    <Page
      showTopbar={ true }
      breadcrumbTitle='App Store'
    >
      <Container
        maxWidth="xl"
        sx={{
          py: 3,
        }}
      >
        <Box>
          <Grid container spacing={ 2 }>
            <Grid item xs={ 12 } sm={ 12 } md={ 12 } lg={ 6 }>
              <Row
                sx={{
                  height: '100%',
                  display: 'flex',
                  flexDirection: 'row',
                  alignItems: 'center',
                }}
              >
                <Cell>
                  <Box
                    component="img"
                    src="/img/logo.png"
                    sx={{
                      width: 100,
                    }}
                  />
                </Cell>
                <Cell
                  sx={{
                    ml: 4,
                  }}
                >
                  <Typography variant={ isBigScreen ? 'h3' : 'h5' } gutterBottom>
                    App Store
                  </Typography>
                  <Typography variant="body1">
                    Take your pick of AI-powered apps you or others have created!
                  </Typography>
                </Cell>
              </Row>
            </Grid>
            <Grid item xs={ 12 } sm={ 12 } md={ 12 } lg={ 6 }>
              <Box
                sx={{
                  textAlign: 'center',
                }}
              >
                <Box
                  component="img"
                  src={ isLight ? '/img/nebula-light.png' : '/img/nebula-dark.png' }
                  sx={{
                    width: '100%',
                    maxWidth: '800px'
                  }}
                />
              </Box>
            </Grid>
          </Grid>
        </Box>
        <AppStoreGrid
          apps={ apps.data }
        />
      </Container>
    </Page>
  )
}

export default AppStore