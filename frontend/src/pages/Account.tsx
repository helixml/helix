import React, { FC } from 'react'
import Box from '@mui/material/Box'

import useAccount from '../hooks/useAccount'
import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'

const Account: FC = () => {
  const account = useAccount()
  if(!account.user) return null
  return (
    <DataGridWithFilters
      datagrid={
        <Box>
          account page
        </Box>
      }
    />
  )
}

export default Account