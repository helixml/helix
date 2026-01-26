/**
 * InsecureContextWarning - Shows when streaming requires HTTPS but page is HTTP
 *
 * WebCodecs and other streaming APIs require a secure context (HTTPS or localhost).
 * When accessed via HTTP on a non-localhost address, we show browser-specific
 * instructions on how to enable the insecure origin flag.
 */

import React, { useState } from 'react';
import { Box, Typography, Alert, Collapse, IconButton, Link } from '@mui/material';
import { ExpandMore, ExpandLess, Warning, ContentCopy, Check } from '@mui/icons-material';

interface BrowserInstructions {
  name: string;
  steps: string[];
  flagUrl?: string;
  notes?: string;
}

const getBrowserInstructions = (currentOrigin: string): BrowserInstructions[] => {
  return [
    {
      name: 'Google Chrome',
      flagUrl: 'chrome://flags/#unsafely-treat-insecure-origin-as-secure',
      steps: [
        'Open a new tab and paste the flag URL below',
        `Add "${currentOrigin}" to the text field`,
        'Set the dropdown to "Enabled"',
        'Click "Relaunch" at the bottom to restart Chrome',
        'Return to this page and refresh',
      ],
      notes: 'This setting persists across browser restarts.',
    },
    {
      name: 'Microsoft Edge',
      flagUrl: 'edge://flags/#unsafely-treat-insecure-origin-as-secure',
      steps: [
        'Open a new tab and paste the flag URL below',
        `Add "${currentOrigin}" to the text field`,
        'Set the dropdown to "Enabled"',
        'Click "Restart" at the bottom to restart Edge',
        'Return to this page and refresh',
      ],
      notes: 'This setting persists across browser restarts.',
    },
    {
      name: 'Mozilla Firefox',
      steps: [
        'Open a new tab and go to about:config',
        'Accept the risk warning if prompted',
        'Search for "dom.securecontext.allowlist"',
        `Add "${currentOrigin}" to the preference value (comma-separated if adding multiple)`,
        'Refresh this page',
      ],
      notes: 'Firefox requires version 97+ for this feature.',
    },
    {
      name: 'Safari',
      steps: [
        'Safari generally does not support treating HTTP as secure',
        'Use HTTPS for your development environment, or',
        'Access via localhost with port forwarding instead',
      ],
      notes: 'Safari has stricter security policies. Consider using Chrome or Firefox for development.',
    },
  ];
};

const InsecureContextWarning: React.FC = () => {
  const [expandedBrowser, setExpandedBrowser] = useState<string | null>('Google Chrome');
  const [copiedUrl, setCopiedUrl] = useState<string | null>(null);

  const currentOrigin = window.location.origin;
  const browserInstructions = getBrowserInstructions(currentOrigin);

  const handleCopyUrl = async (url: string) => {
    try {
      await navigator.clipboard.writeText(url);
      setCopiedUrl(url);
      setTimeout(() => setCopiedUrl(null), 2000);
    } catch {
      // Fallback for when clipboard API fails
      const textArea = document.createElement('textarea');
      textArea.value = url;
      document.body.appendChild(textArea);
      textArea.select();
      document.execCommand('copy');
      document.body.removeChild(textArea);
      setCopiedUrl(url);
      setTimeout(() => setCopiedUrl(null), 2000);
    }
  };

  return (
    <Box
      sx={{
        position: 'absolute',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        backgroundColor: 'rgba(0, 0, 0, 0.95)',
        zIndex: 1001,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'center',
        justifyContent: 'flex-start',
        textAlign: 'left',
        p: 3,
        overflow: 'auto',
      }}
    >
      <Box sx={{ maxWidth: 600, width: '100%' }}>
        <Alert
          severity="warning"
          icon={<Warning />}
          sx={{ mb: 3 }}
        >
          <Typography variant="subtitle1" fontWeight="bold">
            Secure Context Required for Desktop Streaming
          </Typography>
          <Typography variant="body2" sx={{ mt: 1 }}>
            Desktop streaming uses WebCodecs which requires HTTPS or localhost.
            You're accessing this page via HTTP ({currentOrigin}).
          </Typography>
        </Alert>

        <Typography variant="h6" sx={{ color: 'white', mb: 2 }}>
          How to Fix
        </Typography>

        <Typography variant="body2" sx={{ color: 'grey.400', mb: 3 }}>
          Choose your browser below for step-by-step instructions to enable streaming over HTTP:
        </Typography>

        {browserInstructions.map((browser) => (
          <Box
            key={browser.name}
            sx={{
              mb: 1,
              border: 1,
              borderColor: expandedBrowser === browser.name ? 'primary.main' : 'grey.700',
              borderRadius: 1,
              overflow: 'hidden',
              backgroundColor: 'grey.900',
            }}
          >
            <Box
              onClick={() => setExpandedBrowser(expandedBrowser === browser.name ? null : browser.name)}
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                p: 1.5,
                cursor: 'pointer',
                '&:hover': { backgroundColor: 'grey.800' },
              }}
            >
              <Typography variant="subtitle2" sx={{ color: 'white', fontWeight: 'medium' }}>
                {browser.name}
              </Typography>
              {expandedBrowser === browser.name ? (
                <ExpandLess sx={{ color: 'grey.400' }} />
              ) : (
                <ExpandMore sx={{ color: 'grey.400' }} />
              )}
            </Box>

            <Collapse in={expandedBrowser === browser.name}>
              <Box sx={{ p: 2, pt: 0 }}>
                {browser.flagUrl && (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 1,
                      p: 1.5,
                      mb: 2,
                      backgroundColor: 'grey.800',
                      borderRadius: 1,
                      fontFamily: 'monospace',
                    }}
                  >
                    <Typography
                      variant="body2"
                      sx={{
                        color: 'primary.light',
                        flex: 1,
                        wordBreak: 'break-all',
                      }}
                    >
                      {browser.flagUrl}
                    </Typography>
                    <IconButton
                      size="small"
                      onClick={() => handleCopyUrl(browser.flagUrl!)}
                      sx={{ color: copiedUrl === browser.flagUrl ? 'success.main' : 'grey.400' }}
                    >
                      {copiedUrl === browser.flagUrl ? <Check fontSize="small" /> : <ContentCopy fontSize="small" />}
                    </IconButton>
                  </Box>
                )}

                <Box component="ol" sx={{ m: 0, pl: 2.5, color: 'grey.300' }}>
                  {browser.steps.map((step, index) => (
                    <Typography
                      key={index}
                      component="li"
                      variant="body2"
                      sx={{ mb: 1, lineHeight: 1.6 }}
                    >
                      {step}
                    </Typography>
                  ))}
                </Box>

                {browser.notes && (
                  <Typography
                    variant="caption"
                    sx={{ display: 'block', mt: 2, color: 'grey.500', fontStyle: 'italic' }}
                  >
                    {browser.notes}
                  </Typography>
                )}
              </Box>
            </Collapse>
          </Box>
        ))}

        <Box sx={{ mt: 3, p: 2, backgroundColor: 'grey.900', borderRadius: 1 }}>
          <Typography variant="body2" sx={{ color: 'grey.400' }}>
            <strong style={{ color: '#90caf9' }}>Alternative:</strong> Access this page via{' '}
            <code style={{ backgroundColor: '#333', padding: '2px 6px', borderRadius: 4 }}>
              localhost
            </code>{' '}
            with port forwarding. Localhost is automatically treated as a secure context.
          </Typography>
          <Typography variant="caption" sx={{ display: 'block', mt: 1, color: 'grey.500' }}>
            Example: <code>ssh -L 8080:your-server-ip:8080 user@your-server</code>
          </Typography>
        </Box>

        <Typography variant="caption" sx={{ display: 'block', mt: 3, color: 'grey.600', textAlign: 'center' }}>
          After making changes, refresh this page to start streaming.
        </Typography>
      </Box>
    </Box>
  );
};

export default InsecureContextWarning;
