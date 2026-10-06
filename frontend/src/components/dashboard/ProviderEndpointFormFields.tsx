import React, { FC, ReactNode } from 'react'
import {
  Box,
  Button,
  IconButton,
  Stack,
  TextField,
  Tooltip,
  Typography,
} from '@mui/material'
import { Plus, Trash2 } from 'lucide-react'

export interface ProviderEndpointHeader {
  key: string
  value: string
}

interface FormSectionProps {
  title: string
  description?: string
  action?: ReactNode
  children: ReactNode
}

/**
 * A titled block of form controls. Used by the create and edit provider
 * endpoint dialogs so both keep the same heading rhythm.
 */
export const FormSection: FC<FormSectionProps> = ({ title, description, action, children }) => (
  <Box>
    <Stack
      direction="row"
      alignItems="center"
      justifyContent="space-between"
      sx={{ minHeight: 32, mb: description ? 0.5 : 1.5 }}
    >
      <Typography variant="subtitle2" sx={{ fontWeight: 600 }}>{title}</Typography>
      {action}
    </Stack>
    {description && (
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
        {description}
      </Typography>
    )}
    {children}
  </Box>
)

/** Responsive two column form row that collapses to one column on narrow screens. */
export const FormRow: FC<{ children: ReactNode }> = ({ children }) => (
  <Box sx={{ display: 'grid', gap: 2, gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr' } }}>
    {children}
  </Box>
)

interface CustomHeadersEditorProps {
  headers: ProviderEndpointHeader[]
  onChange: (headers: ProviderEndpointHeader[]) => void
}

export const CustomHeadersEditor: FC<CustomHeadersEditorProps> = ({ headers, onChange }) => {
  const updateHeader = (index: number, field: 'key' | 'value', value: string) => {
    onChange(headers.map((header, i) => (i === index ? { ...header, [field]: value } : header)))
  }

  return (
    <FormSection
      title="Custom headers"
      description="Sent with every request to this endpoint, in addition to the authentication header."
      action={(
        <Button
          startIcon={<Plus size={16} />}
          onClick={() => onChange([...headers, { key: '', value: '' }])}
          variant="outlined"
          size="small"
        >
          Add header
        </Button>
      )}
    >
      {headers.length === 0 ? (
        <Typography variant="body2" color="text.secondary" sx={{ fontStyle: 'italic' }}>
          No custom headers.
        </Typography>
      ) : (
        <Stack spacing={1.5}>
          {headers.map((header, index) => (
            <Box
              key={index}
              sx={{ display: 'grid', gridTemplateColumns: '1fr 1fr auto', gap: 1, alignItems: 'center' }}
            >
              <TextField
                label="Header name"
                value={header.key}
                onChange={(e) => updateHeader(index, 'key', e.target.value)}
                placeholder="X-Api-Key"
                autoComplete="off"
                size="small"
              />
              <TextField
                label="Header value"
                value={header.value}
                onChange={(e) => updateHeader(index, 'value', e.target.value)}
                placeholder="your-api-key"
                autoComplete="off"
                size="small"
              />
              <Tooltip title="Remove header">
                <IconButton
                  onClick={() => onChange(headers.filter((_, i) => i !== index))}
                  aria-label={`Remove header ${header.key || index + 1}`}
                  sx={{ width: 30, height: 30, color: 'text.secondary', '&:hover': { color: 'error.main' } }}
                >
                  <Trash2 size={18} />
                </IconButton>
              </Tooltip>
            </Box>
          ))}
        </Stack>
      )}
    </FormSection>
  )
}
