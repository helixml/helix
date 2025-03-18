import React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';
import Link from '@mui/material/Link';
import JsonWindowLink from '../widgets/JsonWindowLink';

interface DevelopersSectionProps {
  schema: string;
  setSchema: (schema: string) => void;
  showErrors: boolean;
  appId: string;
  navigate: (route: string) => void;
}

const DevelopersSection: React.FC<DevelopersSectionProps> = ({
  schema,
  setSchema,
  showErrors,
  appId,
  navigate,
}) => {
  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="h6" sx={{mb: 1}}>
        App Configuration
      </Typography>
      <TextField
        error={showErrors && !schema}
        value={schema}
        onChange={(e) => setSchema(e.target.value)}
        disabled={true}
        fullWidth
        multiline
        rows={10}
        id="app-schema"
        name="app-schema"
        label="AISpec YAML for App"
        helperText={showErrors && !schema ? "Please enter a schema" : ""}
        InputProps={{
          style: { fontFamily: 'monospace' }
        }}
      />
      <Box sx={{ textAlign: 'right', mb: 1 }}>
        <JsonWindowLink
          sx={{textDecoration: 'underline'}}
          data={schema}
          withFancyRenderingControls={false}
        >
          expand
        </JsonWindowLink>
      </Box>
      <Typography variant="subtitle1" sx={{ mt: 4 }}>
        CLI Access
      </Typography>
      <Typography variant="body2" sx={{ mt: 1, mb: 2 }}>
        You can access this app configuration with the CLI command:
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
      <Typography variant="body2" sx={{ mt: 1, mb: 2 }}>
        Write it to a file, then deploy the app with the CLI with the following command:
      </Typography>
      <Box sx={{
        backgroundColor: '#1e1e2f',
        padding: '10px',
        borderRadius: '4px',
        fontFamily: 'monospace',
        fontSize: '0.9rem'
      }}>
        helix apply -f [filename].yaml
      </Box>
      <Typography variant="body2" sx={{ mt: 1, mb: 2 }}>
        To achieve GitOps for GenAI, put the file in version control and run <code>helix apply</code> from CI.
      </Typography>
      <Typography variant="body2" sx={{ mt: 2, mb: 1 }}>
        Don't have the CLI installed? 
        <Link 
          onClick={() => navigate('account')}
          sx={{ ml: 1, textDecoration: 'underline', cursor: 'pointer' }}
        >
          Install it from your account page
        </Link>
      </Typography>
    </Box>
  );
};

export default DevelopersSection;