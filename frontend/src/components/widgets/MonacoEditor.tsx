import React, { Suspense } from 'react'
import { Box, CircularProgress } from '@mui/material'

const Impl = React.lazy(() => import('./MonacoEditorImpl'))

export default function MonacoEditor(props: any) {
  return (
    <Suspense fallback={
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 200, border: '1px solid #303047', borderRadius: 1 }}>
        <CircularProgress size={24} />
      </Box>
    }>
      <Impl {...props} />
    </Suspense>
  )
}
