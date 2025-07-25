import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import TextField from '@mui/material/TextField'
import IconButton from '@mui/material/IconButton'
import ClearIcon from '@mui/icons-material/Clear'

import Page from '../../components/system/Page'
import LLMCallsTable from '../../components/dashboard/LLMCallsTable'
import useRouter from '../../hooks/useRouter'

const AdminLLMCalls: FC = () => {
  const router = useRouter()
  const [sessionFilter, setSessionFilter] = useState('')

  const { filter_sessions } = router.params

  useEffect(() => {
    if (filter_sessions) {
      setSessionFilter(filter_sessions)
    }
  }, [filter_sessions])

  const handleFilterChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newFilter = event.target.value
    setSessionFilter(newFilter)
    if (newFilter) {
      router.setParams({ filter_sessions: newFilter })
    } else {
      router.removeParams(['filter_sessions'])
    }
  }

  const clearFilter = () => {
    setSessionFilter('')
    router.removeParams(['filter_sessions'])
  }

  return (
    <Page breadcrumbTitle="LLM Calls">
      <Container maxWidth="xl" sx={{ mt: 2, height: 'calc(100% - 50px)' }}>
        <Box
          sx={{
            width: '100%',
            height: 'calc(100vh - 150px)',
            overflow: 'auto',
          }}
        >
          <Box sx={{ mb: 2, display: 'flex', alignItems: 'center' }}>
            <TextField
              label="Filter by Session ID"
              variant="outlined"
              value={sessionFilter}
              onChange={handleFilterChange}
              sx={{ flexGrow: 1, mr: 1 }}
            />
            {sessionFilter && (
              <IconButton onClick={clearFilter} size="small">
                <ClearIcon />
              </IconButton>
            )}
          </Box>
          <LLMCallsTable sessionFilter={sessionFilter} />
        </Box>
      </Container>
    </Page>
  )
}

export default AdminLLMCalls 