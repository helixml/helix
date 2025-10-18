import Box from '@mui/material/Box'
import Checkbox from '@mui/material/Checkbox'
import Divider from '@mui/material/Divider'
import FormControl from '@mui/material/FormControl'
import FormControlLabel from '@mui/material/FormControlLabel'
import FormGroup from '@mui/material/FormGroup'
import Grid from '@mui/material/Grid'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Tab from '@mui/material/Tab'
import Tabs from '@mui/material/Tabs'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import React, { Dispatch, FC, SetStateAction, useEffect, useState } from 'react'

import useAccount from '../../hooks/useAccount'
import Window from '../widgets/Window'
import { AgentTypeSelector } from '../agent'

import {
  ICreateSessionConfig,
  IAgentType,
  IExternalAgentConfig,
  AGENT_TYPE_HELIX_BASIC,
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
  onSetSessionConfig: Dispatch<SetStateAction<ICreateSessionConfig>>,
  onClose: () => void,
}> = ({
  mode,
  type,
  sessionConfig,
  onSetSessionConfig,
  onClose,
}) => {
    const account = useAccount()
    const [activeSettingsTab, setActiveSettingsTab] = useState(0)

    const showLearn = mode == 'finetune'
    const agentTab = 0
    const learnTab = showLearn ? 1 : 0

    const handleToolsCheckboxChange = (id: string, event: React.ChangeEvent<HTMLInputElement>) => {
      if (event.target.checked) {
        onSetSessionConfig(config => ({
          ...config,
          activeToolIDs: [...config.activeToolIDs, id],
        }))
      } else {
        onSetSessionConfig(config => ({
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
        onCancel={onClose}
        withCancel
        cancelTitle="Close"
      >
        <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
          <Tabs value={activeSettingsTab} onChange={(event: React.SyntheticEvent, newValue: number) => {
            setActiveSettingsTab(newValue)
          }}>
            <Tab label="Agent Settings" />
            {
              showLearn && (
                <Tab label="Finetune & RAG" />
              )
            }
          </Tabs>
        </Box>
        <Box>
          {
            activeSettingsTab == agentTab && (
              <Box sx={{ mt: 2 }}>
                <Typography variant="h6" gutterBottom sx={{ mb: 2 }}>Agent Configuration</Typography>
                <AgentTypeSelector
                  value={sessionConfig.agentType}
                  onChange={(agentType: IAgentType, config?: IExternalAgentConfig) => {
                    onSetSessionConfig(prevConfig => ({
                      ...prevConfig,
                      agentType,
                      externalAgentConfig: config,
                    }))
                  }}
                  externalAgentConfig={sessionConfig.externalAgentConfig}
                  showExternalConfig={true}
                />
              </Box>
            )
          }
          {
            showLearn && activeSettingsTab == learnTab && (
              <Box sx={{ mt: 2 }}>
                {
                  mode == SESSION_MODE_FINETUNE && (
                    <FormGroup row>
                      <FormControlLabel
                        control={
                          <Checkbox
                            checked={sessionConfig.finetuneEnabled}
                            onChange={(event) => onSetSessionConfig(config => ({
                              ...config,
                              finetuneEnabled: event.target.checked,
                              // Set rag to the opposite of RAG
                              ragEnabled: !event.target.checked,
                            }))}
                          />
                        }
                        label="Enable Fine-Tuning"
                      />
                      {
                        type == SESSION_TYPE_TEXT && (
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={sessionConfig.ragEnabled}
                                onChange={(event) => onSetSessionConfig(config => ({
                                  ...config,
                                  ragEnabled: event.target.checked,
                                  // Set finetune to the opposite of RAG
                                  finetuneEnabled: !event.target.checked,
                                }))}
                              />
                            }
                            label="Enable RAG"
                          />
                        )
                      }
                    </FormGroup>
                  )
                }
                {
                  type == 'text' && sessionConfig.ragEnabled && (
                    <>
                      <Divider sx={{ mt: 2, mb: 2 }} />
                      <Typography variant="h6" gutterBottom sx={{ mb: 2 }}>RAG Settings</Typography>
                      <Grid container spacing={3}>
                        <Grid item xs={12} md={4}>
                          <FormControl fullWidth>
                            <InputLabel>Rag Distance Function</InputLabel>
                            <Select
                              value={sessionConfig.ragDistanceFunction}
                              label="Rag Distance Function"
                              onChange={(event) => onSetSessionConfig(config => ({
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
                        <Grid item xs={12} md={4}>
                          <TextField
                            fullWidth
                            label="Rag Threshold"
                            type="number"
                            InputLabelProps={{
                              shrink: true,
                            }}
                            variant="standard"
                            value={sessionConfig.ragThreshold}
                            onChange={(event) => onSetSessionConfig(config => ({
                              ...config,
                              ragThreshold: event.target.value as any,
                            }))}
                          />
                        </Grid>
                        <Grid item xs={12} md={4}>
                          <TextField
                            fullWidth
                            label="Rag Results Count"
                            type="number"
                            InputLabelProps={{
                              shrink: true,
                            }}
                            variant="standard"
                            value={sessionConfig.ragResultsCount}
                            onChange={(event) => onSetSessionConfig(config => ({
                              ...config,
                              ragResultsCount: event.target.value as any,
                            }))}
                          />
                        </Grid>
                        <Grid item xs={12} md={4}>
                          <TextField
                            fullWidth
                            label="Rag Chunk Size"
                            type="number"
                            InputLabelProps={{
                              shrink: true,
                            }}
                            variant="standard"
                            value={sessionConfig.ragChunkSize}
                            onChange={(event) => onSetSessionConfig(config => ({
                              ...config,
                              ragChunkSize: event.target.value as any,
                            }))}
                          />
                        </Grid>
                        <Grid item xs={12} md={4}>
                          <FormControlLabel
                            control={
                              <Checkbox
                                checked={sessionConfig.ragDisableChunking}
                                onChange={(event) => onSetSessionConfig(config => ({
                                  ...config,
                                  ragDisableChunking: event.target.checked,
                                }))}
                              />
                            }
                            label="Disable Chunking"
                          />
                        </Grid>
                        <Grid item xs={12} md={4}>
                          <TextField
                            fullWidth
                            label="Rag Chunk Overflow"
                            type="number"
                            InputLabelProps={{
                              shrink: true,
                            }}
                            variant="standard"
                            value={sessionConfig.ragChunkOverflow}
                            onChange={(event) => onSetSessionConfig(config => ({
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
