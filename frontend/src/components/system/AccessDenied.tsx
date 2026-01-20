import React from 'react';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import LockIcon from '@mui/icons-material/Lock';
import HomeIcon from '@mui/icons-material/Home';
import useRouter from '../../hooks/useRouter';

const AccessDenied: React.FC = () => {
  const router = useRouter();

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'center',
        height: '100vh',
        gap: 2,
      }}
    >
      <LockIcon sx={{ fontSize: 64, color: 'text.secondary' }} />
      <Typography variant="h4" color="text.primary">
        Access Denied
      </Typography>
      <Typography variant="body1" color="text.secondary">
        You don't have access to view this app
      </Typography>
      <Button
        variant="contained"
        color="secondary"
        startIcon={<HomeIcon />}
        onClick={() => router.navigate('projects')}
        sx={{ mt: 2 }}
      >
        Back to Projects
      </Button>
    </Box>
  );
};

export default AccessDenied; 