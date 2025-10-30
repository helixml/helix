import React, { useState, useEffect } from 'react';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import InfoIcon from '@mui/icons-material/Info';
import useAccount from '../../hooks/useAccount';

export const TokenExpiryCounter: React.FC = () => {
  const account = useAccount();
  const [timeRemaining, setTimeRemaining] = useState<string>('');

  useEffect(() => {
    const updateTimer = () => {
      try {
        // Check token in memory (from axios/account)
        const token = account.user?.token;
        if (token) {
          const payload = JSON.parse(atob(token.split('.')[1]));
          const expiry = new Date(payload.exp * 1000);
          const now = new Date();
          const secondsRemaining = Math.floor((expiry.getTime() - now.getTime()) / 1000);

          if (secondsRemaining > 0) {
            const minutes = Math.floor(secondsRemaining / 60);
            const seconds = secondsRemaining % 60;
            setTimeRemaining(`${minutes}m ${seconds}s`);
          } else {
            setTimeRemaining('EXPIRED!');
          }
        }

        // Note: access_token cookie is HttpOnly so we can't read it via document.cookie
        // The cookie exists and is used by backend, we just can't see it from JS
      } catch (e) {
        setTimeRemaining('');
      }
    };

    updateTimer();
    const interval = setInterval(updateTimer, 1000);
    return () => clearInterval(interval);
  }, [account.user?.token]);

  if (!timeRemaining) return null;

  return (
    <Tooltip title={`Token expires in: ${timeRemaining}`} placement="bottom">
      <IconButton
        size="small"
        sx={{
          ml: 0.5,
          p: 0.25,
          color: '#fff',
          opacity: 0.6,
          minWidth: 'auto',
          width: 'auto',
          '&:hover': {
            opacity: 1,
            bgcolor: 'rgba(255, 255, 255, 0.1)'
          }
        }}
      >
        <InfoIcon sx={{ fontSize: '0.9rem' }} />
      </IconButton>
    </Tooltip>
  );
};

export default TokenExpiryCounter;
