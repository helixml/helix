import React, { FC, useContext } from 'react'
import Box from '@mui/material/Box'

import { AccountContext } from '../contexts/account'
import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'

const Account: FC = () => {
  const account = useContext(AccountContext)
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