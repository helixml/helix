import React, { FC, useState, memo } from 'react';
import Box from '@mui/material/Box';
import Tab from '@mui/material/Tab';
import Tabs from '@mui/material/Tabs';
import Button from '@mui/material/Button';
import Typography from '@mui/material/Typography';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { API_CODE_EXAMPLES } from '../../data/apiCodeExamples';
import { Prism as SyntaxHighlighterPrism } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';
import { AdvancedModelPicker } from '../create/AdvancedModelPicker';

interface ApiCodeExamplesProps {
  apiKey: string;
  model?: string;
}

const SyntaxHighlighter = SyntaxHighlighterPrism as unknown as React.FC<any>;

function buildModelName(provider: string, modelId: string): string {
  if (!provider) return modelId;
  // If model already contains a slash (e.g. HuggingFace IDs like meta-llama/Llama-3),
  // still prefix with provider so the backend can route via ParseProviderFromModel
  return `${provider}/${modelId}`;
}

const ApiCodeExamples: FC<ApiCodeExamplesProps> = ({ apiKey, model }) => {
  const [selectedTab, setSelectedTab] = useState(0);
  const [selectedProvider, setSelectedProvider] = useState('');
  const [selectedModelId, setSelectedModelId] = useState(model || '');

  const url = window.location.origin;

  const effectiveModel = model || buildModelName(selectedProvider, selectedModelId);

  const handleSelectModel = (provider: string, modelId: string) => {
    setSelectedProvider(provider);
    setSelectedModelId(modelId);
  };

  const handleCopyCode = async () => {
    try {
      await navigator.clipboard.writeText(API_CODE_EXAMPLES[selectedTab].code(url, apiKey, effectiveModel));
    } catch (err) {
      console.error('Failed to copy code:', err);
    }
  };

  return (
    <Box sx={{ width: '100%' }}>
      {!model && (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 2 }}>
          <Typography variant="body2" color="text.secondary">Model:</Typography>
          <AdvancedModelPicker
            selectedModelId={selectedModelId}
            selectedProvider={selectedProvider}
            onSelectModel={handleSelectModel}
            currentType="chat"
            displayMode="full"
            autoSelectFirst={true}
          />
        </Box>
      )}

      <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
        <Tabs
          value={selectedTab}
          onChange={(_, newValue) => setSelectedTab(newValue)}
          aria-label="API code examples"
          variant="scrollable"
          scrollButtons="auto"
        >
          {API_CODE_EXAMPLES.map((example, index) => (
            <Tab key={index} label={example.label} />
          ))}
        </Tabs>
      </Box>

      {API_CODE_EXAMPLES.map((example, index) => (
        <Box
          key={index}
          role="tabpanel"
          hidden={selectedTab !== index}
          sx={{ position: 'relative' }}
        >
          {selectedTab === index && (
            <>
              <Box sx={{ position: 'absolute', right: 2, top: 2, zIndex: 1 }}>
                <Button
                  size="small"
                  onClick={handleCopyCode}
                  startIcon={<ContentCopyIcon />}
                  sx={{
                    backgroundColor: 'rgba(0, 0, 0, 0.6)',
                    '&:hover': {
                      backgroundColor: 'rgba(0, 0, 0, 0.8)',
                    }
                  }}
                >
                  Copy
                </Button>
              </Box>
              <SyntaxHighlighter
                language={example.language}
                style={oneDark}
                customStyle={{
                  margin: 0,
                  borderRadius: '4px',
                  maxHeight: '70vh',
                }}
              >
                {example.code(url, apiKey, effectiveModel)}
              </SyntaxHighlighter>
            </>
          )}
        </Box>
      ))}
    </Box>
  );
};

export default memo(ApiCodeExamples);
