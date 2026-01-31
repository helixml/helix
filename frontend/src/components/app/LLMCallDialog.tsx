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
} from '@mui/material';
import CloseIcon from '@mui/icons-material/Close';
import DarkDialog from '../dialog/DarkDialog';
import CopyButton from '../common/CopyButton';

interface LLMCall {
  id: string;
  created: string;
  duration_ms: number;
  step?: string;
  model?: string;
  response?: any;
  request?: any;
  provider?: string;
  prompt_tokens?: number;
  completion_tokens?: number;
  total_tokens?: number;
  error?: string;
}

interface LLMCallDialogProps {
  open: boolean;
  onClose: () => void;
  llmCall: LLMCall | null;
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
      id={`llm-call-tabpanel-${index}`}
      aria-labelledby={`llm-call-tab-${index}`}
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

const LLMCallDialog: React.FC<LLMCallDialogProps> = ({
  open,
  onClose,
  llmCall,
}) => {
  const theme = useTheme();
  const [tabValue, setTabValue] = React.useState(0);

  if (!llmCall) return null;

  const formatTime = (dateString: string) => {
    return new Date(dateString).toLocaleString();
  };

  const formatDuration = (ms: number) => {
    if (ms < 1000) return `${ms} ms`;
    return `${(ms / 1000).toFixed(2)} s`;
  };

  const parseJson = (data: any): any => {
    try {
      if (typeof data === 'string') {
        return JSON.parse(data);
      }
      return data;
    } catch (e) {
      return data;
    }
  };

  const formatJson = (data: any): string => {
    const parsed = parseJson(data);
    return JSON.stringify(parsed, null, 2);
  };

  const getRequestMessages = (request: any): any[] => {
    const parsed = parseJson(request);
    return parsed?.messages || [];
  };

  const getResponseContent = (response: any): string => {
    const parsed = parseJson(response);
    return parsed?.choices?.[0]?.message?.content || 'No content';
  };

  const getToolCalls = (response: any): any[] => {
    const parsed = parseJson(response);
    return parsed?.choices?.[0]?.message?.tool_calls || [];
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
            LLM Call: {llmCall.step || 'Unknown Step'}
          </Typography>
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
          <Box
            sx={{
              display: 'grid',
              gridTemplateColumns: { xs: '1fr', sm: '1fr 1fr' },
              gap: 2,
              mb: 2,
            }}
          >
            {/* Left column */}
            <Box>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Started:
                </Typography>
                <Typography variant="body2">
                  {llmCall.created ? formatTime(llmCall.created) : 'N/A'}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Duration:
                </Typography>
                <Typography variant="body2">
                  {llmCall.duration_ms ? formatDuration(llmCall.duration_ms) : 'N/A'}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Model:
                </Typography>
                <Typography variant="body2">
                  {llmCall.model || 'N/A'}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Provider:
                </Typography>
                <Typography variant="body2">
                  {llmCall.provider || 'N/A'}
                </Typography>
              </Box>

              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Messages:
                </Typography>
                <Typography variant="body2">
                  {llmCall.request ? (() => {
                    const messages = getRequestMessages(llmCall.request);
                    const counts = messages.reduce((acc, msg) => {
                      const role = msg.role?.toLowerCase() || 'unknown';
                      acc[role] = (acc[role] || 0) + 1;
                      return acc;
                    }, {} as Record<string, number>);
                    
                    const parts = [];
                    if (counts.system) parts.push(`System: ${counts.system}`);
                    if (counts.developer) parts.push(`Developer: ${counts.developer}`);
                    if (counts.user) parts.push(`User: ${counts.user}`);
                    if (counts.assistant) parts.push(`Assistant: ${counts.assistant}`);
                    if (counts.tool) parts.push(`Tool: ${counts.tool}`);
                    
                    return parts.length > 0 ? parts.join(' | ') : 'N/A';
                  })() : 'N/A'}
                </Typography>
              </Box>
            </Box>
            {/* Right column */}
            <Box>
              <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                  Status:
                </Typography>
                <Typography variant="body2" color={llmCall.error ? 'error' : 'success'}>
                  {llmCall.error ? 'Error' : 'Success'}
                </Typography>                
              </Box>

              {llmCall.prompt_tokens && (
                <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                  <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                    Prompt Tokens:
                  </Typography>
                  <Typography variant="body2">
                    {llmCall.prompt_tokens.toLocaleString()}
                  </Typography>
                </Box>
              )}

              {llmCall.completion_tokens && (
                <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                  <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                    Completion Tokens:
                  </Typography>
                  <Typography variant="body2">
                    {llmCall.completion_tokens.toLocaleString()}
                  </Typography>
                </Box>
              )}

              {llmCall.total_tokens && (
                <Box sx={{ display: 'flex', justifyContent: 'space-between', mb: 1 }}>
                  <Typography variant="body2" color="text.secondary" sx={{ fontWeight: 'bold' }}>
                    Total Tokens:
                  </Typography>
                  <Typography variant="body2">
                    {llmCall.total_tokens.toLocaleString()}
                  </Typography>
                </Box>
              )}

              
            </Box>
          </Box>
        </Box>

        {llmCall.error && (
          <Box sx={{ mb: 3 }}>
            <Typography variant="h6" color="error" sx={{ mb: 1 }}>
              Error
            </Typography>
            <Paper
              sx={{
                p: 2,
                backgroundColor: 'transparent',
                borderRadius: 1,
                border: '1px solid',
                borderColor: 'divider',
              }}
            >
              <Typography variant="body2" color="error">
                {llmCall.error}
              </Typography>
            </Paper>
          </Box>
        )}

        <Box sx={{ width: '100%' }}>
          <Box sx={{ borderBottom: 1, borderColor: 'divider' }}>
            <Tabs value={tabValue} onChange={handleTabChange} aria-label="LLM call tabs">
              <Tab label="Request" />
              <Tab label="Response" />
              <Tab label="Raw JSON" />
            </Tabs>
          </Box>

          <TabPanel value={tabValue} index={0}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              Request
            </Typography>
            {llmCall.request && (
              <JsonContentWithCopy
                content={formatJson(llmCall.request)} 
                title="Request"
              />
            )}
          </TabPanel>

          <TabPanel value={tabValue} index={1}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              Response
            </Typography>
            {llmCall.response && (
              <JsonContentWithCopy 
                content={formatJson(llmCall.response)} 
                title="Response"
              />
            )}
          </TabPanel>

          <TabPanel value={tabValue} index={2}>
            <Typography variant="h6" sx={{ mb: 2 }}>
              Raw JSON Data
            </Typography>
            <JsonContentWithCopy 
              content={JSON.stringify(llmCall, null, 2)} 
              title="Raw JSON"
            />
          </TabPanel>
        </Box>

        {llmCall.id && (
          <Box sx={{ mt: 3, pt: 2, borderTop: `1px solid ${theme.palette.divider}` }}>
            <Typography variant="caption" color="text.secondary">
              Call ID: {llmCall.id}
            </Typography>
          </Box>
        )}
      </DialogContent>
    </DarkDialog>
  );
};

export default LLMCallDialog; 