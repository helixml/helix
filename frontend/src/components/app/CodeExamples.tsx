import React, { FC, useState, useEffect, useRef, useCallback, memo } from 'react';
import Box from '@mui/material/Box';
import Tab from '@mui/material/Tab';
import Grid from '@mui/material/Grid';
import Tabs from '@mui/material/Tabs';
import Tooltip from '@mui/material/Tooltip';
import Button from '@mui/material/Button';
import IconButton from '@mui/material/IconButton';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import { Eye, EyeOff } from 'lucide-react';
import { CODE_EXAMPLES } from '../../data/codeExamples';
import { Prism as SyntaxHighlighterPrism } from 'react-syntax-highlighter';
import { oneDark } from 'react-syntax-highlighter/dist/esm/styles/prism';

interface CodeExamplesProps {
  apiKey: string;
}

const SyntaxHighlighter = SyntaxHighlighterPrism as unknown as React.FC<any>;

const MASKED_KEY_PLACEHOLDER = 'hl-••••••••';
const AUTO_HIDE_MS = 30_000;

const CodeExamples: FC<CodeExamplesProps> = ({ apiKey }) => {
  const [selectedTab, setSelectedTab] = useState(0);
  const [showKey, setShowKey] = useState(false);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearHideTimer = useCallback(() => {
    if (hideTimerRef.current) {
      clearTimeout(hideTimerRef.current);
      hideTimerRef.current = null;
    }
  }, []);

  // Auto-hide after 30s
  useEffect(() => {
    if (!showKey) return;
    hideTimerRef.current = setTimeout(() => {
      setShowKey(false);
      hideTimerRef.current = null;
    }, AUTO_HIDE_MS);
    return clearHideTimer;
  }, [showKey, clearHideTimer]);

  // Hide on tab/window blur
  useEffect(() => {
    if (!showKey) return;
    const onVisibility = () => {
      if (document.visibilityState === 'hidden') {
        setShowKey(false);
        clearHideTimer();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, [showKey, clearHideTimer]);

  const address = window.location.origin;

  // Always copy with the real key, regardless of reveal state
  const handleCopyCode = async () => {
    try {
      await navigator.clipboard.writeText(CODE_EXAMPLES[selectedTab].code(address, apiKey));
    } catch (err) {
      console.error('Failed to copy code:', err);
    }
  };

  const displayKey = showKey ? apiKey : MASKED_KEY_PLACEHOLDER;

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
                <Box
                  sx={{
                    position: 'absolute',
                    right: 2,
                    top: 2,
                    zIndex: 1,
                    display: 'flex',
                    gap: 0.5,
                    alignItems: 'center',
                  }}
                >
                  <Tooltip title={showKey ? 'Hide key in code' : 'Show key in code'}>
                    <IconButton
                      size="small"
                      onClick={() => setShowKey((prev) => !prev)}
                      aria-label={showKey ? 'Hide API key in code examples' : 'Show API key in code examples'}
                      sx={{
                        color: '#F8FAFC',
                        backgroundColor: 'rgba(0, 0, 0, 0.6)',
                        '&:hover': { backgroundColor: 'rgba(0, 0, 0, 0.8)' },
                      }}
                    >
                      {showKey ? <EyeOff size={16} /> : <Eye size={16} />}
                    </IconButton>
                  </Tooltip>
                  <Button
                    size="small"
                    onClick={handleCopyCode}
                    startIcon={<ContentCopyIcon />}
                    sx={{
                      color: '#F8FAFC',
                      backgroundColor: 'rgba(0, 0, 0, 0.6)',
                      '&:hover': {
                        backgroundColor: 'rgba(0, 0, 0, 0.8)',
                      },
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
                  {example.code(address, displayKey)}
                </SyntaxHighlighter>
              </>
            )}
          </Box>
        ))}
      </Box>
    </Grid>
  );
};

export default memo(CodeExamples);
