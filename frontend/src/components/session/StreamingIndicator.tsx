import React, { FC } from "react";
import Box from "@mui/material/Box";
import { keyframes } from "@mui/material/styles";

// Smooth lemniscate (figure-8) path with 48 keyframes
// Parametric: x = 18*cos(t), y = 8*sin(2t)
const infinityPath = keyframes`
  0% { transform: translate(18px, 0); }
  2.08% { transform: translate(17.9px, 2px); }
  4.17% { transform: translate(17.4px, 3.9px); }
  6.25% { transform: translate(16.6px, 5.6px); }
  8.33% { transform: translate(15.4px, 7px); }
  10.42% { transform: translate(14px, 7.9px); }
  12.5% { transform: translate(12.2px, 8.3px); }
  14.58% { transform: translate(10.3px, 8.1px); }
  16.67% { transform: translate(8.3px, 7.4px); }
  18.75% { transform: translate(6.1px, 6.2px); }
  20.83% { transform: translate(3.9px, 4.6px); }
  22.92% { transform: translate(1.7px, 2.6px); }
  25% { transform: translate(0, 0); }
  27.08% { transform: translate(-1.7px, -2.6px); }
  29.17% { transform: translate(-3.9px, -4.6px); }
  31.25% { transform: translate(-6.1px, -6.2px); }
  33.33% { transform: translate(-8.3px, -7.4px); }
  35.42% { transform: translate(-10.3px, -8.1px); }
  37.5% { transform: translate(-12.2px, -8.3px); }
  39.58% { transform: translate(-14px, -7.9px); }
  41.67% { transform: translate(-15.4px, -7px); }
  43.75% { transform: translate(-16.6px, -5.6px); }
  45.83% { transform: translate(-17.4px, -3.9px); }
  47.92% { transform: translate(-17.9px, -2px); }
  50% { transform: translate(-18px, 0); }
  52.08% { transform: translate(-17.9px, 2px); }
  54.17% { transform: translate(-17.4px, 3.9px); }
  56.25% { transform: translate(-16.6px, 5.6px); }
  58.33% { transform: translate(-15.4px, 7px); }
  60.42% { transform: translate(-14px, 7.9px); }
  62.5% { transform: translate(-12.2px, 8.3px); }
  64.58% { transform: translate(-10.3px, 8.1px); }
  66.67% { transform: translate(-8.3px, 7.4px); }
  68.75% { transform: translate(-6.1px, 6.2px); }
  70.83% { transform: translate(-3.9px, 4.6px); }
  72.92% { transform: translate(-1.7px, 2.6px); }
  75% { transform: translate(0, 0); }
  77.08% { transform: translate(1.7px, -2.6px); }
  79.17% { transform: translate(3.9px, -4.6px); }
  81.25% { transform: translate(6.1px, -6.2px); }
  83.33% { transform: translate(8.3px, -7.4px); }
  85.42% { transform: translate(10.3px, -8.1px); }
  87.5% { transform: translate(12.2px, -8.3px); }
  89.58% { transform: translate(14px, -7.9px); }
  91.67% { transform: translate(15.4px, -7px); }
  93.75% { transform: translate(16.6px, -5.6px); }
  95.83% { transform: translate(17.4px, -3.9px); }
  97.92% { transform: translate(17.9px, -2px); }
  100% { transform: translate(18px, 0); }
`;

const DURATION = 2; // seconds
const TRAIL_COUNT = 24;
const TRAIL_SPACING = 0.025; // seconds between trail elements
const OFFSET = -0.67; // 33% offset for purple to avoid collision

// Colors
const CYAN = "#00d4ff";
const PURPLE_HEAD = "#d8b4fe";
const PURPLE_TRAIL = "#a855f7";

interface StreamingIndicatorProps {
  className?: string;
}

const StreamingIndicator: FC<StreamingIndicatorProps> = ({ className }) => {
  // Generate trail elements for a given color and base delay
  const renderTrails = (color: string, baseDelay: number) => {
    return Array.from({ length: TRAIL_COUNT }, (_, i) => {
      const index = i + 1;
      const delay = baseDelay - DURATION + index * TRAIL_SPACING;
      const opacity = 0.5 - index * 0.018;
      const size = Math.max(2, 5 - index * 0.1);

      return (
        <Box
          key={index}
          sx={{
            position: "absolute",
            borderRadius: "50%",
            top: "50%",
            left: "50%",
            width: size,
            height: size,
            backgroundColor: color,
            animation: `${infinityPath} ${DURATION}s linear infinite`,
            animationDelay: `${delay}s`,
            opacity: Math.max(0, opacity),
          }}
        />
      );
    });
  };

  return (
    <Box
      className={className}
      sx={{
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        width: 44,
        height: 20,
        position: "relative",
      }}
    >
      {/* Cyan trails */}
      {renderTrails(CYAN, 0)}

      {/* Purple trails */}
      {renderTrails(PURPLE_TRAIL, OFFSET)}

      {/* Cyan dot (head) */}
      <Box
        sx={{
          position: "absolute",
          borderRadius: "50%",
          top: "50%",
          left: "50%",
          width: 5,
          height: 5,
          backgroundColor: CYAN,
          boxShadow: `0 0 3px ${CYAN}, 0 0 6px ${CYAN}`,
          animation: `${infinityPath} ${DURATION}s linear infinite`,
          animationDelay: "0s",
        }}
      />

      {/* Purple dot (head) */}
      <Box
        sx={{
          position: "absolute",
          borderRadius: "50%",
          top: "50%",
          left: "50%",
          width: 5,
          height: 5,
          backgroundColor: PURPLE_HEAD,
          boxShadow: `0 0 3px ${PURPLE_HEAD}, 0 0 6px ${PURPLE_HEAD}`,
          animation: `${infinityPath} ${DURATION}s linear infinite`,
          animationDelay: `${OFFSET}s`,
        }}
      />
    </Box>
  );
};

export default StreamingIndicator;
