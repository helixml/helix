/**
 * AgentCursorOverlay - Renders the AI agent cursor
 */

import React from 'react';
import { Box, Typography } from '@mui/material';
import { AgentCursorInfo } from '../../lib/helix-stream/stream/websocket-stream';

export interface AgentCursorOverlayProps {
  agentCursor: AgentCursorInfo | null;
  canvasDisplaySize: { width: number; height: number } | null;
  containerSize: { width: number; height: number } | null;
  streamWidth: number;
  streamHeight: number;
}

const IDLE_TIMEOUT_MS = 30000; // 30 seconds

const AgentCursorOverlay: React.FC<AgentCursorOverlayProps> = ({
  agentCursor,
  canvasDisplaySize,
  containerSize,
  streamWidth,
  streamHeight,
}) => {
  if (!agentCursor || !canvasDisplaySize || !containerSize) return null;

  // Hide if idle for too long
  if (Date.now() - agentCursor.lastSeen >= IDLE_TIMEOUT_MS) return null;

  // Hide cursor if outside the canvas region (in stream coordinates)
  if (agentCursor.x < 0 || agentCursor.x > streamWidth || agentCursor.y < 0 || agentCursor.y > streamHeight) {
    return null;
  }

  // Scale agent cursor from screen coordinates to container-relative coordinates
  const scaleX = canvasDisplaySize.width / streamWidth;
  const scaleY = canvasDisplaySize.height / streamHeight;
  const offsetX = (containerSize.width - canvasDisplaySize.width) / 2;
  const offsetY = (containerSize.height - canvasDisplaySize.height) / 2;
  const displayX = offsetX + agentCursor.x * scaleX;
  const displayY = offsetY + agentCursor.y * scaleY;

  return (
    <Box
      sx={{
        position: 'absolute',
        left: displayX,
        top: displayY,
        pointerEvents: 'none',
        zIndex: 1002,
        display: 'flex',
        flexDirection: 'column',
        alignItems: 'flex-start',
        transition: 'left 0.15s ease-out, top 0.15s ease-out',
        willChange: 'left, top',
      }}
    >
      {/* Cyan arrow cursor with pulse animation and glow */}
      <svg
        width="24"
        height="24"
        style={{
          color: '#00D4FF',
          filter: 'drop-shadow(0 0 6px #00D4FF) drop-shadow(0 0 12px #00D4FF80)',
          animation: agentCursor.action !== 'idle' ? 'pulse 0.5s infinite' : 'none',
        }}
      >
        <defs>
          <filter id="agent-glow" x="-50%" y="-50%" width="200%" height="200%">
            <feGaussianBlur stdDeviation="3" result="coloredBlur"/>
            <feMerge>
              <feMergeNode in="coloredBlur"/>
              <feMergeNode in="SourceGraphic"/>
            </feMerge>
          </filter>
        </defs>
        <path
          fill="currentColor"
          stroke="white"
          strokeWidth="1"
          d="M0,0 L0,16 L4,12 L8,20 L10,19 L6,11 L12,11 Z"
          filter="url(#agent-glow)"
        />
      </svg>
      {/* Agent name pill with action indicator */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          backgroundColor: '#00D4FF',
          borderRadius: '12px',
          padding: '2px 8px 2px 4px',
          marginLeft: '8px',
          marginTop: '-4px',
        }}
      >
        <Box sx={{ fontSize: 14, marginRight: '4px' }}>🤖</Box>
        <Typography
          sx={{
            color: 'white',
            fontSize: 12,
            fontWeight: 500,
            textShadow: '0 1px 2px rgba(0,0,0,0.3)',
          }}
        >
          AI Agent
          {agentCursor.action !== 'idle' && (
            <Box component="span" sx={{ marginLeft: '4px', fontStyle: 'italic' }}>
              {agentCursor.action}...
            </Box>
          )}
        </Typography>
      </Box>
    </Box>
  );
};

export default AgentCursorOverlay;
