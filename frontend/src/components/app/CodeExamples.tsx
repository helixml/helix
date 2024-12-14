import React, { FC, useState } from 'react';
import Box from '@mui/material/Box';
import Tab from '@mui/material/Tab';
import Grid from '@mui/material/Grid';
import Tabs from '@mui/material/Tabs';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { CODE_EXAMPLES } from '../../data/codeExamples';
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';

interface CodeExamplesProps {
  apiKey: string;
}

const CodeExamples: FC<CodeExamplesProps> = ({ apiKey }) => {
  const [selectedTab, setSelectedTab] = useState(0);

  const address = window.location.origin;

  const handleCopyCode = async () => {
    try {
      await navigator.clipboard.writeText(CODE_EXAMPLES[selectedTab].code(address, apiKey));
    } catch (err) {
      console.error('Failed to copy code:', err);
    }
  };

  return (
    <Grid item xs={12} md={6}>
      <Box sx={{ width: '100%' }}>
        <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 2 }}>
          <Tabs
            value={selectedTab}
            onChange={(_, newValue) => setSelectedTab(newValue)}
            aria-label="code examples tabs"
          >
            {CODE_EXAMPLES.map((example, index) => (
              <Tab key={index} label={example.label} />
            ))}
          </Tabs>
        </Box>

        {CODE_EXAMPLES.map((example, index) => (
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
                  }}
                >
                  {example.code(address, apiKey)}
                </SyntaxHighlighter>
              </>
            )}
          </Box>
        ))}
      </Box>
    </Grid>
  );
};

export default CodeExamples; 