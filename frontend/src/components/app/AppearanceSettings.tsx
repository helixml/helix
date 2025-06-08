import React, { FC, useState } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import DeleteIcon from '@mui/icons-material/Delete'
import AddIcon from '@mui/icons-material/Add'
import { IAppFlatState } from '../../types'

interface AppearanceSettingsProps {
  app: IAppFlatState
  onUpdate: (updates: IAppFlatState) => Promise<void>
  readOnly?: boolean
  showErrors?: boolean
}

const AppearanceSettings: FC<AppearanceSettingsProps> = ({
  app,
  onUpdate,
  readOnly = false,
  showErrors = true,
}) => {
  const [name, setName] = useState(app.name || '')
  const [description, setDescription] = useState(app.description || '')
  const [avatar, setAvatar] = useState(app.avatar || '')
  const [image, setImage] = useState(app.image || '')
  const [conversationStarters, setConversationStarters] = useState<string[]>(app.conversation_starters || [])
  const [newStarter, setNewStarter] = useState('')

  const handleBlur = (field: 'name' | 'description' | 'avatar' | 'image') => {
    const currentValue = {
      name,
      description,
      avatar,
      image,
    }[field]
    
    const originalValue = (app[field] || '') as string
    
    if (currentValue !== originalValue) {
      const updatedApp: IAppFlatState = {
        ...app,
        name,
        description,
        avatar,
        image,
        conversation_starters: conversationStarters
      }
      
      onUpdate(updatedApp)
    }
  }

  const handleConversationStarterBlur = () => {
    if (newStarter.trim()) {
      const updatedStarters = [...conversationStarters, newStarter.trim()]
      setConversationStarters(updatedStarters)
      setNewStarter('')
      
      const updatedApp: IAppFlatState = {
        ...app,
        conversation_starters: updatedStarters
      }
      onUpdate(updatedApp)
    }
  }

  const handleConversationStarterChange = (index: number, value: string) => {
    const updatedStarters = [...conversationStarters]
    updatedStarters[index] = value
    setConversationStarters(updatedStarters)
    
    const updatedApp: IAppFlatState = {
      ...app,
      conversation_starters: updatedStarters
    }
    onUpdate(updatedApp)
  }

  const handleAddStarter = () => {
    if (newStarter.trim()) {
      const updatedStarters = [...conversationStarters, newStarter.trim()]
      setConversationStarters(updatedStarters)
      setNewStarter('')
      
      const updatedApp: IAppFlatState = {
        ...app,
        conversation_starters: updatedStarters
      }
      onUpdate(updatedApp)
    }
  }

  const handleRemoveStarter = (index: number) => {
    const updatedStarters = conversationStarters.filter((_, i) => i !== index)
    setConversationStarters(updatedStarters)
    
    const updatedApp: IAppFlatState = {
      ...app,
      conversation_starters: updatedStarters
    }
    onUpdate(updatedApp)
  }

  return (
    <Box sx={{ mt: 2 }}>
      <Box sx={{ mb: 3 }}>
        <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
          Application name
        </Typography>
        <TextField
          sx={{ mb: 2 }}
          id="app-name"
          name="app-name"
          error={showErrors && !name}
          value={name}
          disabled={readOnly}
          onChange={(e) => setName(e.target.value)}
          onBlur={() => handleBlur('name')}
          fullWidth
          label="Name"
          helperText="Name your app"
        />
        <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
          Description
        </Typography>
        <TextField
          sx={{ mb: 2 }}
          id="app-description"
          name="app-description"
          value={description}
          onChange={(e) => setDescription(e.target.value)}
          onBlur={() => handleBlur('description')}
          disabled={readOnly}
          fullWidth
          rows={2}
          label="Description"
          helperText="Enter a short description of what this app does, e.g. 'Tax filing assistant'"
        />
        <TextField
          sx={{ mb: 2 }}
          id="app-avatar"
          name="app-avatar"
          value={avatar}
          onChange={(e) => setAvatar(e.target.value)}
          onBlur={() => handleBlur('avatar')}
          disabled={readOnly}
          fullWidth
          label="Avatar"
          helperText="URL for the app's avatar image"
        />
        <TextField
          sx={{ mb: 2 }}
          id="app-image"
          name="app-image"
          value={image}
          onChange={(e) => setImage(e.target.value)}
          onBlur={() => handleBlur('image')}
          disabled={readOnly}
          fullWidth
          label="Background Image"
          helperText="URL for the app's main image"
        />
        
        <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
          Conversation Starters
        </Typography>
        <Box sx={{ mb: 2 }}>
          <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
            Add example messages that users can click to start a conversation. These help showcase the app's capabilities.
          </Typography>
          {conversationStarters.map((starter, index) => (
            <Box key={index} sx={{ display: 'flex', alignItems: 'center', mb: 1 }}>
              <TextField
                fullWidth
                value={starter}
                onChange={(e) => handleConversationStarterChange(index, e.target.value)}
                onBlur={() => {
                  const updatedApp: IAppFlatState = {
                    ...app,
                    conversation_starters: conversationStarters
                  }
                  onUpdate(updatedApp)
                }}
                disabled={readOnly}
                size="small"
              />
              <IconButton 
                onClick={() => handleRemoveStarter(index)}
                disabled={readOnly}
                sx={{ ml: 1 }}
              >
                <DeleteIcon />
              </IconButton>
            </Box>
          ))}
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <TextField
              fullWidth
              label="Conversation Starter"
              value={newStarter}
              onChange={(e) => setNewStarter(e.target.value)}
              onBlur={handleConversationStarterBlur}
              onKeyPress={(e) => {
                if (e.key === 'Enter') {
                  handleAddStarter()
                }
              }}
              disabled={readOnly}
              size="small"              
            />
            <IconButton 
              onClick={handleAddStarter}
              disabled={readOnly || !newStarter.trim()}
              sx={{ ml: 1, mb: 3 }}
            >
              <AddIcon />
            </IconButton>
          </Box>
        </Box>
      </Box>
    </Box>
  )
}

export default AppearanceSettings 