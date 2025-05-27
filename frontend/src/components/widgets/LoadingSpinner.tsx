import React from 'react'
import Box from '@mui/material/Box'

const LoadingSpinner = () => {
  return (
    <Box sx={{
      display: 'flex',
      justifyContent: 'flex-start',
      alignItems: 'center',
      height: '20px',
      width: '20px',
      p: 0,
      m: 0,
    }}>
      <Box sx={dotStyle(0)} />
      <Box sx={dotStyle(0.2)} />
      <Box sx={dotStyle(0.4)} />
    </Box>
  );
};

const dotStyle = (delay: number) => ({
  width: '4px',
  height: '4px',
  backgroundColor: '#999',
  borderRadius: '50%',
  margin: '0 1px',
  animation: 'wave 0.9s infinite',
  animationDelay: `${delay}s`,
  '@keyframes wave': {
    '0%, 60%, 100%': {
      transform: 'translateY(0)',
    },
    '30%': {
      transform: 'translateY(-6px)',
    },
  }
});

export default LoadingSpinner;
