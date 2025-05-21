import React, { useEffect, useRef, useState } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';

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

  // Split text into lines for custom rendering
  const lines = text.split('\n');
  const totalLines = lines.length;
  const visibleCount = 2;
  const blurredCount = 3;
  const startBlur = Math.max(0, totalLines - visibleCount - blurredCount);
  const startVisible = Math.max(0, totalLines - visibleCount);

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
        <Typography variant="subtitle2" sx={{ fontWeight: 600, color: 'primary.main' }}>
          💡 {isStreaming ? `Thinking ${formatDuration(elapsed)}` : `Thoughts`}
        </Typography>
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
            minHeight: 60,
          }}
        >
          {/* Blurred lines */}
          {lines.slice(startBlur, startVisible).map((line, idx) => (
            <Typography
              key={`blurred-${idx}`}
              variant="body1"
              sx={{
                color: 'text.primary',
                px: { xs: 0, sm: 2 },
                fontSize: 16,
                fontFamily: 'inherit',
                whiteSpace: 'pre-line',
                filter: 'blur(4px)',
                opacity: 0.7,
                pointerEvents: 'none',
                userSelect: 'none',
              }}
            >
              {line}
            </Typography>
          ))}
          {/* Fully visible lines */}
          {lines.slice(startVisible).map((line, idx) => (
            <Typography
              key={`visible-${idx}`}
              variant="body1"
              sx={{
                color: 'text.primary',
                px: { xs: 0, sm: 2 },
                fontSize: 16,
                fontFamily: 'inherit',
                whiteSpace: 'pre-line',
              }}
            >
              {line}
            </Typography>
          ))}
        </Box>
      )}
    </Box>
  );
};

export default ThinkingWidget; 