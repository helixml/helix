import React, { FC, useCallback, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'

import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'
import ToolsGrid from '../components/datagrid/Tools'
import CreateToolWindow from '../components/tools/CreateToolWindow'

import useTools from '../hooks/useTools'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'

import {
  IAssistant,
} from '../types'


const Tools: FC = () => {
  const account = useAccount()
  const tools = useTools()
  const snackbar = useSnackbar()
  const {
    navigate,
  } = useRouter()

  const [ addingTool, setAddingTool ] = useState(false)

  const onCreateTool = useCallback(async (url: string, schema: string) => {
    const newTool = await tools.createTool(url, schema)
    if(!newTool) return
    setAddingTool(false)
    snackbar.success('Tool created')
    navigate('tool', {
      tool_id: newTool.id,
    })
  }, [
    tools.createTool,
  ])

  const onEditTool = useCallback((tool: IAssistant) => {
    navigate('tool', {
      tool_id: tool.id,
    })
  }, [])

  const onDeleteTool = useCallback((tool: IAssistant) => {

  }, [])

  useEffect(() => {
    if(!account.user) return
    tools.loadData()
  }, [
    account.user,
  ])

  if(!account.user) return null

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
                  onClick={ () => {
                    setAddingTool(true)
                  }}
                >
                  Create Tool
                </Button>
              </Box>
            }
            datagrid={
              <ToolsGrid
                data={ tools.data }
                onEdit={ onEditTool }
                onDelete={ onDeleteTool }
              />
            }
          />
        </Box>
      </Container>
      {
        addingTool && (
          <CreateToolWindow
            onCreate={ onCreateTool }
            onCancel={ () => setAddingTool(false) }
          />
        )
      }
    </>
  )
}

export default Tools