import React, { useState } from 'react';
import { Box, Tabs, Tab, IconButton, Tooltip } from '@mui/material';
import { Terminal as TerminalIcon, Videocam, SplitscreenRounded } from '@mui/icons-material';
import DesktopStreamViewer from './DesktopStreamViewer';
import ClaudeTerminal from './ClaudeTerminal';
import { DesktopStreamViewerProps, ViewMode } from './DesktopStreamViewer.types';

export interface AgentSessionViewerProps extends DesktopStreamViewerProps {
  // When true, show tabs to switch between video and terminal
  // Default: false for zed/vscode, true for claude_code
  showViewToggle?: boolean;
  // Initial view mode
  initialViewMode?: ViewMode;
}

/**
 * AgentSessionViewer - Unified viewer for all agent host types
 *
 * For Zed and VS Code sessions: Shows video streaming (default)
 * For Claude Code sessions: Shows terminal (default) with optional video toggle
 *
 * This component wraps DesktopStreamViewer and ClaudeTerminal to provide
 * a consistent interface for all agent types.
 */
const AgentSessionViewer: React.FC<AgentSessionViewerProps> = ({
  sessionId,
  agentHostType = 'zed',
  showViewToggle,
  initialViewMode,
  ...videoProps
}) => {
  // Determine default view mode based on agent type
  const defaultViewMode: ViewMode = agentHostType === 'claude_code' ? 'terminal' : 'video';
  const [viewMode, setViewMode] = useState<ViewMode>(initialViewMode || defaultViewMode);

  // Determine if we should show the toggle
  // Claude Code sessions always show toggle (can view desktop for debugging)
  // Other sessions only show toggle if explicitly requested
  const shouldShowToggle = showViewToggle !== undefined ? showViewToggle : agentHostType === 'claude_code';

  // For non-Claude sessions without toggle, just show video
  if (!shouldShowToggle && agentHostType !== 'claude_code') {
    return (
      <DesktopStreamViewer
        sessionId={sessionId}
        agentHostType={agentHostType}
        {...videoProps}
      />
    );
  }

  // For terminal-only mode (no toggle)
  if (!shouldShowToggle && agentHostType === 'claude_code') {
    return (
      <ClaudeTerminal
        sessionId={sessionId}
        className={videoProps.className}
        onConnectionChange={videoProps.onConnectionChange}
        onError={videoProps.onError}
      />
    );
  }

  return (
    <Box sx={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* View mode tabs */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          borderBottom: 1,
          borderColor: 'divider',
          backgroundColor: 'background.paper',
        }}
      >
        <Tabs
          value={viewMode}
          onChange={(_, newValue) => setViewMode(newValue)}
          sx={{ minHeight: 36 }}
        >
          <Tab
            icon={<TerminalIcon fontSize="small" />}
            iconPosition="start"
            label="Terminal"
            value="terminal"
            sx={{ minHeight: 36, py: 0 }}
          />
          <Tab
            icon={<Videocam fontSize="small" />}
            iconPosition="start"
            label="Desktop"
            value="video"
            sx={{ minHeight: 36, py: 0 }}
          />
        </Tabs>
      </Box>

      {/* Content area */}
      <Box sx={{ flex: 1, overflow: 'hidden', position: 'relative' }}>
        {/* Terminal view */}
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: viewMode === 'terminal' ? 'flex' : 'none',
          }}
        >
          <ClaudeTerminal
            sessionId={sessionId}
            className={videoProps.className}
            onConnectionChange={viewMode === 'terminal' ? videoProps.onConnectionChange : undefined}
            onError={videoProps.onError}
          />
        </Box>

        {/* Video view */}
        <Box
          sx={{
            position: 'absolute',
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: viewMode === 'video' ? 'flex' : 'none',
          }}
        >
          <DesktopStreamViewer
            sessionId={sessionId}
            agentHostType={agentHostType}
            onConnectionChange={viewMode === 'video' ? videoProps.onConnectionChange : undefined}
            {...videoProps}
          />
        </Box>
      </Box>
    </Box>
  );
};

export default AgentSessionViewer;
