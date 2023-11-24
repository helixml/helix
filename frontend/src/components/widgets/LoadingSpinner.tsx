import React from 'react'
import Box from '@mui/material/Box'

const LoadingSpinner = () => {
  return (
    <Box sx={{
      display: 'flex',
      justifyContent: 'flex-start',
      alignItems: 'center',
      height: '60px'
    }}>
      <Box sx={dotStyle(0)} />
      <Box sx={dotStyle(0.2)} />
      <Box sx={dotStyle(0.4)} />
    </Box>
  );
};

const dotStyle = (delay: number) => ({
  width: '8px',
  height: '8px',
  backgroundColor: '#999',
  borderRadius: '50%',
  margin: '0 3px',
  animation: 'wave 0.9s infinite',
  animationDelay: `${delay}s`,
  '@keyframes wave': {
    '0%, 60%, 100%': {
      transform: 'translateY(0)',
    },
    '30%': {
      transform: 'translateY(-15px)',
    },
  }
});

export default LoadingSpinner;
