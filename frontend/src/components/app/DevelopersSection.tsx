import React from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Link from '@mui/material/Link';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import DownloadIcon from '@mui/icons-material/Download';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import CheckIcon from '@mui/icons-material/Check';
import MonacoEditor from '../widgets/MonacoEditor';
import { generateYamlFilename } from '../../utils/format';
import useSnackbar from '../../hooks/useSnackbar';

interface DevelopersSectionProps {
  schema: string;
  setSchema: (schema: string) => void;
  showErrors: boolean;
  appId: string;
  appName?: string;
  navigate: (route: string) => void;
}

const DevelopersSection: React.FC<DevelopersSectionProps> = ({
  schema,
  setSchema,
  showErrors,
  appId,
  appName,
  navigate,
}) => {
  const yamlFilename = generateYamlFilename(appName || 'app');
  const snackbar = useSnackbar();
  const [inspectCopied, setInspectCopied] = React.useState(false);
  const [applyCopied, setApplyCopied] = React.useState(false);

  const handleDownloadYaml = () => {
    const blob = new Blob([schema], { type: 'text/yaml' });
    const url = URL.createObjectURL(blob);
    const link = document.createElement('a');
    link.href = url;
    link.download = yamlFilename;
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
    URL.revokeObjectURL(url);
  };

  const handleCopyCommand = (command: string, setCopied: (value: boolean) => void) => {
    navigator.clipboard.writeText(command)
      .then(() => {
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
        snackbar.success('Command copied to clipboard');
      })
      .catch((error) => {
        console.error('Failed to copy:', error);
        snackbar.error('Failed to copy to clipboard');
      });
  };

  const inspectCommand = `helix agent inspect ${appId}`;
  const applyCommand = `helix apply -f ${yamlFilename}`;

  return (
    <Box sx={{ mt: 2, mr: 4 }}>
      <Typography variant="h6" sx={{mb: 1}}>
        Agent Spec
      </Typography>
      <Box sx={{ mb: 1 }}>
        {showErrors && !schema && (
          <Typography variant="caption" sx={{ color: 'error.main', display: 'block', mb: 1 }}>
            Please enter a schema
          </Typography>
        )}
        <MonacoEditor
          value={schema}
          onChange={setSchema}
          language="yaml"
          readOnly={true}
          autoHeight={true}
          minHeight={200}
          maxHeight={600}
          theme="helix-dark"
          options={{
            fontSize: 14,
            lineNumbers: 'on',
            folding: true,
            lineDecorationsWidth: 0,
            lineNumbersMinChars: 3,
            scrollBeyondLastLine: false,
            minimap: { enabled: false },
            wordWrap: 'on',
            wrappingIndent: 'indent',
          }}
        />
      </Box>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1, mt: 2 }}>
        <Button
          startIcon={<DownloadIcon />}
          onClick={handleDownloadYaml}
          variant="outlined"
          size="small"
        >
          Download {yamlFilename}
        </Button>
      </Box>
      <Typography variant="subtitle1" sx={{ mt: 4 }}>
        CLI Access
      </Typography>
      <Typography variant="body2" sx={{ mt: 1, mb: 2 }}>
        You can access this agent configuration with the CLI command:
      </Typography>
      <Box sx={{
        backgroundColor: '#1e1e2f',
        padding: '10px',
        borderRadius: '4px',
        fontFamily: 'monospace',
        fontSize: '0.9rem',
        position: 'relative',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between'
      }}>
        <span>{inspectCommand}</span>
        <Tooltip title={inspectCopied ? "Copied!" : "Copy command"} placement="top">
          <IconButton
            onClick={() => handleCopyCommand(inspectCommand, setInspectCopied)}
            sx={{
              color: 'white',
              padding: '4px',
              '&:hover': {
                backgroundColor: 'rgba(255, 255, 255, 0.1)',
              },
            }}
            size="small"
          >
            {inspectCopied ? <CheckIcon sx={{ fontSize: 16 }} /> : <ContentCopyIcon sx={{ fontSize: 16 }} />}
          </IconButton>
        </Tooltip>
      </Box>
      <Typography variant="body2" sx={{ mt: 1, mb: 2 }}>
        Write it to a file, then deploy the agent with the CLI with the following command:
      </Typography>
      <Box sx={{
        backgroundColor: '#1e1e2f',
        padding: '10px',
        borderRadius: '4px',
        fontFamily: 'monospace',
        fontSize: '0.9rem',
        position: 'relative',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'space-between'
      }}>
        <span>{applyCommand}</span>
        <Tooltip title={applyCopied ? "Copied!" : "Copy command"} placement="top">
          <IconButton
            onClick={() => handleCopyCommand(applyCommand, setApplyCopied)}
            sx={{
              color: 'white',
              padding: '4px',
              '&:hover': {
                backgroundColor: 'rgba(255, 255, 255, 0.1)',
              },
            }}
            size="small"
          >
            {applyCopied ? <CheckIcon sx={{ fontSize: 16 }} /> : <ContentCopyIcon sx={{ fontSize: 16 }} />}
          </IconButton>
        </Tooltip>
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