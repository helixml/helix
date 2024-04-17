import React, { FC, useState, ChangeEvent } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import IconButton from '@mui/material/IconButton'
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline'
import DeleteIcon from '@mui/icons-material/Delete'

const StringArrayEditor: FC<React.PropsWithChildren<{
  data: string[],
  entityTitle?: string,
  disabled?: boolean,
  onChange: (data: string[]) => void,
}>> = ({
  data,
  entityTitle = 'key',
  disabled = false,
  onChange,
}) => {
  const [currentList, setCurrentList] = useState<string[]>(data)
  const [newValue, setNewValue] = useState('')
  
  const handleAddEntry = () => {
    const newList = currentList.concat([newValue])
    setNewValue('')
    setCurrentList(newList)
    onChange(newList)
  }

  const handleDeleteEntry = (index: number) => {
    const newList = currentList.filter((_, i) => i !== index)
    setNewValue('')
    setCurrentList(newList)
    onChange(newList)
  }

  const handleChangeValue = (index: number, newValue: string) => {
    const newList = currentList.map((value, i) => i === index ? newValue : value)
    setNewValue('')
    setCurrentList(newList)
    onChange(newList)
  }

  return (
    <Box>
      {currentList.map((value, index) => (
        <Box key={index} display="flex" alignItems="center" gap={2} marginBottom={2}>
          <TextField
            size="small"
            variant="outlined"
            value={ value }
            disabled={ disabled }
            onChange={(e: ChangeEvent<HTMLInputElement>) => handleChangeValue(index, e.target.value)}
          />
          <IconButton onClick={() => handleDeleteEntry(index)}>
            <DeleteIcon />
          </IconButton>
        </Box>
      ))}
      <Box display="flex" alignItems="center" gap={2} marginBottom={2}>
        <TextField
          size="small"
          label="new value"
          variant="outlined"
          value={newValue}
          disabled={ disabled }
          onChange={(e: ChangeEvent<HTMLInputElement>) => setNewValue(e.target.value)}
          onKeyPress={(e) => e.key === 'Enter' && handleAddEntry()}
        />
        <IconButton onClick={handleAddEntry} disabled={!newValue}>
          <AddCircleOutlineIcon />
        </IconButton>
      </Box>
    </Box>
  )
}

export default StringArrayEditor
