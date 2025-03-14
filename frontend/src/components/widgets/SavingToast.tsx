import React, { useEffect, useState, useRef } from 'react';
import Box from '@mui/material/Box';
import CircularProgress from '@mui/material/CircularProgress';
import Typography from '@mui/material/Typography';
import Fade from '@mui/material/Fade';

interface SavingToastProps {
  isSaving: boolean;
}

/**
 * Toast notification that appears when app is saving
 * Shows a spinner and "Saving app..." text
 * Fades in when saving starts and fades out after saving completes
 */
const SavingToast: React.FC<SavingToastProps> = ({ isSaving }) => {
  const [visible, setVisible] = useState(false);
  const isInitialMount = useRef(true);
  
  useEffect(() => {
    let fadeOutTimer: NodeJS.Timeout;
    
    // Skip showing toast on initial mount
    if (isInitialMount.current) {
      isInitialMount.current = false;
      return;
    }
    
    if (isSaving) {
      // Show immediately when saving starts
      setVisible(true);
    } else if (visible) {
      // When saving completes, wait a bit before fading out
      fadeOutTimer = setTimeout(() => {
        setVisible(false);
      }, 1000); // Keep visible for 1 second after saving completes
    }
    
    return () => {
      if (fadeOutTimer) clearTimeout(fadeOutTimer);
    };
  }, [isSaving, visible]);
  
  return (
    <Fade in={visible} timeout={{ enter: 300, exit: 1000 }}>
      <Box
        sx={{
          position: 'fixed',
          bottom: '20px',
          right: '20px',
          zIndex: 9999,
          display: 'flex',
          alignItems: 'center',
          backgroundColor: '#fff',
          borderRadius: '4px',
          boxShadow: '0 2px 10px rgba(0, 0, 0, 0.2)',
          padding: '10px 16px',
          minWidth: '140px',
        }}
      >
        <CircularProgress size={20} thickness={4} sx={{ mr: 1.5 }} />
        <Typography variant="body2" sx={{ color: '#000' }}>
          Saving app...
        </Typography>
      </Box>
    </Fade>
  );
};

export default SavingToast; 