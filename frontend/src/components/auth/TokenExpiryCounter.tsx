import React, { useState, useEffect } from 'react';
import Typography from '@mui/material/Typography';
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
    <Typography
      variant="caption"
      sx={{
        fontSize: '0.7rem',
        color: 'text.secondary',
        opacity: 0.7,
        ml: 1
      }}
    >
      Token: {timeRemaining}
    </Typography>
  );
};

export default TokenExpiryCounter;
