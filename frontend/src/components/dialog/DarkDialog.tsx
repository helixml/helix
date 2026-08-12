import React from 'react';
import { Dialog, DialogProps } from '@mui/material';

interface DarkDialogProps extends DialogProps {
  children: React.ReactNode;
}

const DarkDialog: React.FC<DarkDialogProps> = ({ children, ...props }) => {
  return <Dialog {...props}>{children}</Dialog>;
};

export default DarkDialog;
