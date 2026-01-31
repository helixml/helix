import React from 'react';
import {
  DialogTitle,
  DialogContent,
  IconButton,
  Typography,
  Box,
  Chip,
  Divider,
  Paper,
  useTheme,
  Tabs,
  Tab,
  Grid,
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DarkDialog from '../dialog/DarkDialog';
import CopyButton from '../common/CopyButton';
import { TypesInteraction, TypesLLMCall } from '../../api/api';

interface InteractionDialogProps {
  open: boolean;
  onClose: () => void;
  interaction: TypesInteraction | null;
  llmCalls: TypesLLMCall[];
}

interface TabPanelProps {
  children?: React.ReactNode;
  index: number;
  value: number;
}

function TabPanel(props: TabPanelProps) {
  const { children, value, index, ...other } = props;

  return (
    <div
      role="tabpanel"
      hidden={value !== index}
      id={`interaction-tabpanel-${index}`}
      aria-labelledby={`interaction-tab-${index}`}
      {...other}
    >
      {value === index && <Box sx={{ pt: 2 }}>{children}</Box>}
    </div>
  );
}

const JsonContentWithCopy: React.FC<{ content: string; title: string }> = ({ content, title }) => {
  return (
    <Box sx={{ position: 'relative' }}>
      <CopyButton content={content} title={title} />
      <Paper
        sx={{
          p: 2,
          backgroundColor: 'transparent',
          borderRadius: 1,
          border: '1px solid',
          borderColor: 'divider',
          fontFamily: 'monospace',
          fontSize: '0.875rem',
          whiteSpace: 'pre-wrap',
          wordBreak: 'break-word',
          maxHeight: '600px',
          overflow: 'auto',
        }}
      >
        {content}
      </Paper>
    </Box>
  );
};

const InteractionDialog: React.FC<InteractionDialogProps> = ({
  open,
  onClose,
  interaction,
  llmCalls,
}) => {
  const theme = useTheme();
  const [tabValue, setTabValue] = React.useState(0);

  if (!interaction) return null;

  const formatTime = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms} ms`;
    return `${(ms / 1000).toFixed(2)} s`;
  };

  // Calculate token totals from LLM calls
  const tokenTotals = llmCalls.reduce(
    (totals: { prompt_tokens: number; completion_tokens: number; total_tokens: number }, call) => ({
      prompt_tokens: totals.prompt_tokens + (call.prompt_tokens || 0),
      completion_tokens: totals.completion_tokens + (call.completion_tokens || 0),
      total_tokens: totals.total_tokens + (call.total_tokens || 0),
    }),
    { prompt_tokens: 0, completion_tokens: 0, total_tokens: 0 }
  );

  // Get the first LLM call to extract prompt and response
  const firstCall = llmCalls.length > 0 ? llmCalls[0] : null;
  
  const getPromptMessage = (interaction: TypesInteraction): string => {
    // If we have a single content, return the text
    if (interaction.prompt_message) {
      return interaction.prompt_message;
    }
    // If we have multi-content, return the first one
    if (interaction.prompt_message_content?.parts?.length) {
      return interaction.prompt_message_content.parts[0].text;
    }
    return 'No prompt available';
  };

  const getResponseMessage = (interaction: TypesInteraction): string => {
    if (interaction.response_message) {
      return interaction.response_message;
    }
    return 'No response available';
  };

  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    setTabValue(newValue);
  };

  return (
    <DarkDialog
      open={open}
      onClose={onClose}
      maxWidth="lg"
      fullWidth
      PaperProps={{
        sx: {
          maxHeight: '90vh',
        },
      }}
    >
      <DialogTitle sx={{ m: 0, p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography variant="h6" component="div">
            Interaction Details
          </Typography>
          <Chip 
            label={interaction.state || 'unknown'} 
            color={interaction.state === 'error' ? 'error' : 'secondary'}
            size="small"
          />
        </Box>
        <IconButton
          aria-label="close"
          onClick={onClose}
          sx={{ color: theme.palette.grey[500] }}
        >
          <CloseIcon />
        </IconButton>
      </DialogTitle>

      <DialogContent sx={{ p: 3 }}>
        <Box sx={{ mb: 3 }}>
          <Grid container spacing={2}>
            <Grid item xs={12} sm={6}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Started:
                </Typography>
                <Typography variant="body2">
                  {interaction.created ? formatTime(interaction.created) : 'N/A'}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Duration:
                </Typography>
                <Typography variant="body2">
                  {interaction.duration_ms ? formatDuration(interaction.duration_ms) : 'N/A'}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Session ID:
                </Typography>
                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                  {interaction.session_id || 'N/A'}
                </Typography>
              </Box>
            </Grid>
            <Grid item xs={12} sm={6}>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Total Prompt Tokens:
                </Typography>
                <Typography variant="body2">
                  {tokenTotals.prompt_tokens.toLocaleString()}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Total Completion Tokens:
                </Typography>
                <Typography variant="body2">
                  {tokenTotals.completion_tokens.toLocaleString()}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Total Tokens:
                </Typography>
                <Typography variant="body2">
                  {tokenTotals.total_tokens.toLocaleString()}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  LLM Calls:
                </Typography>
                <Typography variant="body2">
                  {llmCalls.length}
                </Typography>
              </Box>
            </Grid>
          </Grid>
        </Box>

        <Box sx={{ width: '100%' }}>
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <Tabs value={tabValue} onChange={handleTabChange} aria-label="Interaction tabs">
              <Tab label="Prompt" />
              <Tab label="Response" />
              <Tab label="Raw Data" />
            </Tabs>
          </Box>

          <TabPanel value={tabValue} index={0}>           
            <JsonContentWithCopy
              content={getPromptMessage(interaction)}
              title="Prompt"
            />
          </TabPanel>

          <TabPanel value={tabValue} index={1}>
            <JsonContentWithCopy
              content={getResponseMessage(interaction)}
              title="Response"
            />
          </TabPanel>

          <TabPanel value={tabValue} index={2}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              Raw Interaction Data
            </Typography>
            <JsonContentWithCopy
              content={JSON.stringify(interaction, null, 2)}
              title="Raw Interaction"
            />
          </TabPanel>
        </Box>

        {interaction.id && (
          <Box sx={{ mt: 3, pt: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
            <Typography variant="caption" color="text.secondary">
              Interaction ID: {interaction.id}
            </Typography>
          </Box>
        )}
      </DialogContent>
    </DarkDialog>
  );
};

export default InteractionDialog; 