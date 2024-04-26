import React, { FC, useState, SetStateAction, Dispatch } from 'react'
import Box from '@mui/material/Box'
import Tabs from '@mui/material/Tabs'
import Tab from '@mui/material/Tab'
import Typography from '@mui/material/Typography'
import MenuItem from '@mui/material/MenuItem'
import Grid from '@mui/material/Grid'
import Divider from '@mui/material/Divider'
import FormControl from '@mui/material/FormControl'
import FormGroup from '@mui/material/FormGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Checkbox from '@mui/material/Checkbox'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import TextField from '@mui/material/TextField'

import Window from '../widgets/Window'
import useAccount from '../../hooks/useAccount'
import useTools from '../../hooks/useTools'

import {
  ICreateSessionConfig,
} from '../../types'

import {
  ISessionMode,
  ISessionType,
  SESSION_MODE_FINETUNE,
  SESSION_TYPE_TEXT,
} from '../../types'

const CreateSettingsWindow: FC<{
  mode: ISessionMode,
  type: ISessionType,
  sessionConfig: ICreateSessionConfig,
  setSessionConfig: Dispatch<SetStateAction<ICreateSessionConfig>>,
  onClose: () => void,
}> = ({
  mode,
  type,
  sessionConfig,
  setSessionConfig,
  onClose,
}) => {
  const account = useAccount()
  const tools = useTools()
  const [activeSettingsTab, setActiveSettingsTab] = useState(0)

  const handleToolsCheckboxChange = (id: string, event: React.ChangeEvent<HTMLInputElement>) => {
    if(event.target.checked) {
      setSessionConfig(config => ({
        ...config,
        activeToolIDs: [ ...config.activeToolIDs, id ],
      }))
    } else {
      setSessionConfig(config => ({
        ...config,
        activeToolIDs: config.activeToolIDs.filter(toolId => toolId !== id)
      }))
    }
  }

  return (
    <Window
      open
      size="md"
      title="Session Settings"
      onCancel={ onClose }
      withCancel
      cancelTitle="Close"
    >
      <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
        <Tabs value={activeSettingsTab} onChange={(event: React.SyntheticEvent, newValue: number) => {
          setActiveSettingsTab(newValue)
        }}>
          {
            account.serverConfig.tools_enabled && (
              <Tab label="Active Tools" />
            )
          }
          {
            mode == SESSION_MODE_FINETUNE && account.admin && (
              <Tab label="Admin" />
            )
          }
        </Tabs>
      </Box>
      <Box>
        {
          account.serverConfig.tools_enabled && activeSettingsTab == 0 && (
            <Box sx={{ mt: 2 }}>
              <Grid container spacing={3}>
                <Grid item xs={ 12 } md={ 6 }>
                  <Typography variant="body1">Your Tools:</Typography>
                  <Divider sx={{mt:2,mb:2}} />
                  {
                    tools.userTools.map((tool) => {
                      return (
                        <Box sx={{ mb: 2 }} key={tool.id}>
                          <FormControlLabel
                            control={
                              <Checkbox 
                                checked={sessionConfig.activeToolIDs.includes(tool.id)}
                                onChange={(event) => {
                                  handleToolsCheckboxChange(tool.id, event)
                                }}
                              />
                            }
                            label={(
                              <Box>
                                <Box>
                                  <Typography variant="body1">{ tool.name }</Typography>
                                </Box>
                                <Box>
                                  <Typography variant="caption">{ tool.description }</Typography>
                                </Box>
                              </Box> 
                            )}
                          />
                        </Box>
                      )
                    })
                  }
                </Grid>
                <Grid item xs={ 12 } md={ 6 }>
                  <Typography variant="body1">Global Tools:</Typography>
                  <Divider sx={{mt:2,mb:2}} />
                  {
                    tools.globalTools.map((tool) => {
                      return (
                        <Box sx={{ mb: 2 }} key={tool.id}>
                          <FormControlLabel
                            key={tool.id}
                            control={
                              <Checkbox 
                                checked={ sessionConfig.activeToolIDs.includes(tool.id) }
                                onChange={(event) => {
                                  handleToolsCheckboxChange(tool.id, event)
                                }}
                              />
                            }
                            label={(
                              <Box>
                                <Box>
                                  <Typography variant="body1">{ tool.name }</Typography>
                                </Box>
                                <Box>
                                  <Typography variant="caption">{ tool.description }</Typography>
                                </Box>
                              </Box> 
                            )}
                          />
                        </Box>
                      )
                    })
                  }
                </Grid>
              </Grid>
            </Box>
          )
        }

        {
          activeSettingsTab == (account.serverConfig.tools_enabled ? 1 : 0) && (
            <Box sx={{ mt: 2 }}>
              {
                mode == SESSION_MODE_FINETUNE && (
                  <FormGroup row>
                    <FormControlLabel
                      control={
                        <Checkbox 
                          checked={sessionConfig.finetuneEnabled}
                          onChange={(event) => setSessionConfig(config => ({
                            ...config,
                            finetuneEnabled: event.target.checked,
                          }))}
                        />
                      }
                      label="Finetune Enabled?"
                    />
                    {
                      type == SESSION_TYPE_TEXT && (
                        <FormControlLabel
                          control={
                            <Checkbox 
                              checked={sessionConfig.ragEnabled}
                              onChange={(event) => setSessionConfig(config => ({
                                ...config,
                                ragEnabled: event.target.checked,
                              }))}
                            />
                          }
                          label="Rag Enabled?"
                        />
                      )
                    }
                  </FormGroup>
                )
              }
              {
                sessionConfig.ragEnabled && (
                  <>
                    <Divider sx={{mt:2,mb:2}} />
                    <Typography variant="h6" gutterBottom sx={{mb: 2}}>RAG Settings</Typography>
                    <Grid container spacing={3}>
                      <Grid item xs={ 12 } md={ 4 }>
                        <FormControl fullWidth>
                          <InputLabel>Rag Distance Function</InputLabel>
                          <Select
                            value={sessionConfig.ragDistanceFunction}
                            label="Rag Distance Function"
                            onChange={(event) => setSessionConfig(config => ({
                              ...config,
                              ragDistanceFunction: event.target.value as any,
                            }))}
                          >
                            <MenuItem value="l2">l2</MenuItem>
                            <MenuItem value="inner_product">inner_product</MenuItem>
                            <MenuItem value="cosine">cosine</MenuItem>
                          </Select>
                        </FormControl>
                      </Grid>
                      <Grid item xs={ 12 } md={ 4 }>
                        <TextField
                          fullWidth
                          label="Rag Threshold"
                          type="number"
                          InputLabelProps={{
                            shrink: true,
                          }}
                          variant="standard"
                          value={ sessionConfig.ragThreshold }
                          onChange={(event) => setSessionConfig(config => ({
                            ...config,
                            ragThreshold: event.target.value as any,
                          }))}
                        />
                      </Grid>
                      <Grid item xs={ 12 } md={ 4 }>
                        <TextField
                          fullWidth
                          label="Rag Results Count"
                          type="number"
                          InputLabelProps={{
                            shrink: true,
                          }}
                          variant="standard"
                          value={ sessionConfig.ragResultsCount }
                          onChange={(event) => setSessionConfig(config => ({
                            ...config,
                            ragResultsCount: event.target.value as any,
                          }))}
                        />
                      </Grid>
                      <Grid item xs={ 12 } md={ 4 }>
                        <TextField
                          fullWidth
                          label="Rag Chunk Size"
                          type="number"
                          InputLabelProps={{
                            shrink: true,
                          }}
                          variant="standard"
                          value={ sessionConfig.ragChunkSize }
                          onChange={(event) => setSessionConfig(config => ({
                            ...config,
                            ragChunkSize: event.target.value as any,
                          }))}
                        />
                      </Grid>
                      <Grid item xs={ 12 } md={ 4 }>
                        <TextField
                          fullWidth
                          label="Rag Chunk Overflow"
                          type="number"
                          InputLabelProps={{
                            shrink: true,
                          }}
                          variant="standard"
                          value={ sessionConfig.ragChunkOverflow }
                          onChange={(event) => setSessionConfig(config => ({
                            ...config,
                            ragChunkOverflow: event.target.value as any,
                          }))}
                        />
                      </Grid>
                    </Grid>
                  </>
                )
              }
            </Box>
          )
        }              
      </Box>
    </Window>
  )
}

export default CreateSettingsWindow
