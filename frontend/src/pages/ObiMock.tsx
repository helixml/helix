import React, { FC, useCallback, useEffect, useState, useMemo, useRef } from 'react'
import Typography from '@mui/material/Typography'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'

const ObiMock: FC = () => {
  return (
    <>
      <Container
        maxWidth="xl"
        sx={{
          mt: 12,
          height: 'calc(100% - 100px)',
        }}
      >
        <Box
          sx={{
            height: 'calc(100vh - 100px)',
            width: '100%',
            flexGrow: 1,
            p: 2,
          }}
        >
          <Typography variant="h2">Hello World ORANGES</Typography>
        </Box>
      </Container>
    </>
  )
}

export default ObiMock