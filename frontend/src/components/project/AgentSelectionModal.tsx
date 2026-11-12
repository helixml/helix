import React, { FC, useState } from 'react'
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import DialogActions from '@mui/material/DialogActions';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';

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
  description = 'Choose the coding runtime, credentials, provider, and model for tasks in this project.',
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
          autoSelectDefault
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
