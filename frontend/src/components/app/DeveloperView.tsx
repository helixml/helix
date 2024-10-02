import React from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import TextField from '@mui/material/TextField';
import Link from '@mui/material/Link';
import JsonWindowLink from '../widgets/JsonWindowLink';

interface DeveloperViewProps {
  schema: string;
  showErrors: boolean;
  appId: string;
  onNavigate: (page: string) => void;
}

const DeveloperView: React.FC<DeveloperViewProps> = ({
  schema,
  showErrors,
  appId,
  onNavigate,
}) => {
  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="h6" sx={{mb: 1}}>
        App Configuration
      </Typography>
      <TextField
        error={showErrors && !schema}
        value={schema}
        disabled={true}
        fullWidth
        multiline
        rows={10}
        id="app-schema"
        name="app-schema"
        label="App Configuration"
        helperText={showErrors && !schema ? "Please enter a schema" : ""}
        InputProps={{
          style: { fontFamily: 'monospace' }
        }}
      />
      <Box sx={{ textAlign: 'right', mb: 1 }}>
        <JsonWindowLink
          sx={{textDecoration: 'underline'}}
          data={schema}
        >
          expand
        </JsonWindowLink>
      </Box>
      <Typography variant="subtitle1" sx={{ mt: 4 }}>
        CLI Access
      </Typography>
      <Typography variant="body2" sx={{ mt: 1, mb: 2 }}>
        You can also access this app configuration with the CLI command:
      </Typography>
      <Box sx={{ 
        backgroundColor: '#1e1e2f', 
        padding: '10px', 
        borderRadius: '4px',
        fontFamily: 'monospace',
        fontSize: '0.9rem'
      }}>
        helix app inspect {appId}
      </Box>
      <Typography variant="body2" sx={{ mt: 2, mb: 1 }}>
        Don't have the CLI installed? 
        <Link 
          onClick={() => onNavigate('account')}
          sx={{ ml: 1, textDecoration: 'underline', cursor: 'pointer' }}
        >
          Install it from your account page
        </Link>
      </Typography>
    </Box>
  );
};

export default DeveloperView;