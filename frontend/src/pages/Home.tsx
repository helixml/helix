import React, { FC } from 'react'
import { useTheme } from '@mui/material/styles'
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
          <Row>
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
            <Cell
              grow
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
            </Cell>
          </Row>
        </Box>
        <HomeFeatureGrid />
      </Container>
    </>
  )
}

export default Home