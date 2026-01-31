/**
 * RemoteCursorsOverlay - Renders remote user cursors (Figma-style multi-player)
 *
 * Uses shared CursorRenderer for consistent cursor appearance with local cursor.
 */

import React from 'react';
import { Box, Typography } from '@mui/material';
import { RemoteCursorPosition, RemoteUserInfo } from '../../lib/helix-stream/stream/websocket-stream';
import CursorRenderer from './CursorRenderer';

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
            }}
          >
            {/* Cursor - uses shared CursorRenderer for consistent appearance */}
            <Box sx={{ position: 'relative' }}>
              <CursorRenderer
                x={0}
                y={0}
                cursorImage={cursor.cursorImage}
                cursorCssName={cursor.cursorImage?.cursorName || 'default'}
                userColor={displayColor}
                zIndexOffset={1}
              />
            </Box>
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
