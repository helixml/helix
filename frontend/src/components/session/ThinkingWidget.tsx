import React, { useEffect, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import CircularProgress from '@mui/material/CircularProgress';
import useLightTheme from '../../hooks/useLightTheme';

interface ThinkingWidgetProps {
  text: string;
  startTime?: number | Date;
  isStreaming: boolean;
}

function formatDuration(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, '0')}`;
}

const ThinkingWidget: React.FC<ThinkingWidgetProps> = ({ text, startTime, isStreaming }) => {
  const [elapsed, setElapsed] = useState(0);
  const [open, setOpen] = useState(false);
  const intervalRef = useRef<NodeJS.Timeout | null>(null);
  const start = useRef<number>(
    typeof startTime === 'number'
      ? startTime
      : startTime instanceof Date
      ? startTime.getTime()
      : Date.now()
  );
  const textContainerRef = useRef<HTMLDivElement | null>(null);
  const lightTheme = useLightTheme();

  useEffect(() => {
    // Do not auto-expand/collapse based on isStreaming
  }, [isStreaming]);

  useEffect(() => {
    intervalRef.current = setInterval(() => {
      setElapsed(Math.floor((Date.now() - start.current) / 1000));
    }, 1000);
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current);
    };
  }, []);

  // Auto-scroll to bottom in collapsed mode when text changes
  useEffect(() => {
    if (!open && textContainerRef.current) {
      textContainerRef.current.scrollTo({
        top: textContainerRef.current.scrollHeight,
        behavior: 'smooth',
      });
    }
  }, [text, open]);

  // Split text into lines for custom rendering
  const lines = text.split('\n');

  if (!open && !isStreaming) {
    return (
      <Box
        sx={{
          position: 'relative',
          borderRadius: 3,
          boxShadow: 3,
          background: theme =>
            theme.palette.mode === 'light'
              ? 'rgba(245,245,250,0.95)'
              : 'rgba(30,32,40,0.95)',
          p: { xs: 2, sm: 3 },
          my: 2,
          overflow: 'hidden',
          minHeight: 60,
          width: '100%',
          mx: 0,
          cursor: 'pointer',
        }}
        onClick={() => setOpen(true)}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', width: '100%' }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: 'primary.main' }}>
            💡 Thoughts
          </Typography>
          <Typography variant="body2" sx={{ color: 'text.secondary', ml: 2 }}>
            Expand for details
          </Typography>
        </Box>
      </Box>
    );
  }

  return (
    <Box
      sx={{
        position: 'relative',
        borderRadius: 3,
        boxShadow: 3,
        background: theme =>
          theme.palette.mode === 'light'
            ? 'rgba(245,245,250,0.95)'
            : 'rgba(30,32,40,0.95)',
        p: { xs: 2, sm: 3 },
        my: 2,
        overflow: 'hidden',
        minHeight: 120,
        width: '100%',
        mx: 0,
      }}
    >
      {/* Clickable top bar for collapse/expand */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          width: '100%',
          cursor: 'pointer',
          userSelect: 'none',
          minHeight: 40,          
        }}
        onClick={() => setOpen(!open)}
      >
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
          <Typography variant="subtitle2" sx={{ fontWeight: 600, color: 'primary.main' }}>
            💡 {isStreaming ? `Thinking ${formatDuration(elapsed)}` : `Thoughts`}
          </Typography>
          {isStreaming && (
            <CircularProgress
              size={16}
              thickness={4}
              sx={{ color: 'primary.main' }}
            />
          )}
        </Box>
        <Typography variant="body2" sx={{ color: 'text.secondary', ml: 2 }}>
          {open ? 'Collapse details' : 'Expand for details'}
        </Typography>
      </Box>
      {/* Main content, show differently based on open state */}
      {open ? (
        <Box
          sx={{
            position: 'relative',
            zIndex: 2,
            py: 2,
          }}
        >
          <Typography
            variant="body1"
            sx={{
              color: 'text.primary',
              px: { xs: 0, sm: 2 },
              maxHeight: 200,
              overflowY: 'auto',
              fontSize: 16,
              fontFamily: 'inherit',
              whiteSpace: 'pre-line',
              ...lightTheme.scrollbar,
            }}
          >
            {text}
          </Typography>
        </Box>
      ) : (
        <Box
          sx={{
            position: 'relative',
            zIndex: 2,
            py: 2,
            height: 120,
            overflow: 'hidden',
          }}
        >
          {/* Blurred gradient overlays */}
          <Box
            sx={{
              position: 'absolute',
              left: 0,
              right: 0,
              top: 0,
              height: '35%',
              pointerEvents: 'none',
              zIndex: 3,
              background: theme =>
                theme.palette.mode === 'light'
                  ? 'linear-gradient(to bottom, rgba(245,245,250,0.95) 70%, rgba(245,245,250,0.0) 100%)'
                  : 'linear-gradient(to bottom, rgba(30,32,40,0.95) 70%, rgba(30,32,40,0.0) 100%)',
              backdropFilter: 'blur(1px)',
            }}
          />
          <Box
            sx={{
              position: 'absolute',
              left: 0,
              right: 0,
              bottom: 0,
              height: '35%',
              pointerEvents: 'none',
              zIndex: 3,
              background: theme =>
                theme.palette.mode === 'light'
                  ? 'linear-gradient(to top, rgba(245,245,250,0.95) 70%, rgba(245,245,250,0.0) 100%)'
                  : 'linear-gradient(to top, rgba(30,32,40,0.95) 70%, rgba(30,32,40,0.0) 100%)',
              backdropFilter: 'blur(1px)',
            }}
          />
          <Box
            ref={textContainerRef}
            sx={{
              height: '100%',
              overflowY: 'auto',
              zIndex: 2,
              position: 'relative',
              px: { xs: 0, sm: 2 },
              ...lightTheme.scrollbar,
            }}
          >
            <Typography
              variant="body1"
              sx={{
                color: 'text.primary',
                fontSize: 10,
                fontFamily: 'inherit',
                whiteSpace: 'pre-line',
              }}
            >
              {text}
            </Typography>
          </Box>
        </Box>
      )}
    </Box>
  );
};

export default ThinkingWidget; 