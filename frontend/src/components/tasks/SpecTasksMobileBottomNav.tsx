import React, { FC } from 'react';
import { Box, Button, IconButton } from '@mui/material';
import { Plus, MessageCircle, MoreHorizontal } from 'lucide-react';

interface SpecTasksMobileBottomNavProps {
  onNewTask: () => void;
  onNewChat: () => void;
  chatDisabled?: boolean;
  onMenuClick?: (event: React.MouseEvent<HTMLElement>) => void;
}

const SpecTasksMobileBottomNav: FC<SpecTasksMobileBottomNavProps> = ({
  onNewTask,
  onNewChat,
  chatDisabled = false,
  onMenuClick,
}) => {
  return (
    <Box
      sx={{
        position: 'fixed',
        bottom: 0,
        left: 0,
        right: 0,
        display: 'flex',
        gap: 1.5,
        px: 2,
        py: 1.5,
        backgroundColor: 'background.paper',
        zIndex: 1100,
        boxShadow: '0 -2px 8px rgba(0,0,0,0.1)',
      }}
    >
      <Button
        variant="text"
        color="secondary"
        startIcon={<Plus size={20} />}
        onClick={onNewTask}
        sx={{
          flex: 1,
          py: 1.25,
          fontWeight: 500,
          fontSize: '0.9rem',
          textTransform: 'none',
          borderRadius: 2,
          '&:hover': {
            backgroundColor: 'action.hover',
          },
        }}
      >
        New Task
      </Button>
      <Button
        variant="text"
        startIcon={<MessageCircle size={20} />}
        onClick={onNewChat}
        disabled={chatDisabled}
        sx={{
          flex: 1,
          py: 1.25,
          fontWeight: 500,
          fontSize: '0.9rem',
          textTransform: 'none',
          borderRadius: 2,
          '&:hover': {
            backgroundColor: 'action.hover',
          },
        }}
      >
        Chat
      </Button>
      {onMenuClick && (
        <IconButton
          onClick={onMenuClick}
          sx={{
            color: 'text.primary',
            '&:hover': {
              backgroundColor: 'action.hover',
            },
          }}
        >
          <MoreHorizontal size={20} />
        </IconButton>
      )}
    </Box>
  );
};

export default SpecTasksMobileBottomNav;
