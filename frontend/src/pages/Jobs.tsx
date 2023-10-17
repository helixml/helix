import React, { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'

import router from '../router'

import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'
import JobGrid from '../components/datagrid/Job'

import useAccount from '../hooks/useAccount'

const Jobs: FC = () => {
  const account = useAccount()
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
              router.navigate('/')
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