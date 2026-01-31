import React, { useState } from 'react';
import { IconButton, Tooltip, useTheme, Box } from '@mui/material';
import { Copy, Check } from 'lucide-react';

interface CopyButtonProps {
  content: string;
  title?: string;
  size?: 'small' | 'medium' | 'large';
  position?: 'top-right' | 'top-left' | 'bottom-right' | 'bottom-left';
  sx?: any;
}

const CopyButton: React.FC<CopyButtonProps> = ({ 
  content, 
  title = 'Content', 
  size = 'small',
  position = 'top-right',  
  sx = {
    mr: 2,
    mt: 1
  }
}) => {
  const [copied, setCopied] = useState(false);
  const theme = useTheme();

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(content);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy text: ', err);
    }
  };

  const getPositionStyles = () => {
    const baseStyles = {
      position: 'absolute' as const,
      zIndex: 1,
    };

    switch (position) {
      case 'top-left':
        return { ...baseStyles, left: 8, top: 8 };
      case 'bottom-right':
        return { ...baseStyles, right: 8, bottom: 8 };
      case 'bottom-left':
        return { ...baseStyles, left: 8, bottom: 8 };
      case 'top-right':
      default:
        return { ...baseStyles, right: 8, top: 8 };
    }
  };

  return (
    <Box sx={getPositionStyles()}>
      <Tooltip title={copied ? "Copied!" : `Copy ${title}`}>
        <IconButton
          onClick={handleCopy}
          size={size}
          sx={{
            backgroundColor: theme.palette.mode === 'light' ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)',
            '&:hover': {
              backgroundColor: theme.palette.mode === 'light' ? 'rgba(255, 255, 255, 0.2)' : 'rgba(0, 0, 0, 0.2)',
            },
            '& .MuiSvgIcon-root': {
              color: theme.palette.mode === 'light' ? 'rgba(0, 0, 0, 0.6)' : 'rgba(255, 255, 255, 0.6)',
            },
            ...sx
          }}
        >
          {copied ? <Check size={size === 'small' ? 16 : size === 'medium' ? 20 : 24} /> : <Copy size={size === 'small' ? 16 : size === 'medium' ? 20 : 24} />}
        </IconButton>
      </Tooltip>
    </Box>
  );
};

export default CopyButton; 