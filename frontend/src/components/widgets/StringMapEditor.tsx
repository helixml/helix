import React, { FC, useState, ChangeEvent } from 'react'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import IconButton from '@mui/material/IconButton'
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline'
import DeleteIcon from '@mui/icons-material/Delete'

const StringMapEditor: FC<React.PropsWithChildren<{
  data: Record<string, string>,
  entityTitle?: string,
  disabled?: boolean,
  onChange: (data: Record<string, string>) => void,
}>> = ({
  data,
  entityTitle = 'key',
  disabled = false,
  onChange,
}) => {
  const [record, setRecord] = useState<Record<string, string>>(data)
  const [newKey, setNewKey] = useState('')
  const [newValue, setNewValue] = useState('')

  const handleAddEntry = () => {
    if (newKey && !record[newKey]) { // Prevent adding if key exists
      const updatedRecord = { ...record, [newKey]: newValue }
      setRecord(updatedRecord)
      onChange(updatedRecord)
      setNewKey('')
      setNewValue('')
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
            disabled={ disabled }
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
          label={`new ${entityTitle}`}
          variant="outlined"
          value={newKey}
          disabled={ disabled }
          onChange={(e: ChangeEvent<HTMLInputElement>) => setNewKey(e.target.value)}
          onKeyPress={(e) => e.key === 'Enter' && handleAddEntry()}
        />
        =
        <TextField
          size="small"
          label="new value"
          variant="outlined"
          value={newValue}
          disabled={ disabled }
          onChange={(e: ChangeEvent<HTMLInputElement>) => setNewValue(e.target.value)}
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
