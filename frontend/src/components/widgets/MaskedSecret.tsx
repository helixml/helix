import React, { FC, useCallback, useEffect, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import { Copy, Check, Eye, EyeOff } from 'lucide-react';

const MASK = '•'.repeat(16); // 16 bullets, fixed width — does not encode key length
const DEFAULT_AUTO_HIDE_MS = 30_000;

const PALETTE = {
  text: '#F8FAFC',
  border: '#4A5568',
  borderHover: '#718096',
  borderFocus: '#3182CE',
  iconMuted: '#A0AEC0',
};

export interface MaskedSecretProps {
  value: string;
  monospace?: boolean;
  copyable?: boolean;
  revealable?: boolean;
  autoHideMs?: number;
  ariaLabel?: string;
}

const MaskedSecret: FC<MaskedSecretProps> = ({
  value,
  monospace = true,
  copyable = true,
  revealable = true,
  autoHideMs = DEFAULT_AUTO_HIDE_MS,
  ariaLabel = 'Secret value',
}) => {
  const [revealed, setRevealed] = useState(false);
  const [copied, setCopied] = useState(false);
  const hideTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const clearHideTimer = useCallback(() => {
    if (hideTimerRef.current) {
      clearTimeout(hideTimerRef.current);
      hideTimerRef.current = null;
    }
  }, []);

  const hide = useCallback(() => {
    clearHideTimer();
    setRevealed(false);
  }, [clearHideTimer]);

  // Auto-hide after timeout when revealed
  useEffect(() => {
    if (!revealed || autoHideMs <= 0) return;
    hideTimerRef.current = setTimeout(() => {
      setRevealed(false);
      hideTimerRef.current = null;
    }, autoHideMs);
    return clearHideTimer;
  }, [revealed, autoHideMs, clearHideTimer]);

  // Hide immediately when the tab is hidden / window loses focus
  useEffect(() => {
    if (!revealed) return;
    const onVisibility = () => {
      if (document.visibilityState === 'hidden') {
        hide();
      }
    };
    document.addEventListener('visibilitychange', onVisibility);
    return () => document.removeEventListener('visibilitychange', onVisibility);
  }, [revealed, hide]);

  const handleToggleReveal = () => {
    setRevealed((prev) => !prev);
  };

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(value);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  };

  const display = revealed ? value : MASK;

  return (
    <Box
      sx={{
        display: 'inline-flex',
        alignItems: 'center',
        gap: 0.5,
        px: 1,
        py: 0.25,
        borderRadius: 1,
        border: `1px solid ${PALETTE.border}`,
        backgroundColor: 'transparent',
        color: PALETTE.text,
        '&:hover': { borderColor: PALETTE.borderHover },
        '&:focus-within': { borderColor: PALETTE.borderFocus },
      }}
    >
      <Box
        component="span"
        aria-label={ariaLabel}
        sx={{
          fontFamily: monospace ? 'monospace' : undefined,
          fontSize: '0.8rem',
          color: PALETTE.text,
          userSelect: revealed ? 'text' : 'none',
          maxWidth: 320,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          px: 0.5,
          flexShrink: 1,
          minWidth: 0,
        }}
        title={revealed ? value : undefined}
      >
        {display}
      </Box>

      {revealable && (
        <Tooltip title={revealed ? 'Hide' : 'Reveal'}>
          <IconButton
            size="small"
            onClick={handleToggleReveal}
            sx={{ color: PALETTE.iconMuted, '&:hover': { color: PALETTE.text } }}
            aria-label={revealed ? 'Hide secret' : 'Reveal secret'}
          >
            {revealed ? <EyeOff size={16} /> : <Eye size={16} />}
          </IconButton>
        </Tooltip>
      )}

      {copyable && (
        <Tooltip title={copied ? 'Copied!' : 'Copy'}>
          <IconButton
            size="small"
            onClick={handleCopy}
            sx={{ color: PALETTE.iconMuted, '&:hover': { color: PALETTE.text } }}
            aria-label="Copy secret"
          >
            {copied ? <Check size={16} /> : <Copy size={16} />}
          </IconButton>
        </Tooltip>
      )}
    </Box>
  );
};

export default MaskedSecret;
