import React from 'react';
import { Dialog, DialogProps, styled } from '@mui/material';

const StyledDarkDialog = styled(Dialog)(({ theme }) => ({
  '& .MuiPaper-root': {
    background: '#181A20',
    color: '#F1F1F1',
    borderRadius: 16,
    boxShadow: '0 8px 32px rgba(0,0,0,0.5)',
    transition: 'all 0.2s ease-in-out',
  },
  '&.MuiDialog-root': {
    transition: 'all 0.2s ease-in-out',
  },
}));

interface DarkDialogProps extends DialogProps {
  children: React.ReactNode;
}

const DarkDialog: React.FC<DarkDialogProps> = ({ children, ...props }) => {
  return <StyledDarkDialog {...props}>{children}</StyledDarkDialog>;
};

export default DarkDialog;
