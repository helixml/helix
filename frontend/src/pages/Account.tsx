import React, { FC } from 'react'
import Box from '@mui/material/Box'
import axios from 'axios'

import useAccount from '../hooks/useAccount'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import Typography from '@mui/material/Typography'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import IconButton from '@mui/material/IconButton'
import DeleteIcon from '@mui/icons-material/Delete'

const Account: FC = () => {
  const account = useAccount()
  if(!account.user) return null
  const handleDeleteApiKey = async (key: string) => {
    try {
      await axios.delete(`/api/v1/api_keys?key=${key}`)
    } catch (error) {
      console.error(error)
    }
  }

  return (
    <Box>
      <Typography variant="h6">API Keys</Typography>
      <List>
        {account.apiKeys.map((apiKey) => (
          <ListItem key={apiKey.key}>
            <ListItemText primary={apiKey.name} secondary={apiKey.key} />
            <ListItemSecondaryAction>
              <IconButton
                edge="end"
                aria-label="delete"
                onClick={() => handleDeleteApiKey(apiKey.key)}
              >
                <DeleteIcon />
              </IconButton>
            </ListItemSecondaryAction>
          </ListItem>
        ))}
      </List>
    </Box>
  )
}

export default Account