import React, { FC, useState } from 'react'
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  Typography,
} from '@mui/material'

import { TypesCodeAgentExecutionConfig } from '../../api/api'
import CodeAgentExecutionControls from '../agent/CodeAgentExecutionControls'


interface AgentSelectionModalProps {
  open: boolean
  onClose: () => void
  onSelect: (config: TypesCodeAgentExecutionConfig) => void
  title?: string
  description?: string
}

const AgentSelectionModal: FC<AgentSelectionModalProps> = ({
  open,
  onClose,
  onSelect,
  title = 'Select Task Defaults',
  description = 'Choose the coding harness, credentials, provider, and model for tasks in this project.',
}) => {
  const [config, setConfig] = useState<TypesCodeAgentExecutionConfig>()

  const handleSelect = () => {
    if (config) {
      onSelect(config)
      onClose()
    }
  }

  const handleClose = () => {
    setConfig(undefined)
    onClose()
  }

  return (
    <Dialog
      open={open}
      onClose={handleClose}
      maxWidth="sm"
      fullWidth
    >
      <DialogTitle>{title}</DialogTitle>
      <DialogContent>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          {description}
        </Typography>
        <CodeAgentExecutionControls
          value={config}
          onChange={setConfig}
          grouped
        />
      </DialogContent>

      <DialogActions>
        <Button onClick={handleClose}>Cancel</Button>
        <Button
          onClick={handleSelect}
          variant="contained"
          disabled={!config?.model}
        >
          Continue
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default AgentSelectionModal
