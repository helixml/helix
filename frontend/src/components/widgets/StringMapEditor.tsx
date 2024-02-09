import React, { FC, useState, ChangeEvent } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import IconButton from '@mui/material/IconButton'
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline'
import DeleteIcon from '@mui/icons-material/Delete'

const StringMapEditor: FC<React.PropsWithChildren<{
  data: Record<string, string>,
  onChange: (data: Record<string, string>) => void,
}>> = ({
  data,
  onChange,
}) => {
  const [record, setRecord] = useState<Record<string, string>>(data)
  const [newKey, setNewKey] = useState('')

  const handleAddEntry = () => {
    if (newKey && !record[newKey]) { // Prevent adding if key exists
      const updatedRecord = { ...record, [newKey]: '' }
      setRecord(updatedRecord)
      onChange(updatedRecord)
      setNewKey('')
    }
  }

  const handleDeleteEntry = (key: string) => {
    const { [key]: value, ...rest } = record
    setRecord(rest)
    onChange(rest)
  }

  const handleChangeValue = (key: string, newValue: string) => {
    const updatedRecord = { ...record, [key]: newValue }
    setRecord(updatedRecord)
    onChange(updatedRecord)
  }

  return (
    <Box>
      {Object.keys(record).map((key) => (
        <Box key={key} display="flex" alignItems="center" gap={2} marginBottom={2}>
          <TextField
            size="small"
            disabled
            variant="outlined"
            value={key}
          />
          =
          <TextField
            size="small"
            variant="outlined"
            value={record[key]}
            onChange={(e: ChangeEvent<HTMLInputElement>) => handleChangeValue(key, e.target.value)}
          />
          <IconButton onClick={() => handleDeleteEntry(key)}>
            <DeleteIcon />
          </IconButton>
        </Box>
      ))}
      <Box display="flex" alignItems="center" gap={2} marginBottom={2}>
        <TextField
          size="small"
          label="New Key"
          variant="outlined"
          value={newKey}
          onChange={(e: ChangeEvent<HTMLInputElement>) => setNewKey(e.target.value)}
          onKeyPress={(e) => e.key === 'Enter' && handleAddEntry()}
        />
        <IconButton onClick={handleAddEntry} disabled={!newKey}>
          <AddCircleOutlineIcon />
        </IconButton>
      </Box>
    </Box>
  )
}

export default StringMapEditor