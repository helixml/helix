import React, { FC, useCallback, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'

import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'
import ToolsGrid from '../components/datagrid/Tools'
import CreateToolWindow from '../components/tools/CreateToolWindow'

import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'
import { IAssistant, IOwnerType, IToolType, IToolConfig } from '../types'

const MY_FIXTURE_ASSISTANTS: IAssistant[] = [
  {
    id: '1',
    created: '2023-01-01T00:00:00Z',
    updated: '2023-01-01T00:00:00Z',
    owner: 'user123',
    owner_type: 'user' as IOwnerType,
    name: 'test1',
    description: 'This is a test assistant',
    tool_type: 'function' as IToolType,
    config: {
      api: {
          url: '', // Provide the actual URL
          schema: '', // Provide the actual schema
          actions: [], // Provide the actual actions array
          headers: {}, // Provide the actual headers object
          query: {}, // Provide the actual query object
      } // Do not cast here, let TypeScript infer the type
  },
}
]

const Tools: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const { navigate } = useRouter()

  const [addingTool, setAddingTool] = useState(false)

  const onCreateTool = useCallback(async (url: string, schema: string) => {
    // Simulate tool creation using fixture data
    const newTool = MY_FIXTURE_ASSISTANTS[0] // Replace with logic to select/create a fixture tool
    setAddingTool(false)
    snackbar.success('Tool created')
    navigate('tool', {
      tool_id: newTool.id,
    })
  }, [navigate, snackbar])

  const onEditTool = useCallback((tool: IAssistant) => {
    navigate('tool', {
      tool_id: tool.id,
    })
  }, [navigate])

  const onDeleteTool = useCallback((tool: IAssistant) => {
    // Handle deletion using fixture data
  }, [])

  useEffect(() => {
    if (!account.user) return
    // Load fixture data instead of API call
  }, [account.user])

  if (!account.user) return null

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
          }}
        >
          <DataGridWithFilters
            filters={
              <Box
                sx={{
                  width: '100%',
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                }}
              >
                <Button
                  sx={{
                    width: '100%',
                  }}
                  variant="contained"
                  color="secondary"
                  endIcon={<AddIcon />}
                  onClick={() => {
                    setAddingTool(true)
                  }}
                >
                  Create Tool
                </Button>
              </Box>
            }
            datagrid={
              <ToolsGrid
                data={MY_FIXTURE_ASSISTANTS}
                onEdit={onEditTool}
                onDelete={onDeleteTool}
              />
            }
          />
        </Box>
      </Container>
      {
        addingTool && (
          <CreateToolWindow
            onCreate={onCreateTool}
            onCancel={() => setAddingTool(false)}
          />
        )
      }
    </>
  )
}

export default Tools