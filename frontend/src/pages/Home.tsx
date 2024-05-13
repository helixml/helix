import React, { FC } from 'react'
import Container from '@mui/material/Container'
import HomeFeatureGrid from '../components/home/FeatureGrid'

const Home: FC = () => {

  return (
    <>
      <Container
        maxWidth="xl"
        sx={{
          mt: 12,
          pt: 3,
          height: 'calc(100% - 100px)',
        }}
      >
        <HomeFeatureGrid />
      </Container>
    </>
  )
}

export default Home