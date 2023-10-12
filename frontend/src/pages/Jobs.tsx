import React, { FC, useContext } from 'react'
import { navigate } from 'hookrouter'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'

import AddIcon from '@mui/icons-material/Add'
import { AccountContext } from '../contexts/account'
import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'
import JobGrid from '../components/datagrid/Job'

const Jobs: FC = () => {
  const account = useContext(AccountContext)
  if(!account.user) return null
  return (
    <DataGridWithFilters
      filters={
        <Box
          sx={{
            width: '100%',
          }}
        >
          <Button
            sx={{
              width: '100%',
            }}
            variant="contained"
            color="secondary"
            endIcon={<AddIcon />}
            onClick={ () => {
              navigate('/')
            }}
          >
            Create Job
          </Button>
        </Box>
      }
      datagrid={
        <JobGrid
          jobs={ account.jobs }
          loading={ account.initialized ? false : true }
        />
      }
    />
  )
}

export default Jobs