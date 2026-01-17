/**
 * RemoteCursorsOverlay - Renders remote user cursors (Figma-style multi-player)
 */

import React from 'react';
import { Box, Typography } from '@mui/material';
import { RemoteCursorPosition, RemoteUserInfo } from '../../lib/helix-stream/stream/websocket-stream';

export interface RemoteCursorsOverlayProps {
  cursors: Map<number, RemoteCursorPosition>;
  users: Map<number, RemoteUserInfo>;
  selfClientId: number | null;
  selfClientIdRef: React.RefObject<number | null>;
  canvasDisplaySize: { width: number; height: number } | null;
  containerSize: { width: number; height: number } | null;
  streamWidth: number;
  streamHeight: number;
}

const IDLE_TIMEOUT_MS = 30000; // 30 seconds

const RemoteCursorsOverlay: React.FC<RemoteCursorsOverlayProps> = ({
  cursors,
  users,
  selfClientId,
  selfClientIdRef,
  canvasDisplaySize,
  containerSize,
  streamWidth,
  streamHeight,
}) => {
  if (!canvasDisplaySize || !containerSize) return null;

  // Calculate scale factors
  const scaleX = canvasDisplaySize.width / streamWidth;
  const scaleY = canvasDisplaySize.height / streamHeight;

  // Calculate offset (canvas is centered in container)
  const offsetX = (containerSize.width - canvasDisplaySize.width) / 2;
  const offsetY = (containerSize.height - canvasDisplaySize.height) / 2;

  return (
    <>
      {Array.from(cursors.entries()).map(([userId, cursor]) => {
        // Skip our own cursor (we render it separately)
        if (userId === selfClientId || userId === selfClientIdRef.current) {
          return null;
        }
        // Skip idle cursors
        const isIdle = Date.now() - cursor.lastSeen > IDLE_TIMEOUT_MS;
        if (isIdle) {
          return null;
        }

        const user = users.get(userId);
        const displayColor = user?.color || cursor.color || '#0D99FF';
        const displayName = user?.userName || `User ${userId}`;

        // Transform cursor position
        const displayX = offsetX + cursor.x * scaleX;
        const displayY = offsetY + cursor.y * scaleY;

        return (
          <Box
            key={`remote-cursor-${userId}`}
            sx={{
              position: 'absolute',
              left: displayX,
              top: displayY,
              pointerEvents: 'none',
              zIndex: 1001,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'flex-start',
              transition: 'left 0.1s ease-out, top 0.1s ease-out',
              willChange: 'left, top',
            }}
          >
            {/* Cursor - use actual shape if available, otherwise default arrow */}
            {cursor.cursorImage ? (
              <Box
                component="img"
                src={cursor.cursorImage.imageUrl}
                sx={{
                  width: cursor.cursorImage.width,
                  height: cursor.cursorImage.height,
                  marginLeft: `-${cursor.cursorImage.hotspotX}px`,
                  marginTop: `-${cursor.cursorImage.hotspotY}px`,
                  filter: `drop-shadow(0 0 4px ${displayColor}) drop-shadow(0 0 8px ${displayColor}80)`,
                  pointerEvents: 'none',
                }}
              />
            ) : (
              <svg
                width="24"
                height="24"
                style={{
                  color: displayColor,
                  filter: `drop-shadow(0 0 4px ${displayColor}) drop-shadow(0 0 8px ${displayColor}80)`,
                }}
              >
                <defs>
                  <filter id={`glow-${userId}`} x="-50%" y="-50%" width="200%" height="200%">
                    <feGaussianBlur stdDeviation="2" result="coloredBlur"/>
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
                  filter={`url(#glow-${userId})`}
                />
              </svg>
            )}
            {/* User name pill */}
            <Box
              sx={{
                display: 'flex',
                alignItems: 'center',
                backgroundColor: displayColor,
                borderRadius: '12px',
                padding: '2px 8px 2px 4px',
                marginLeft: '8px',
                marginTop: '-4px',
              }}
            >
              {user?.avatarUrl ? (
                <Box
                  component="img"
                  src={user.avatarUrl}
                  sx={{ width: 20, height: 20, borderRadius: '50%' }}
                />
              ) : (
                <Box
                  sx={{
                    width: 20,
                    height: 20,
                    borderRadius: '50%',
                    backgroundColor: 'rgba(255,255,255,0.3)',
                    display: 'flex',
                    alignItems: 'center',
                    justifyContent: 'center',
                    fontSize: 10,
                    fontWeight: 'bold',
                    color: 'white',
                  }}
                >
                  {displayName.charAt(0).toUpperCase()}
                </Box>
              )}
              <Typography
                sx={{
                  marginLeft: '4px',
                  color: 'white',
                  fontSize: 12,
                  fontWeight: 500,
                  textShadow: '0 1px 2px rgba(0,0,0,0.3)',
                }}
              >
                {displayName}
              </Typography>
            </Box>
          </Box>
        );
      })}
    </>
  );
};

export default RemoteCursorsOverlay;
