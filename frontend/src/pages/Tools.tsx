import React, { FC, useCallback, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'

import Page from '../components/system/Page'
import CreateToolWindow from '../components/tools/CreateToolWindow'
import CreateGPTScriptToolWindow from '../components/tools/CreateGPTScriptToolWindow'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import ToolsTable from '../components/tools/ToolsTable'
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

  const [ addingTool, setAddingApiTool ] = useState(false)
  const [ addingGptScriptTool, setAddingGptScriptTool ] = useState(false)
  const [ deletingTool, setDeletingTool ] = useState<ITool>()

  const onCreateTool = useCallback(async (url: string, schema: string) => {
    const newTool = await tools.createTool('', 'api', '', {
      api: {
        url,
        schema,
        actions: [],
        headers: {},
        query: {},
      }
    })
    if(!newTool) return
    setAddingApiTool(false)
    snackbar.success('API tool created')
    navigate('tool', {
      tool_id: newTool.id,
    })
  }, [
    tools.createTool,
  ])

  const onCreateGptScriptTool = useCallback(async (name: string, description: string, script: string) => {
    console.log(name, description, script)
    const newTool = await tools.createTool(name, 'gptscript', description, {
      gptscript: {
        script,        
      }
    })
    if(!newTool) return
    setAddingApiTool(false)
    snackbar.success('GPTScript tool created')
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

  if(!account.user) return null

  return (
    <Page
      topbarTitle="Tools"
      topbarContent={(
        <div>
          <Button
            variant="contained"
            color="secondary"
            endIcon={<AddIcon />}
            sx={{
              mr: 2,
            }}
            onClick={ () => {
              setAddingGptScriptTool(true)
            }}
            >
              New GPTScript tool
          </Button>

          <Button
              variant="contained"
              color="secondary"
              endIcon={<AddIcon />}
              onClick={ () => {
                setAddingApiTool(true)
              }}
            >
              New API tool
          </Button>
        </div>
      )}
    >
      <Container
        maxWidth="xl"
        sx={{
          mb: 4,
        }}
      >
        <ToolsTable
          data={ tools.data }
          onEdit={ onEditTool }
          onDelete={ setDeletingTool }
        />
      </Container>
      {
        addingTool && (
          <CreateToolWindow
            onCreate={ onCreateTool }
            onCancel={ () => setAddingApiTool(false) }
          />
        )
      }
      {
        addingGptScriptTool && (
          <CreateGPTScriptToolWindow
            onCreate={ onCreateGptScriptTool }
            onCancel={ () => setAddingGptScriptTool(false) }
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
    </Page>
  )
}

export default Tools