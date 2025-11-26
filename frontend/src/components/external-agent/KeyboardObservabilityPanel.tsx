import React, { useState, useEffect, useRef } from 'react';
import {
  Box,
  Typography,
  IconButton,
  Select,
  MenuItem,
  FormControl,
  InputLabel,
  Button,
  Chip,
  Tooltip,
} from '@mui/material';
import { Close, Refresh, Warning } from '@mui/icons-material';
import {
  useWolfKeyboardState,
  useResetWolfKeyboardState,
  SessionKeyboardState,
} from '../../services/wolfService';

// Windows Virtual Key Code mappings
// https://docs.microsoft.com/en-us/windows/win32/inputdev/virtual-key-codes
const VK_TO_KEY: Record<number, { label: string; width?: number }> = {
  // Row 1 - Function keys
  0x1B: { label: 'Esc', width: 1 },
  0x70: { label: 'F1', width: 1 },
  0x71: { label: 'F2', width: 1 },
  0x72: { label: 'F3', width: 1 },
  0x73: { label: 'F4', width: 1 },
  0x74: { label: 'F5', width: 1 },
  0x75: { label: 'F6', width: 1 },
  0x76: { label: 'F7', width: 1 },
  0x77: { label: 'F8', width: 1 },
  0x78: { label: 'F9', width: 1 },
  0x79: { label: 'F10', width: 1 },
  0x7A: { label: 'F11', width: 1 },
  0x7B: { label: 'F12', width: 1 },

  // Row 2 - Number row
  0xC0: { label: '`', width: 1 },
  0x31: { label: '1', width: 1 },
  0x32: { label: '2', width: 1 },
  0x33: { label: '3', width: 1 },
  0x34: { label: '4', width: 1 },
  0x35: { label: '5', width: 1 },
  0x36: { label: '6', width: 1 },
  0x37: { label: '7', width: 1 },
  0x38: { label: '8', width: 1 },
  0x39: { label: '9', width: 1 },
  0x30: { label: '0', width: 1 },
  0xBD: { label: '-', width: 1 },
  0xBB: { label: '=', width: 1 },
  0x08: { label: 'Backspace', width: 2 },

  // Row 3 - QWERTY
  0x09: { label: 'Tab', width: 1.5 },
  0x51: { label: 'Q', width: 1 },
  0x57: { label: 'W', width: 1 },
  0x45: { label: 'E', width: 1 },
  0x52: { label: 'R', width: 1 },
  0x54: { label: 'T', width: 1 },
  0x59: { label: 'Y', width: 1 },
  0x55: { label: 'U', width: 1 },
  0x49: { label: 'I', width: 1 },
  0x4F: { label: 'O', width: 1 },
  0x50: { label: 'P', width: 1 },
  0xDB: { label: '[', width: 1 },
  0xDD: { label: ']', width: 1 },
  0xDC: { label: '\\', width: 1.5 },

  // Row 4 - ASDF
  0x14: { label: 'Caps', width: 1.75 },
  0x41: { label: 'A', width: 1 },
  0x53: { label: 'S', width: 1 },
  0x44: { label: 'D', width: 1 },
  0x46: { label: 'F', width: 1 },
  0x47: { label: 'G', width: 1 },
  0x48: { label: 'H', width: 1 },
  0x4A: { label: 'J', width: 1 },
  0x4B: { label: 'K', width: 1 },
  0x4C: { label: 'L', width: 1 },
  0xBA: { label: ';', width: 1 },
  0xDE: { label: "'", width: 1 },
  0x0D: { label: 'Enter', width: 2.25 },

  // Row 5 - ZXCV
  0x10: { label: 'Shift', width: 2.25 },
  0xA0: { label: 'LShift', width: 2.25 },
  0xA1: { label: 'RShift', width: 2.75 },
  0x5A: { label: 'Z', width: 1 },
  0x58: { label: 'X', width: 1 },
  0x43: { label: 'C', width: 1 },
  0x56: { label: 'V', width: 1 },
  0x42: { label: 'B', width: 1 },
  0x4E: { label: 'N', width: 1 },
  0x4D: { label: 'M', width: 1 },
  0xBC: { label: ',', width: 1 },
  0xBE: { label: '.', width: 1 },
  0xBF: { label: '/', width: 1 },

  // Row 6 - Bottom row
  0x11: { label: 'Ctrl', width: 1.25 },
  0xA2: { label: 'LCtrl', width: 1.25 },
  0xA3: { label: 'RCtrl', width: 1.25 },
  0x5B: { label: 'Win', width: 1.25 },
  0x5C: { label: 'RWin', width: 1.25 },
  0x12: { label: 'Alt', width: 1.25 },
  0xA4: { label: 'LAlt', width: 1.25 },
  0xA5: { label: 'RAlt', width: 1.25 },
  0x20: { label: 'Space', width: 6.25 },
  0x5D: { label: 'Menu', width: 1.25 },

  // Navigation cluster
  0x2C: { label: 'PrtSc', width: 1 },
  0x91: { label: 'ScrLk', width: 1 },
  0x13: { label: 'Pause', width: 1 },
  0x2D: { label: 'Ins', width: 1 },
  0x24: { label: 'Home', width: 1 },
  0x21: { label: 'PgUp', width: 1 },
  0x2E: { label: 'Del', width: 1 },
  0x23: { label: 'End', width: 1 },
  0x22: { label: 'PgDn', width: 1 },

  // Arrow keys
  0x26: { label: '\u2191', width: 1 }, // Up arrow
  0x28: { label: '\u2193', width: 1 }, // Down arrow
  0x25: { label: '\u2190', width: 1 }, // Left arrow
  0x27: { label: '\u2192', width: 1 }, // Right arrow

  // Numpad
  0x90: { label: 'NumLk', width: 1 },
  0x6F: { label: 'N/', width: 1 },
  0x6A: { label: 'N*', width: 1 },
  0x6D: { label: 'N-', width: 1 },
  0x6B: { label: 'N+', width: 1 },
  0x60: { label: 'N0', width: 2 },
  0x61: { label: 'N1', width: 1 },
  0x62: { label: 'N2', width: 1 },
  0x63: { label: 'N3', width: 1 },
  0x64: { label: 'N4', width: 1 },
  0x65: { label: 'N5', width: 1 },
  0x66: { label: 'N6', width: 1 },
  0x67: { label: 'N7', width: 1 },
  0x68: { label: 'N8', width: 1 },
  0x69: { label: 'N9', width: 1 },
  0x6E: { label: 'N.', width: 1 },
};

// Keyboard layouts - define which keys appear in which rows
const US_LAYOUT = {
  functionRow: [0x1B, 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77, 0x78, 0x79, 0x7A, 0x7B],
  numberRow: [0xC0, 0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38, 0x39, 0x30, 0xBD, 0xBB, 0x08],
  qwertyRow: [0x09, 0x51, 0x57, 0x45, 0x52, 0x54, 0x59, 0x55, 0x49, 0x4F, 0x50, 0xDB, 0xDD, 0xDC],
  asdfRow: [0x14, 0x41, 0x53, 0x44, 0x46, 0x47, 0x48, 0x4A, 0x4B, 0x4C, 0xBA, 0xDE, 0x0D],
  zxcvRow: [0xA0, 0x5A, 0x58, 0x43, 0x56, 0x42, 0x4E, 0x4D, 0xBC, 0xBE, 0xBF, 0xA1],
  bottomRow: [0xA2, 0x5B, 0xA4, 0x20, 0xA5, 0x5C, 0x5D, 0xA3],
  navCluster: [0x2D, 0x24, 0x21, 0x2E, 0x23, 0x22],
  arrowKeys: [0x26, 0x25, 0x28, 0x27],
};

interface KeyEvent {
  timestamp: number;
  keyCode: number;
  keyName: string;
  action: 'press' | 'release';
  sessionId: string;
}

interface KeyboardObservabilityPanelProps {
  sandboxInstanceId: string;
  onClose: () => void;
}

const KeyboardObservabilityPanel: React.FC<KeyboardObservabilityPanelProps> = ({
  sandboxInstanceId,
  onClose,
}) => {
  const [keyboardLayout, setKeyboardLayout] = useState<'us' | 'gb' | 'fr'>('us');
  const [eventLog, setEventLog] = useState<KeyEvent[]>([]);
  const previousStateRef = useRef<Map<string, Set<number>>>(new Map());
  const eventLogRef = useRef<HTMLDivElement>(null);

  const { data: keyboardState, isLoading, error } = useWolfKeyboardState({
    sandboxInstanceId,
    enabled: !!sandboxInstanceId,
    refetchInterval: 200, // Poll every 200ms for responsive UI
  });

  const resetMutation = useResetWolfKeyboardState(sandboxInstanceId);

  // Track key state changes and build event log
  useEffect(() => {
    if (!keyboardState?.sessions) return;

    const newEvents: KeyEvent[] = [];
    const now = Date.now();

    keyboardState.sessions.forEach((session: SessionKeyboardState) => {
      const currentKeys = new Set(session.pressed_keys);
      const previousKeys = previousStateRef.current.get(session.session_id) || new Set();

      // Detect newly pressed keys
      currentKeys.forEach(keyCode => {
        if (!previousKeys.has(keyCode)) {
          newEvents.push({
            timestamp: now,
            keyCode,
            keyName: VK_TO_KEY[keyCode]?.label || `0x${keyCode.toString(16).toUpperCase()}`,
            action: 'press',
            sessionId: session.session_id,
          });
        }
      });

      // Detect released keys
      previousKeys.forEach(keyCode => {
        if (!currentKeys.has(keyCode)) {
          newEvents.push({
            timestamp: now,
            keyCode,
            keyName: VK_TO_KEY[keyCode]?.label || `0x${keyCode.toString(16).toUpperCase()}`,
            action: 'release',
            sessionId: session.session_id,
          });
        }
      });

      previousStateRef.current.set(session.session_id, currentKeys);
    });

    if (newEvents.length > 0) {
      setEventLog(prev => [...newEvents, ...prev].slice(0, 100)); // Keep last 100 events
    }
  }, [keyboardState]);

  // Auto-scroll event log
  useEffect(() => {
    if (eventLogRef.current) {
      eventLogRef.current.scrollTop = 0;
    }
  }, [eventLog]);

  // Get all pressed keys across all sessions
  const allPressedKeys = new Set<number>();
  keyboardState?.sessions?.forEach((session: SessionKeyboardState) => {
    session.pressed_keys?.forEach(key => allPressedKeys.add(key));
  });

  // Check for stuck modifier keys (pressed for more than 5 seconds)
  const hasStuckKeys = keyboardState?.sessions?.some((session: SessionKeyboardState) => {
    const timeSinceUpdate = Date.now() - session.timestamp_ms;
    return session.pressed_keys?.length > 0 && timeSinceUpdate > 5000;
  });

  const handleReset = async (sessionId: string) => {
    try {
      await resetMutation.mutateAsync(sessionId);
    } catch (err) {
      console.error('Failed to reset keyboard state:', err);
    }
  };

  const renderKey = (vkCode: number) => {
    const keyInfo = VK_TO_KEY[vkCode] || { label: '?', width: 1 };
    const isPressed = allPressedKeys.has(vkCode);
    const isModifier = [0x10, 0xA0, 0xA1, 0x11, 0xA2, 0xA3, 0x12, 0xA4, 0xA5, 0x14, 0x5B, 0x5C].includes(vkCode);

    return (
      <Box
        key={vkCode}
        sx={{
          width: (keyInfo.width || 1) * 36,
          height: 36,
          margin: '2px',
          borderRadius: '4px',
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          fontSize: keyInfo.label.length > 2 ? '10px' : '12px',
          fontWeight: isPressed ? 'bold' : 'normal',
          backgroundColor: isPressed
            ? (isModifier ? '#ff6b6b' : '#4caf50')
            : 'rgba(255, 255, 255, 0.1)',
          color: isPressed ? '#fff' : 'rgba(255, 255, 255, 0.7)',
          border: isPressed
            ? '2px solid rgba(255, 255, 255, 0.5)'
            : '1px solid rgba(255, 255, 255, 0.2)',
          transition: 'all 0.1s ease-out',
          transform: isPressed ? 'scale(0.95)' : 'scale(1)',
          boxShadow: isPressed ? '0 0 10px rgba(76, 175, 80, 0.5)' : 'none',
        }}
      >
        {keyInfo.label}
      </Box>
    );
  };

  const renderKeyRow = (keyCodes: number[]) => (
    <Box sx={{ display: 'flex', justifyContent: 'center', flexWrap: 'nowrap' }}>
      {keyCodes.map(renderKey)}
    </Box>
  );

  return (
    <Box
      sx={{
        position: 'fixed',
        top: 100,
        right: 20,
        width: 650,
        maxHeight: '80vh',
        backgroundColor: 'rgba(30, 30, 40, 0.98)',
        borderRadius: 2,
        boxShadow: '0 8px 32px rgba(0, 0, 0, 0.5)',
        zIndex: 2000,
        overflow: 'hidden',
        border: '1px solid rgba(255, 255, 255, 0.1)',
      }}
    >
      {/* Header */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: 1.5,
          borderBottom: '1px solid rgba(255, 255, 255, 0.1)',
          backgroundColor: 'rgba(0, 0, 0, 0.3)',
        }}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography variant="h6" sx={{ color: 'white', fontSize: '14px' }}>
            Keyboard State Monitor
          </Typography>
          {hasStuckKeys && (
            <Tooltip title="Stuck keys detected!">
              <Warning sx={{ color: '#ff6b6b', fontSize: 18 }} />
            </Tooltip>
          )}
        </Box>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <FormControl size="small" sx={{ minWidth: 80 }}>
            <Select
              value={keyboardLayout}
              onChange={(e) => setKeyboardLayout(e.target.value as 'us' | 'gb' | 'fr')}
              sx={{
                color: 'white',
                fontSize: '12px',
                '& .MuiOutlinedInput-notchedOutline': {
                  borderColor: 'rgba(255, 255, 255, 0.2)',
                },
              }}
            >
              <MenuItem value="us">US</MenuItem>
              <MenuItem value="gb">GB</MenuItem>
              <MenuItem value="fr">FR</MenuItem>
            </Select>
          </FormControl>
          <IconButton onClick={onClose} size="small" sx={{ color: 'white' }}>
            <Close fontSize="small" />
          </IconButton>
        </Box>
      </Box>

      {/* Keyboard Visualization */}
      <Box sx={{ padding: 2 }}>
        {isLoading && (
          <Typography sx={{ color: 'rgba(255, 255, 255, 0.5)', textAlign: 'center' }}>
            Loading keyboard state...
          </Typography>
        )}

        {error && (
          <Typography sx={{ color: '#ff6b6b', textAlign: 'center', fontSize: '12px' }}>
            Error: {String(error)}
          </Typography>
        )}

        {!isLoading && !error && (
          <>
            {/* Modifier Status Bar */}
            <Box sx={{ display: 'flex', gap: 1, mb: 2, justifyContent: 'center' }}>
              {keyboardState?.sessions?.map((session: SessionKeyboardState) => (
                <Box key={session.session_id} sx={{ display: 'flex', gap: 0.5 }}>
                  <Chip
                    label="Shift"
                    size="small"
                    color={session.modifier_state?.shift ? 'error' : 'default'}
                    sx={{ fontSize: '10px', height: 20 }}
                  />
                  <Chip
                    label="Ctrl"
                    size="small"
                    color={session.modifier_state?.ctrl ? 'error' : 'default'}
                    sx={{ fontSize: '10px', height: 20 }}
                  />
                  <Chip
                    label="Alt"
                    size="small"
                    color={session.modifier_state?.alt ? 'error' : 'default'}
                    sx={{ fontSize: '10px', height: 20 }}
                  />
                  <Chip
                    label="Meta"
                    size="small"
                    color={session.modifier_state?.meta ? 'error' : 'default'}
                    sx={{ fontSize: '10px', height: 20 }}
                  />
                </Box>
              ))}
            </Box>

            {/* Keyboard Layout */}
            <Box sx={{ mb: 2 }}>
              {renderKeyRow(US_LAYOUT.functionRow)}
              {renderKeyRow(US_LAYOUT.numberRow)}
              {renderKeyRow(US_LAYOUT.qwertyRow)}
              {renderKeyRow(US_LAYOUT.asdfRow)}
              {renderKeyRow(US_LAYOUT.zxcvRow)}
              {renderKeyRow(US_LAYOUT.bottomRow)}
            </Box>

            {/* Arrow Keys */}
            <Box sx={{ display: 'flex', justifyContent: 'center', mb: 2 }}>
              <Box sx={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
                {renderKey(0x26)} {/* Up */}
                <Box sx={{ display: 'flex' }}>
                  {renderKey(0x25)} {/* Left */}
                  {renderKey(0x28)} {/* Down */}
                  {renderKey(0x27)} {/* Right */}
                </Box>
              </Box>
            </Box>
          </>
        )}
      </Box>

      {/* Session Info & Reset */}
      {keyboardState?.sessions && keyboardState.sessions.length > 0 && (
        <Box
          sx={{
            padding: 1.5,
            borderTop: '1px solid rgba(255, 255, 255, 0.1)',
            backgroundColor: 'rgba(0, 0, 0, 0.2)',
          }}
        >
          <Typography sx={{ color: 'rgba(255, 255, 255, 0.5)', fontSize: '11px', mb: 1 }}>
            Active Sessions: {keyboardState.sessions.length}
          </Typography>
          {keyboardState.sessions.map((session: SessionKeyboardState) => (
            <Box
              key={session.session_id}
              sx={{
                display: 'flex',
                alignItems: 'center',
                justifyContent: 'space-between',
                mb: 0.5,
              }}
            >
              <Typography sx={{ color: 'white', fontSize: '11px', fontFamily: 'monospace' }}>
                Session {session.session_id.slice(-8)}: {session.pressed_keys?.length || 0} keys pressed
              </Typography>
              <Button
                size="small"
                variant="outlined"
                color="warning"
                startIcon={<Refresh fontSize="small" />}
                onClick={() => handleReset(session.session_id)}
                disabled={resetMutation.isPending}
                sx={{ fontSize: '10px', py: 0.25 }}
              >
                Reset
              </Button>
            </Box>
          ))}
        </Box>
      )}

      {/* Event Log */}
      <Box
        sx={{
          borderTop: '1px solid rgba(255, 255, 255, 0.1)',
          backgroundColor: 'rgba(0, 0, 0, 0.4)',
          maxHeight: 150,
          overflowY: 'auto',
        }}
        ref={eventLogRef}
      >
        <Typography
          sx={{
            color: 'rgba(255, 255, 255, 0.5)',
            fontSize: '10px',
            padding: '4px 8px',
            borderBottom: '1px solid rgba(255, 255, 255, 0.05)',
            position: 'sticky',
            top: 0,
            backgroundColor: 'rgba(0, 0, 0, 0.6)',
          }}
        >
          Event Log (newest first)
        </Typography>
        {eventLog.length === 0 ? (
          <Typography
            sx={{
              color: 'rgba(255, 255, 255, 0.3)',
              fontSize: '11px',
              padding: '8px',
              textAlign: 'center',
            }}
          >
            No key events recorded yet. Press keys to see events.
          </Typography>
        ) : (
          eventLog.map((event, index) => (
            <Box
              key={`${event.timestamp}-${event.keyCode}-${index}`}
              sx={{
                display: 'flex',
                alignItems: 'center',
                gap: 1,
                padding: '2px 8px',
                fontSize: '11px',
                fontFamily: 'monospace',
                color: event.action === 'press' ? '#4caf50' : '#ff9800',
                '&:hover': {
                  backgroundColor: 'rgba(255, 255, 255, 0.05)',
                },
              }}
            >
              <span style={{ color: 'rgba(255, 255, 255, 0.3)', width: 70 }}>
                {new Date(event.timestamp).toLocaleTimeString('en-US', {
                  hour12: false,
                  hour: '2-digit',
                  minute: '2-digit',
                  second: '2-digit',
                })}
                .{String(event.timestamp % 1000).padStart(3, '0')}
              </span>
              <span style={{ width: 50 }}>
                {event.action === 'press' ? '\u2193 DOWN' : '\u2191 UP'}
              </span>
              <span style={{ color: 'white' }}>{event.keyName}</span>
              <span style={{ color: 'rgba(255, 255, 255, 0.3)' }}>
                (0x{event.keyCode.toString(16).toUpperCase()})
              </span>
            </Box>
          ))
        )}
      </Box>
    </Box>
  );
};

export default KeyboardObservabilityPanel;
