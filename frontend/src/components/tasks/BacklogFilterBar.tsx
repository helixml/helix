import React from 'react';
import {
  Box,
  TextField,
  InputAdornment,
  Select,
  MenuItem,
  Chip,
  Button,
  FormControl,
  InputLabel,
  OutlinedInput,
  Checkbox,
  ListItemText,
} from '@mui/material';
import { Search as SearchIcon, Clear as ClearIcon } from '@mui/icons-material';

import { TypesSpecTaskPriority } from '../../api/api';

interface BacklogFilterBarProps {
  search: string;
  onSearchChange: (value: string) => void;
  priorityFilter: TypesSpecTaskPriority[];
  onPriorityFilterChange: (value: TypesSpecTaskPriority[]) => void;
}

const PRIORITY_OPTIONS = [
  { value: TypesSpecTaskPriority.SpecTaskPriorityCritical, label: 'Critical', color: 'error' },
  { value: TypesSpecTaskPriority.SpecTaskPriorityHigh, label: 'High', color: 'warning' },
  { value: TypesSpecTaskPriority.SpecTaskPriorityMedium, label: 'Medium', color: 'info' },
  { value: TypesSpecTaskPriority.SpecTaskPriorityLow, label: 'Low', color: 'success' },
];

const BacklogFilterBar: React.FC<BacklogFilterBarProps> = ({
  search,
  onSearchChange,
  priorityFilter,
  onPriorityFilterChange,
}) => {
  const hasFilters = search.length > 0 || priorityFilter.length > 0;

  const handleClearFilters = () => {
    onSearchChange('');
    onPriorityFilterChange([]);
  };

  return (
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 2,
        px: 2.5,
        py: 1.5,
        borderBottom: '1px solid',
        borderColor: 'divider',
      }}
    >
      {/* Search Input */}
      <TextField
        size="small"
        placeholder="Search prompts..."
        value={search}
        onChange={(e) => onSearchChange(e.target.value)}
        sx={{ flex: 1, maxWidth: 300 }}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <SearchIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
            </InputAdornment>
          ),
        }}
      />

      {/* Priority Filter */}
      <FormControl size="small" sx={{ minWidth: 150 }}>
        <InputLabel>Priority</InputLabel>
        <Select
          multiple
          value={priorityFilter}
          onChange={(e) => onPriorityFilterChange(e.target.value as TypesSpecTaskPriority[])}
          input={<OutlinedInput label="Priority" />}
          renderValue={(selected) => (
            <Box sx={{ display: 'flex', flexWrap: 'wrap', gap: 0.5 }}>
              {selected.map((value) => {
                const option = PRIORITY_OPTIONS.find((o) => o.value === value);
                return (
                  <Chip
                    key={value}
                    label={option?.label || value}
                    size="small"
                    color={option?.color as any}
                    sx={{ height: 20, fontSize: '0.7rem' }}
                  />
                );
              })}
            </Box>
          )}
        >
          {PRIORITY_OPTIONS.map((option) => (
            <MenuItem key={option.value} value={option.value}>
              <Checkbox checked={priorityFilter.includes(option.value)} size="small" />
              <ListItemText
                primary={
                  <Chip
                    label={option.label}
                    size="small"
                    color={option.color as any}
                    sx={{ height: 24 }}
                  />
                }
              />
            </MenuItem>
          ))}
        </Select>
      </FormControl>

      {/* Clear Filters Button */}
      {hasFilters && (
        <Button
          size="small"
          startIcon={<ClearIcon />}
          onClick={handleClearFilters}
          sx={{ textTransform: 'none' }}
        >
          Clear
        </Button>
      )}
    </Box>
  );
};

export default BacklogFilterBar;
