/**
 * CursorRenderer - Shared cursor rendering for local and remote cursors
 *
 * Renders cursor shapes as SVG when bitmap data is unavailable.
 * Supports all common CSS cursor types with appropriate SVG icons.
 */

import React from 'react';
import { Box } from '@mui/material';
import { CursorImageData } from '../../lib/helix-stream/stream/websocket-stream';

const STANDARD_CURSOR_SIZE = 24;

interface CursorRendererProps {
  /** Cursor position relative to container */
  x: number;
  y: number;
  /** Bitmap cursor data (preferred if available) */
  cursorImage?: CursorImageData | null;
  /** CSS cursor name for SVG fallback */
  cursorCssName?: string | null;
  /** User's assigned color for glow effect */
  userColor?: string;
  /** Whether to show the debug hotspot dot */
  showDebugDot?: boolean;
  /** Additional zIndex offset */
  zIndexOffset?: number;
}

/**
 * Get SVG path and dimensions for a cursor type
 * Futuristic dark-mode style: black/dark body with white outline
 */
function getCursorSvg(cursorName: string): { path: JSX.Element; width: number; height: number; centered: boolean } {
  switch (cursorName) {
    case 'default':
    case 'arrow':
      // Sleek futuristic arrow - sharp triangular pointer
      return {
        path: (
          <>
            <path d="M1 1L1 18L5 14L8 20L11 18L8 12L14 12L1 1Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
          </>
        ),
        width: 15,
        height: 21,
        centered: false,
      };

    case 'pointer':
    case 'hand':
      // Sleek pointer arrow - same style as default, slightly offset
      return {
        path: (
          <>
            <path d="M1 1L1 18L5 14L8 20L11 18L8 12L14 12L1 1Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
          </>
        ),
        width: 15,
        height: 21,
        centered: false,
      };

    case 'text':
    case 'ibeam':
      // Clean I-beam
      return {
        path: (
          <>
            <path d="M5 3H11M5 21H11M8 3V21" stroke="#1a1a2e" strokeWidth="3" strokeLinecap="round"/>
            <path d="M5 3H11M5 21H11M8 3V21" stroke="white" strokeWidth="1.5" strokeLinecap="round"/>
          </>
        ),
        width: 16,
        height: 24,
        centered: true,
      };

    case 'crosshair':
    case 'cross':
      // Tech crosshair with center dot
      return {
        path: (
          <>
            <path d="M12 2V8M12 16V22M2 12H8M16 12H22" stroke="#1a1a2e" strokeWidth="3" strokeLinecap="round"/>
            <path d="M12 2V8M12 16V22M2 12H8M16 12H22" stroke="white" strokeWidth="1.5" strokeLinecap="round"/>
            <circle cx="12" cy="12" r="2" fill="white" stroke="#1a1a2e" strokeWidth="1"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'move':
    case 'all-scroll':
      // Four-way arrows
      return {
        path: (
          <>
            <path d="M12 3L8 7H16L12 3ZM12 21L8 17H16L12 21ZM3 12L7 8V16L3 12ZM21 12L17 8V16L21 12Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'grab':
    case 'openhand':
      // Simple open hand
      return {
        path: (
          <>
            <path d="M10 6V4a1.5 1.5 0 013 0v6M13 5V3a1.5 1.5 0 013 0v7M16 7V5a1.5 1.5 0 013 0v8a6 6 0 01-12 0v-3a1.5 1.5 0 013 0v1M7 13v-2a1.5 1.5 0 013 0" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'grabbing':
    case 'closedhand':
      // Closed fist
      return {
        path: (
          <>
            <path d="M10 10V8a1.5 1.5 0 013 0v4M13 9V7a1.5 1.5 0 013 0v5M16 10V8a1.5 1.5 0 013 0v5a6 6 0 01-12 0v-2a1.5 1.5 0 013 0M7 14v-2a1.5 1.5 0 013 0" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'not-allowed':
    case 'no-drop':
      // Prohibition sign
      return {
        path: (
          <>
            <circle cx="12" cy="12" r="9" fill="#1a1a2e" stroke="white" strokeWidth="1.5"/>
            <path d="M6 18L18 6" stroke="white" strokeWidth="2" strokeLinecap="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'wait':
    case 'busy':
      // Hourglass
      return {
        path: (
          <>
            <path d="M7 4H17V7L12 12L17 17V20H7V17L12 12L7 7V4Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
            <path d="M9 6H15L12 9L9 6Z" fill="white" opacity="0.6"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'progress':
      // Arrow with dot
      return {
        path: (
          <>
            <path d="M1 1L1 18L5 14L8 20L11 18L8 12L14 12L1 1Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
            <circle cx="18" cy="18" r="4" fill="#1a1a2e" stroke="white" strokeWidth="1.5"/>
          </>
        ),
        width: 23,
        height: 23,
        centered: false,
      };

    case 'ns-resize':
    case 'row-resize':
      // Vertical resize
      return {
        path: (
          <>
            <path d="M12 3L7 8H17L12 3ZM12 21L7 16H17L12 21Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
            <path d="M12 8V16" stroke="white" strokeWidth="2" strokeLinecap="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'ew-resize':
    case 'col-resize':
      // Horizontal resize
      return {
        path: (
          <>
            <path d="M3 12L8 7V17L3 12ZM21 12L16 7V17L21 12Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
            <path d="M8 12H16" stroke="white" strokeWidth="2" strokeLinecap="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'nwse-resize':
      // Diagonal NW-SE
      return {
        path: (
          <>
            <path d="M5 5L5 11L11 5L5 5ZM19 19L19 13L13 19L19 19Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
            <path d="M7 7L17 17" stroke="white" strokeWidth="2" strokeLinecap="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'nesw-resize':
      // Diagonal NE-SW
      return {
        path: (
          <>
            <path d="M19 5L19 11L13 5L19 5ZM5 19L5 13L11 19L5 19Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
            <path d="M17 7L7 17" stroke="white" strokeWidth="2" strokeLinecap="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    case 'zoom-in':
      // Magnifier with plus
      return {
        path: (
          <>
            <circle cx="10" cy="10" r="7" fill="#1a1a2e" stroke="white" strokeWidth="1.5"/>
            <path d="M15 15L21 21" stroke="white" strokeWidth="2.5" strokeLinecap="round"/>
            <path d="M7 10H13M10 7V13" stroke="white" strokeWidth="1.5" strokeLinecap="round"/>
          </>
        ),
        width: 23,
        height: 23,
        centered: true,
      };

    case 'zoom-out':
      // Magnifier with minus
      return {
        path: (
          <>
            <circle cx="10" cy="10" r="7" fill="#1a1a2e" stroke="white" strokeWidth="1.5"/>
            <path d="M15 15L21 21" stroke="white" strokeWidth="2.5" strokeLinecap="round"/>
            <path d="M7 10H13" stroke="white" strokeWidth="1.5" strokeLinecap="round"/>
          </>
        ),
        width: 23,
        height: 23,
        centered: true,
      };

    case 'help':
      // Arrow with question badge
      return {
        path: (
          <>
            <path d="M1 1L1 14L4 11L6 15L9 14L6 9L11 9L1 1Z" fill="#1a1a2e" stroke="white" strokeWidth="1.2" strokeLinejoin="round"/>
            <circle cx="17" cy="17" r="6" fill="#1a1a2e" stroke="white" strokeWidth="1.2"/>
            <text x="17" y="20" textAnchor="middle" fontSize="9" fontWeight="bold" fill="white">?</text>
          </>
        ),
        width: 24,
        height: 24,
        centered: false,
      };

    case 'context-menu':
      // Arrow with menu
      return {
        path: (
          <>
            <path d="M1 1L1 14L4 11L6 15L9 14L6 9L11 9L1 1Z" fill="#1a1a2e" stroke="white" strokeWidth="1.2" strokeLinejoin="round"/>
            <rect x="13" y="13" width="9" height="9" rx="1" fill="#1a1a2e" stroke="white" strokeWidth="1.2"/>
            <path d="M15 16H20M15 19H18" stroke="white" strokeWidth="1" strokeLinecap="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: false,
      };

    case 'copy':
      // Arrow with plus badge
      return {
        path: (
          <>
            <path d="M1 1L1 14L4 11L6 15L9 14L6 9L11 9L1 1Z" fill="#1a1a2e" stroke="white" strokeWidth="1.2" strokeLinejoin="round"/>
            <circle cx="17" cy="17" r="5" fill="#1a1a2e" stroke="white" strokeWidth="1.2"/>
            <path d="M17 14V20M14 17H20" stroke="white" strokeWidth="1.5" strokeLinecap="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: false,
      };

    case 'alias':
      // Arrow with link
      return {
        path: (
          <>
            <path d="M1 1L1 14L4 11L6 15L9 14L6 9L11 9L1 1Z" fill="#1a1a2e" stroke="white" strokeWidth="1.2" strokeLinejoin="round"/>
            <path d="M14 14L22 22M14 20V14H20" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: false,
      };

    case 'cell':
      // Plus/cell selector
      return {
        path: (
          <>
            <path d="M12 4V20M4 12H20" stroke="#1a1a2e" strokeWidth="3" strokeLinecap="round"/>
            <path d="M12 4V20M4 12H20" stroke="white" strokeWidth="1.5" strokeLinecap="round"/>
          </>
        ),
        width: 24,
        height: 24,
        centered: true,
      };

    default:
      // Sleek default arrow
      return {
        path: (
          <>
            <path d="M1 1L1 18L5 14L8 20L11 18L8 12L14 12L1 1Z" fill="#1a1a2e" stroke="white" strokeWidth="1.5" strokeLinejoin="round"/>
          </>
        ),
        width: 15,
        height: 21,
        centered: false,
      };
  }
}

const CursorRenderer: React.FC<CursorRendererProps> = ({
  x,
  y,
  cursorImage,
  cursorCssName,
  userColor,
  showDebugDot = false,
  zIndexOffset = 0,
}) => {
  const glowFilter = userColor
    ? `drop-shadow(0 0 3px ${userColor}) drop-shadow(0 0 6px ${userColor}80)`
    : 'drop-shadow(0 0 2px rgba(255,255,255,0.8))';

  // Render bitmap cursor if available
  if (cursorImage?.imageUrl) {
    const cursorScale = cursorImage.width > 0
      ? STANDARD_CURSOR_SIZE / cursorImage.width
      : 1;
    const scaledWidth = cursorImage.width * cursorScale;
    const scaledHeight = cursorImage.height * cursorScale;
    const scaledHotspotX = cursorImage.hotspotX * cursorScale;
    const scaledHotspotY = cursorImage.hotspotY * cursorScale;

    return (
      <>
        <Box
          sx={{
            position: 'absolute',
            left: x - scaledHotspotX,
            top: y - scaledHotspotY,
            width: scaledWidth,
            height: scaledHeight,
            backgroundImage: `url(${cursorImage.imageUrl})`,
            backgroundSize: 'contain',
            backgroundRepeat: 'no-repeat',
            pointerEvents: 'none',
            zIndex: 1000 + zIndexOffset,
            filter: glowFilter,
          }}
        />
        {showDebugDot && (
          <Box
            sx={{
              position: 'absolute',
              left: x - 3,
              top: y - 3,
              width: 6,
              height: 6,
              borderRadius: '50%',
              backgroundColor: 'red',
              border: '1px solid white',
              pointerEvents: 'none',
              zIndex: 1002 + zIndexOffset,
            }}
          />
        )}
      </>
    );
  }

  // Render SVG cursor based on cursor name
  const cursorName = cursorCssName || 'default';
  const { path, width, height, centered } = getCursorSvg(cursorName);

  return (
    <Box
      sx={{
        position: 'absolute',
        left: x,
        top: y,
        pointerEvents: 'none',
        zIndex: 1000 + zIndexOffset,
        transform: centered ? 'translate(-50%, -50%)' : 'translate(0, 0)',
        filter: glowFilter,
      }}
    >
      <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} fill="none" xmlns="http://www.w3.org/2000/svg">
        {path}
      </svg>
    </Box>
  );
};

export default CursorRenderer;
