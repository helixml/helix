import React, { useEffect, useMemo, useState } from "react";
import { Box, keyframes } from "@mui/material";

const CONFETTI_COUNT = 60;
const COLORS = [
  "#f59e0b",
  "#10b981",
  "#3b82f6",
  "#8b5cf6",
  "#ef4444",
  "#ec4899",
  "#14b8a6",
  "#eab308",
];

const fall = keyframes`
  0% {
    transform: translate3d(0, -10vh, 0) rotate(0deg) scale(0.6);
    opacity: 0;
  }
  10% {
    opacity: 1;
  }
  100% {
    transform: translate3d(var(--drift, 0px), 110vh, 0) rotate(720deg) scale(1);
    opacity: 0;
  }
`;

interface ConfettiPiece {
  left: number;
  drift: number;
  size: number;
  color: string;
  delay: number;
  duration: number;
  shape: "circle" | "square" | "rect";
}

const generatePieces = (): ConfettiPiece[] =>
  Array.from({ length: CONFETTI_COUNT }, () => ({
    left: Math.random() * 100,
    drift: (Math.random() - 0.5) * 200,
    size: 6 + Math.random() * 10,
    color: COLORS[Math.floor(Math.random() * COLORS.length)],
    delay: Math.random() * 800,
    duration: 1600 + Math.random() * 1400,
    shape: (["circle", "square", "rect"] as const)[
      Math.floor(Math.random() * 3)
    ],
  }));

interface TaDaAnimationProps {
  show: boolean;
  onComplete?: () => void;
  durationMs?: number;
}

const TaDaAnimation: React.FC<TaDaAnimationProps> = ({
  show,
  onComplete,
  durationMs = 2500,
}) => {
  const pieces = useMemo(() => (show ? generatePieces() : []), [show]);
  const [mounted, setMounted] = useState(show);

  useEffect(() => {
    if (!show) return;
    setMounted(true);
    const timer = window.setTimeout(() => {
      setMounted(false);
      onComplete?.();
    }, durationMs);
    return () => window.clearTimeout(timer);
  }, [show, durationMs, onComplete]);

  if (!mounted) return null;

  return (
    <Box
      aria-hidden
      sx={{
        position: "fixed",
        top: 0,
        left: 0,
        width: "100vw",
        height: "100vh",
        pointerEvents: "none",
        zIndex: 9999,
        overflow: "hidden",
      }}
    >
      {pieces.map((p, i) => (
        <Box
          key={i}
          sx={{
            position: "absolute",
            top: 0,
            left: `${p.left}vw`,
            width:
              p.shape === "rect" ? `${p.size * 0.5}px` : `${p.size}px`,
            height:
              p.shape === "rect" ? `${p.size * 1.4}px` : `${p.size}px`,
            backgroundColor: p.color,
            borderRadius: p.shape === "circle" ? "50%" : "2px",
            boxShadow: `0 0 6px ${p.color}66`,
            animation: `${fall} ${p.duration}ms cubic-bezier(0.25, 0.46, 0.45, 0.94) ${p.delay}ms forwards`,
            ["--drift" as any]: `${p.drift}px`,
          }}
        />
      ))}
    </Box>
  );
};

export default TaDaAnimation;
