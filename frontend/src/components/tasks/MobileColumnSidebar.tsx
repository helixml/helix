import React from 'react';
import { Box, Typography } from '@mui/material';

// Column color mapping helper - exported for use by DroppableColumn
export const getColumnAccent = (id: string) => {
  switch (id) {
    case 'backlog': return { color: '#6b7280', bg: 'transparent' };
    case 'planning': return { color: '#f59e0b', bg: 'rgba(245, 158, 11, 0.08)' };
    case 'review': return { color: '#3b82f6', bg: 'rgba(59, 130, 246, 0.08)' };
    case 'implementation': return { color: '#10b981', bg: 'rgba(16, 185, 129, 0.08)' };
    case 'pull_request': return { color: '#8b5cf6', bg: 'rgba(139, 92, 246, 0.08)' };
    case 'completed': return { color: '#6b7280', bg: 'transparent' };
    default: return { color: '#6b7280', bg: 'transparent' };
  }
};

// Short labels for mobile sidebar
const getShortLabel = (id: string): string => {
  const labels: Record<string, string> = {
    backlog: 'B',
    planning: 'P',
    review: 'R',
    implementation: 'D',
    pull_request: 'PR',
    completed: 'M',
  };
  return labels[id] || id[0].toUpperCase();
};

interface Column {
  id: string;
  title: string;
  tasks: any[];
}

interface MobileColumnSidebarProps {
  columns: Column[];
  currentColumnIndex: number;
  onColumnClick: (index: number) => void;
}

/**
 * Mobile sidebar for navigating between kanban columns.
 * Displays column abbreviations with task counts.
 * Designed to fit on small screens like iPhone mini.
 */
const MobileColumnSidebar: React.FC<MobileColumnSidebarProps> = ({
  columns,
  currentColumnIndex,
  onColumnClick,
}) => {
  return (
    <Box
      sx={{
        position: 'absolute',
        right: 0,
        top: 0,
        // Account for mobile bottom nav height (~56px)
        bottom: '56px',
        width: '24px',
        backgroundColor: 'background.paper',
        borderLeft: '1px solid',
        borderColor: 'divider',
        display: 'flex',
        flexDirection: 'column',
        justifyContent: 'stretch', // Distribute buttons across full height
        zIndex: 10,
      }}
    >
      {columns.map((column, index) => {
        const isActive = index === currentColumnIndex;
        const accent = getColumnAccent(column.id);
        const shortLabel = getShortLabel(column.id);

        return (
          <Box
            key={column.id}
            onClick={() => onColumnClick(index)}
            sx={{
              // flex: 1 distributes space evenly across all buttons
              flex: 1,
              // Minimum height for touch target (smaller on very small screens)
              minHeight: '32px',
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              cursor: 'pointer',
              backgroundColor: isActive ? 'action.selected' : 'transparent',
              transition: 'background-color 0.15s ease',
              // Active indicator on the left edge
              borderLeft: isActive ? `2px solid ${accent.color}` : '2px solid transparent',
              '&:active': {
                backgroundColor: 'action.hover',
              },
            }}
          >
            <Typography
              sx={{
                fontSize: '0.6rem',
                fontWeight: isActive ? 700 : 500,
                color: isActive ? accent.color : 'text.secondary',
                lineHeight: 1,
                letterSpacing: '-0.02em',
              }}
            >
              {shortLabel}
            </Typography>
            <Typography
              sx={{
                fontSize: '0.5rem',
                fontWeight: 600,
                color: isActive ? accent.color : 'text.disabled',
                lineHeight: 1,
                mt: 0.25,
              }}
            >
              {column.tasks.length}
            </Typography>
          </Box>
        );
      })}
    </Box>
  );
};

export default MobileColumnSidebar;
