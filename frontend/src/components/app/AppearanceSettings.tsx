import React, { FC, useState, useRef } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import DeleteIcon from '@mui/icons-material/Delete'
import AddIcon from '@mui/icons-material/Add'
import Avatar from '@mui/material/Avatar'
import Grid from '@mui/material/Grid'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import { IAppFlatState } from '../../types'
import { useUpdateAppAvatar, useDeleteAppAvatar } from '../../services/appService'
import { getFlatStateAvatarUrl } from '../../utils/app'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'
import DeleteForeverIcon from '@mui/icons-material/DeleteForever'
import useApps from '../../hooks/useApps'
import useThemeConfig from '../../hooks/useThemeConfig'

interface AppearanceSettingsProps {
  app: IAppFlatState
  onUpdate: (updates: IAppFlatState) => Promise<void>
  readOnly?: boolean
  showErrors?: boolean
  id: string
}

const AppearanceSettings: FC<AppearanceSettingsProps> = ({
  app,
  onUpdate,
  readOnly = false,
  showErrors = true,
  id,
}) => {
  const [name, setName] = useState(app.name || '')
  const [description, setDescription] = useState(app.description || '')
  const [conversationStarters, setConversationStarters] = useState<string[]>(app.conversation_starters || [])
  const [newStarter, setNewStarter] = useState('')
  const [avatarUpdateKey, setAvatarUpdateKey] = useState(0)
  const fileInputRef = useRef<HTMLInputElement>(null)

  const updateAvatarMutation = useUpdateAppAvatar(id)
  const deleteAvatarMutation = useDeleteAppAvatar(id)

  const apps = useApps()
  const themeConfig = useThemeConfig()

  const handleBlur = (field: 'name' | 'description') => {
    const currentValue = {
      name,
      description,
    }[field]
    
    const originalValue = (app[field] || '') as string
    
    if (currentValue !== originalValue) {
      const updatedApp: IAppFlatState = {
        ...app,
        name,
        description,
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

  const handleAvatarClick = () => {
    if (!readOnly && fileInputRef.current) {
      fileInputRef.current.click()
    }
  }

  const handleFileChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0]
    if (file) {
      try {
        await updateAvatarMutation.mutateAsync(file)
        setAvatarUpdateKey(prev => prev + 1)
                
        await apps.loadApp(id)
        const updatedApp = await apps.app
        if (updatedApp) {
          console.log('updated app', updatedApp)
          onUpdate(updatedApp)
        }
      } catch (error) {
        console.error('Failed to upload avatar:', error)
      }
    }
  }

  const handleDeleteAvatar = async () => {
    try {
      await deleteAvatarMutation.mutateAsync()
      setAvatarUpdateKey(prev => prev + 1)
      
      // After deleting the avatar, reload the app and update parent state
      await apps.loadApp(id)
      const updatedApp = await apps.app
      if (updatedApp) {
        onUpdate(updatedApp)
      }
    } catch (error) {
      console.error('Failed to delete avatar:', error)
    }
  }

  return (
    <Box sx={{ mt: 2, mr: 2 }}>
      {/* Basic Information Card */}
      {/* <Card sx={{ mb: 3, backgroundColor: themeConfig.darkPanel }}>
        <CardContent> */}
          <Grid container spacing={3}>
            {/* Left column - Name and Description */}
            <Grid item xs={12} md={6}>
              <Box sx={{ mb: 3 }}>
                <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
                  Agent name
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
                  helperText="Enter a short description of what this agent does, e.g. 'Tax filing assistant'"
                />
              </Box>
            </Grid>

            {/* Right column - Avatar */}
            <Grid item xs={12} md={6}>
              <Box
                sx={{
                  display: 'flex',
                  flexDirection: 'column',
                  alignItems: 'center',
                  justifyContent: 'center',
                  height: '100%',
                  position: 'relative',
                }}
              >
                <Box
                  sx={{
                    position: 'relative',
                    cursor: readOnly ? 'default' : 'pointer',
                    '&:hover .avatar-overlay': {
                      opacity: 1,
                    },
                  }}
                  onClick={handleAvatarClick}
                >              
                  <Avatar
                    src={`${getFlatStateAvatarUrl(app, id)}${getFlatStateAvatarUrl(app, id).includes('?') ? '&' : '?'}t=${avatarUpdateKey}`}
                    // src={`/api/v1/apps/${id}/avatar?t=${avatarUpdateKey}`}
                    sx={{
                      width: 200,
                      height: 200,
                      border: '2px solid #fff',
                      boxShadow: '0 4px 8px rgba(0,0,0,0.1)',
                    }}
                  />
                  {!readOnly && (
                    <Box
                      className="avatar-overlay"
                      sx={{
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        right: 0,
                        bottom: 0,
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        backgroundColor: 'rgba(0, 0, 0, 0.5)',
                        borderRadius: '50%',
                        opacity: 0,
                        transition: 'opacity 0.2s',
                      }}
                    >
                      <CloudUploadIcon sx={{ color: 'white', fontSize: 40 }} />
                    </Box>
                  )}
                </Box>
                {!readOnly && app.avatar && (
                  <IconButton
                    onClick={handleDeleteAvatar}
                    sx={{
                      mt: 2,
                      color: 'error.main',
                      '&:hover': {
                        backgroundColor: 'error.light',
                      },
                    }}
                  >
                    <DeleteForeverIcon />
                  </IconButton>
                )}
                <input
                  type="file"
                  ref={fileInputRef}
                  style={{ display: 'none' }}
                  accept="image/*,.svg"
                  onChange={handleFileChange}
                />
              </Box>
            </Grid>
          </Grid>
        {/* </CardContent>
      </Card> */}

      {/* Conversation Starters Card */}
      {/* <Card sx={{ backgroundColor: themeConfig.darkPanel, boxShadow: 'none' }}>
        <CardContent> */}
          <Typography variant="h6" sx={{ mb: 2 }} gutterBottom>
            Conversation Starters
          </Typography>
          <Box sx={{ mb: 2 }}>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
              Add example messages that users can click to start a conversation. These help showcase the agent's capabilities.
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
                sx={{ ml: 1 }}
              >
                <AddIcon />
              </IconButton>
            </Box>
          </Box>
        {/* </CardContent>
      </Card> */}
    </Box>
  )
}

export default AppearanceSettings 