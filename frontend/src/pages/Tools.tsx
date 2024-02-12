import React, { FC, useCallback, useEffect, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'

import DataGridWithFilters from '../components/datagrid/DataGridWithFilters'
import ToolsGrid from '../components/datagrid/Tools'
import CreateToolWindow from '../components/tools/CreateToolWindow'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useLayout from '../hooks/useLayout'
import useTools from '../hooks/useTools'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'

import {
  ITool,
} from '../types'

const Tools: FC = () => {
  const account = useAccount()
  const tools = useTools()
  const layout = useLayout()
  const snackbar = useSnackbar()
  const {
    navigate,
  } = useRouter()

  const [ addingTool, setAddingTool ] = useState(false)
  const [ deletingTool, setDeletingTool ] = useState<ITool>()

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

  const onEditTool = useCallback((tool: ITool) => {
    navigate('tool', {
      tool_id: tool.id,
    })
  }, [])

  const onDeleteTool = useCallback(async () => {
    if(!deletingTool) return
    const result = await tools.deleteTool(deletingTool.id)
    if(!result) return
    setDeletingTool(undefined)
    snackbar.success('Tool deleted')
  }, [
    deletingTool,
  ])

  useEffect(() => {
    if(!account.user) return
    tools.loadData()
  }, [
    account.user,
  ])

  useEffect(() => {
    layout.setToolbarRenderer(() => () => {
      return (
        <Button
          variant="contained"
          color="secondary"
          endIcon={<AddIcon />}
          onClick={ () => {
            setAddingTool(true)
          }}
        >
          Create Tool
        </Button>
      )
    })

    return () => layout.setToolbarRenderer(undefined)
  }, [])

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
        <ToolsGrid
          data={ tools.data }
          onEdit={ onEditTool }
          onDelete={ setDeletingTool }
        />
      </Container>
      {
        addingTool && (
          <CreateToolWindow
            onCreate={ onCreateTool }
            onCancel={ () => setAddingTool(false) }
          />
        )
      }
      {
        deletingTool && (
          <DeleteConfirmWindow
            title="this tool"
            onCancel={ () => setDeletingTool(undefined) }
            onSubmit={ onDeleteTool }
          />
        )
      }
    </>
  )
}

export default Tools