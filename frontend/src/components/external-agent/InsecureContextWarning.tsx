/**
 * InsecureContextWarning - Shows when streaming requires HTTPS but page is HTTP
 *
 * WebCodecs and other streaming APIs require a secure context (HTTPS or localhost).
 * When accessed via HTTP on a non-localhost address, we show browser-specific
 * instructions on how to enable the insecure origin flag.
 */

import React, { useState } from 'react';
import { Box, Typography, Alert, Collapse, IconButton } from '@mui/material';
import { ExpandMore, ExpandLess, Warning, ContentCopy, Check } from '@mui/icons-material';

interface BrowserInstructions {
  name: string;
  flagUrl?: string;
  notes?: string;
}

const browserConfigs: BrowserInstructions[] = [
  {
    name: 'Google Chrome',
    flagUrl: 'chrome://flags/#unsafely-treat-insecure-origin-as-secure',
    notes: 'This setting persists across browser restarts.',
  },
  {
    name: 'Microsoft Edge',
    flagUrl: 'edge://flags/#unsafely-treat-insecure-origin-as-secure',
    notes: 'This setting persists across browser restarts.',
  },
  {
    name: 'Mozilla Firefox',
    notes: 'Firefox requires version 97+ for this feature.',
  },
  {
    name: 'Safari',
    notes: 'Safari has stricter security policies. Use the localhost port-forwarding method below instead.',
  },
];

/** Copyable code block with copy button */
const CopyableCode: React.FC<{
  value: string;
  copied: string | null;
  onCopy: (value: string) => void;
  label?: string;
}> = ({ value, copied, onCopy, label }) => (
  <Box sx={{ mb: 2 }}>
    {label && (
      <Typography variant="caption" sx={{ color: 'grey.500', display: 'block', mb: 0.5 }}>
        {label}
      </Typography>
    )}
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        gap: 1,
        p: 1.5,
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
          userSelect: 'all',
        }}
      >
        {value}
      </Typography>
      <IconButton
        size="small"
        onClick={() => onCopy(value)}
        sx={{ color: copied === value ? 'success.main' : 'grey.400', flexShrink: 0 }}
      >
        {copied === value ? <Check fontSize="small" /> : <ContentCopy fontSize="small" />}
      </IconButton>
    </Box>
  </Box>
);

const InsecureContextWarning: React.FC = () => {
  const [expandedBrowser, setExpandedBrowser] = useState<string | null>('Google Chrome');
  const [showPortForwarding, setShowPortForwarding] = useState(false);
  const [copiedValue, setCopiedValue] = useState<string | null>(null);

  const currentOrigin = window.location.origin;
  // Extract host and port for SSH command
  const url = new URL(currentOrigin);
  const serverHost = url.hostname;
  const serverPort = url.port || '80';

  const handleCopy = async (value: string) => {
    try {
      await navigator.clipboard.writeText(value);
      setCopiedValue(value);
      setTimeout(() => setCopiedValue(null), 2000);
    } catch {
      // Fallback for when clipboard API fails (we're in insecure context after all!)
      const textArea = document.createElement('textarea');
      textArea.value = value;
      textArea.style.position = 'fixed';
      textArea.style.left = '-9999px';
      document.body.appendChild(textArea);
      textArea.select();
      document.execCommand('copy');
      document.body.removeChild(textArea);
      setCopiedValue(value);
      setTimeout(() => setCopiedValue(null), 2000);
    }
  };

  const renderBrowserSteps = (browser: BrowserInstructions) => {
    if (browser.name === 'Mozilla Firefox') {
      return (
        <Box sx={{ p: 2, pt: 0 }}>
          <Box component="ol" sx={{ m: 0, pl: 2.5, color: 'grey.300' }}>
            <Typography component="li" variant="body2" sx={{ mb: 1, lineHeight: 1.6 }}>
              Open a new tab and go to <code style={{ backgroundColor: '#333', padding: '2px 6px', borderRadius: 4 }}>about:config</code>
            </Typography>
            <Typography component="li" variant="body2" sx={{ mb: 1, lineHeight: 1.6 }}>
              Accept the risk warning if prompted
            </Typography>
            <Typography component="li" variant="body2" sx={{ mb: 1, lineHeight: 1.6 }}>
              Search for <code style={{ backgroundColor: '#333', padding: '2px 6px', borderRadius: 4 }}>dom.securecontext.allowlist</code>
            </Typography>
            <Typography component="li" variant="body2" sx={{ mb: 1, lineHeight: 1.6 }}>
              Add this origin to the preference value (comma-separated if adding multiple):
            </Typography>
          </Box>
          <Box sx={{ pl: 2.5 }}>
            <CopyableCode value={currentOrigin} copied={copiedValue} onCopy={handleCopy} />
          </Box>
          <Box component="ol" start={5} sx={{ m: 0, pl: 2.5, color: 'grey.300' }}>
            <Typography component="li" variant="body2" sx={{ mb: 1, lineHeight: 1.6 }}>
              Refresh this page
            </Typography>
          </Box>
          {browser.notes && (
            <Typography variant="caption" sx={{ display: 'block', mt: 2, color: 'grey.500', fontStyle: 'italic' }}>
              {browser.notes}
            </Typography>
          )}
        </Box>
      );
    }

    if (browser.name === 'Safari') {
      return (
        <Box sx={{ p: 2, pt: 0 }}>
          <Typography variant="body2" sx={{ color: 'grey.300', mb: 2 }}>
            Safari does not support treating HTTP origins as secure. Use the <strong>localhost port-forwarding method</strong> below instead.
          </Typography>
          {browser.notes && (
            <Typography variant="caption" sx={{ display: 'block', color: 'grey.500', fontStyle: 'italic' }}>
              {browser.notes}
            </Typography>
          )}
        </Box>
      );
    }

    // Chrome and Edge
    return (
      <Box sx={{ p: 2, pt: 0 }}>
        <CopyableCode value={browser.flagUrl!} copied={copiedValue} onCopy={handleCopy} label="1. Copy this URL and paste it in a new tab:" />

        <Typography variant="body2" sx={{ color: 'grey.300', mb: 1 }}>
          2. In the text field, add this origin:
        </Typography>
        <CopyableCode value={currentOrigin} copied={copiedValue} onCopy={handleCopy} />

        <Box component="ol" start={3} sx={{ m: 0, pl: 2.5, color: 'grey.300' }}>
          <Typography component="li" variant="body2" sx={{ mb: 1, lineHeight: 1.6 }}>
            Set the dropdown to <strong>"Enabled"</strong>
          </Typography>
          <Typography component="li" variant="body2" sx={{ mb: 1, lineHeight: 1.6 }}>
            Click <strong>"Relaunch"</strong> at the bottom to restart the browser
          </Typography>
          <Typography component="li" variant="body2" sx={{ mb: 1, lineHeight: 1.6 }}>
            Return to this page and refresh
          </Typography>
        </Box>

        {browser.notes && (
          <Typography variant="caption" sx={{ display: 'block', mt: 2, color: 'grey.500', fontStyle: 'italic' }}>
            {browser.notes}
          </Typography>
        )}
      </Box>
    );
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
        p: 2,
        overflowY: 'auto',
        overflowX: 'hidden',
        // Ensure scrolling works on all browsers
        WebkitOverflowScrolling: 'touch',
      }}
    >
      <Box sx={{ maxWidth: 600, width: '100%' }}>
        <Alert severity="warning" icon={<Warning />} sx={{ mb: 3 }}>
          <Typography variant="subtitle1" fontWeight="bold">
            Secure Context Required for Desktop Streaming
          </Typography>
          <Typography variant="body2" sx={{ mt: 1 }}>
            Desktop streaming uses WebCodecs which requires HTTPS or localhost.
            You're accessing via HTTP at:
          </Typography>
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1,
              mt: 1,
              p: 1,
              backgroundColor: 'rgba(0,0,0,0.2)',
              borderRadius: 1,
              fontFamily: 'monospace',
            }}
          >
            <Typography variant="body2" sx={{ flex: 1, wordBreak: 'break-all', userSelect: 'all' }}>
              {currentOrigin}
            </Typography>
            <IconButton
              size="small"
              onClick={() => handleCopy(currentOrigin)}
              sx={{ color: copiedValue === currentOrigin ? 'success.main' : 'inherit', flexShrink: 0 }}
            >
              {copiedValue === currentOrigin ? <Check fontSize="small" /> : <ContentCopy fontSize="small" />}
            </IconButton>
          </Box>
        </Alert>

        <Typography variant="body2" sx={{ color: 'grey.400', mb: 2 }}>
          Select your browser to enable streaming over HTTP:
        </Typography>

        {browserConfigs.map((browser) => (
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
              {renderBrowserSteps(browser)}
            </Collapse>
          </Box>
        ))}

        {/* Port Forwarding Option - Collapsible */}
        <Box
          sx={{
            mt: 2,
            border: 1,
            borderColor: showPortForwarding ? 'primary.main' : 'grey.700',
            borderRadius: 1,
            overflow: 'hidden',
            backgroundColor: 'grey.900',
          }}
        >
          <Box
            onClick={() => setShowPortForwarding(!showPortForwarding)}
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
              Alternative: Localhost Port Forwarding (works with Safari)
            </Typography>
            {showPortForwarding ? (
              <ExpandLess sx={{ color: 'grey.400' }} />
            ) : (
              <ExpandMore sx={{ color: 'grey.400' }} />
            )}
          </Box>

          <Collapse in={showPortForwarding}>
            <Box sx={{ p: 2, pt: 0 }}>
              <Typography variant="body2" sx={{ color: 'grey.300', mb: 2 }}>
                Use SSH to forward a local port to the server. Run in terminal:
              </Typography>
              <CopyableCode
                value={`ssh -L 8080:${serverHost}:${serverPort} user@${serverHost}`}
                copied={copiedValue}
                onCopy={handleCopy}
              />
              <Typography variant="body2" sx={{ color: 'grey.400', mb: 1 }}>
                Then access: <code style={{ backgroundColor: '#333', padding: '2px 6px', borderRadius: 4 }}>http://localhost:8080</code>
              </Typography>
            </Box>
          </Collapse>
        </Box>

        <Typography variant="caption" sx={{ display: 'block', mt: 2, color: 'grey.600', textAlign: 'center' }}>
          After making changes, refresh this page.
        </Typography>
      </Box>
    </Box>
  );
};

export default InsecureContextWarning;
