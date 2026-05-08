import React, { Suspense } from 'react'
import { Box, CircularProgress } from '@mui/material'

export type { IDataGrid2_Column, IDataGrid2_Column_Render_Params } from './DataGridImpl'

const Impl = React.lazy(() => import('./DataGridImpl'))

export default function DataGrid(props: any) {
  return (
    <Suspense fallback={
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', minHeight: 400 }}>
        <CircularProgress size={32} />
      </Box>
    }>
      <Impl {...props} />
    </Suspense>
  )
}
