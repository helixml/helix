import React, { FC, useCallback, useEffect, useState, useMemo } from 'react'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import Container from '@mui/material/Container'
import Grid from '@mui/material/Grid'

import Window from '../components/widgets/Window'
import StringMapEditor from '../components/widgets/StringMapEditor'
import ClickLink from '../components/widgets/ClickLink'
import ToolActionsGrid from '../components/datagrid/ToolActions'

import useTools from '../hooks/useTools'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useRouter from '../hooks/useRouter'

const Tool: FC = () => {
  const account = useAccount()
  const tools = useTools()
  const snackbar = useSnackbar()
  const {
    params,
  } = useRouter()

  const [ name, setName ] = useState('')
  const [ description, setDescription ] = useState('')
  const [ url, setURL ] = useState('')
  const [ headers, setHeaders ] = useState<Record<string, string>>({})
  const [ query, setQuery ] = useState<Record<string, string>>({})
  const [ schema, setSchema ] = useState('')
  const [ showErrors, setShowErrors ] = useState(false)
  const [ showBigSchema, setShowBigSchema ] = useState(false)

  const tool = useMemo(() => {
    return tools.data.find((tool) => tool.id === params.tool_id)
  }, [
    tools.data,
    params,
  ])

  useEffect(() => {
    if(!account.user) return
    tools.loadData()
  }, [
    account.user,
  ])

  useEffect(() => {
    if(!tool) return
    setName(tool.name)
    setDescription(tool.description)
    setURL(tool.config.api.url)
    setSchema(tool.config.api.schema)
    setHeaders(tool.config.api.headers)
    setQuery(tool.config.api.query)
  }, [
    tool,
  ])

  if(!account.user) return null
  if(!tool) return null

  console.log('--------------------------------------------')
  console.dir(tool)

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
            p: 2,
          }}
        >
          <Grid container spacing={2}>
            <Grid item xs={ 12 } md={ 6 }>
              <Typography variant="h6" sx={{mb: 1.5}}>
                Settings
              </Typography>
              <TextField
                sx={{
                  mb: 3,
                }}
                error={ showErrors && !name }
                value={ name }
                onChange={(e) => setName(e.target.value)}
                fullWidth
                label="Name"
                helperText="Please enter a Name"
              />
              <TextField
                sx={{
                  mb: 1,
                }}
                value={ description }
                onChange={(e) => setDescription(e.target.value)}
                fullWidth
                multiline
                rows={2}
                label="Description"
                helperText="Enter a description of this tool (optional)"
              />
              <Typography variant="h6" sx={{mb: 1}}>
                API
              </Typography>
              <TextField
                sx={{
                  mb: 3,
                }}
                error={ showErrors && !url }
                value={ url }
                onChange={(e) => setURL(e.target.value)}
                fullWidth
                label="API URL"
                placeholder="Enter API URL"
                helperText={ showErrors && !url ? "Please enter a URL" : "URL should be in the format: https://api.example.com/v1/endpoint" }
              />
              <TextField
                error={ showErrors && !schema }
                value={ schema }
                onChange={(e) => setSchema(e.target.value)}
                fullWidth
                multiline
                rows={3}
                label="Enter openAI schema"
                helperText={ showErrors && !schema ? "Please enter a schema" : "base64 encoded or escaped JSON/yaml" }
              />
              <Box
                sx={{
                  textAlign: 'right',
                  mb: 1,
                }}
              >
                <ClickLink
                  onClick={ () => setShowBigSchema(true) }
                >
                  expand
                </ClickLink>
              </Box>
              <Typography variant="h6" sx={{mb: 1}}>
                Authentication
              </Typography>
              <Box
                sx={{
                  mb: 3,
                }}
              >
                <Typography variant="subtitle1" sx={{mb: 1}}>
                  Headers
                </Typography>
                <StringMapEditor
                  data={ headers }
                  onChange={ setHeaders }
                />
              </Box>
              <Box
                sx={{
                  mb: 3,
                }}
              >
                <Typography variant="subtitle1" sx={{mb: 1}}>
                  Query Params
                </Typography>
                <StringMapEditor
                  data={ query }
                  onChange={ setQuery }
                />
              </Box>
              <Box
                sx={{
                  textAlign: 'right',
                }}
              >
                <Button
                  sx={{
                    mr: 2,
                  }}
                  type="button"
                  color="primary"
                  variant="outlined"
                  onClick={ () => {} }
                >
                  Cancel
                </Button>
                <Button
                  sx={{
                    mr: 2,
                  }}
                  type="button"
                  color="secondary"
                  variant="contained"
                  onClick={ () => {} }
                >
                  Save
                </Button>
              </Box>
            </Grid>
            <Grid item xs={ 12 } md={ 6 }>
              <Typography variant="h6" sx={{mb: 1}}>
                Actions
              </Typography>
              <ToolActionsGrid
                data={ tool.config.api.actions }
              />
            </Grid>
          </Grid>
        </Box>
      </Container>
      {
        showBigSchema && (
          <Window
            title="Schema"
            fullHeight
            size="lg"
            open
            withCancel
            cancelTitle="Close"
            onCancel={() => setShowBigSchema(false)}
          >
            <Box
              sx={{
                p: 2,
                height: '100%',
              }}
            >
              <TextField
                error={showErrors && !schema}
                value={schema}
                onChange={(e) => setSchema(e.target.value)}
                fullWidth
                multiline
                label="Enter openAI schema"
                helperText={showErrors && !schema ? "Please enter a schema" : "base64 encoded or escaped JSON/yaml"}
                sx={{ height: '100%' }} // Set the height to '100%'
              />
            </Box>
          </Window>
        )
      }
    </>
  )
}

export default Tool