import React, { FC } from 'react'
import { useTheme } from '@mui/material/styles'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import HomeFeatureGrid from '../components/home/FeatureGrid'

import Row from '../components/widgets/Row'
import Cell from '../components/widgets/Cell'

const Home: FC = () => {
  const theme = useTheme()
  const isLight = theme.palette.mode === 'light'
  return (
    <>
      <Container
        maxWidth="xl"
        sx={{
          // mt: 12,
          pt: 3,
          height: 'calc(100% - 100px)',
        }}
      >
        <Box
          sx={{
            mb: 4,
          }}
        >
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
                  <Typography variant="h3" gutterBottom>
                    Welcome
                  </Typography>
                  <Typography variant="body1">
                    Please choose which feature you want to explore today
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
        <HomeFeatureGrid />
      </Container>
    </>
  )
}

export default Home