import React, { Suspense } from 'react'
import { Box, CircularProgress, Typography } from '@mui/material'

export type { ToPDFProps } from './ToPDFImpl'

const Impl = React.lazy(() => import('./ToPDFImpl'))

export default function ToPDF(props: any) {
  return (
    <Suspense fallback={
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100%', minHeight: 200 }}>
        <CircularProgress size={32} />
        <Typography sx={{ ml: 2 }}>Loading PDF editor...</Typography>
      </Box>
    }>
      <Impl {...props} />
    </Suspense>
  )
}
