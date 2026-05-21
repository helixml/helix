import React, { useCallback, useRef, useEffect } from "react";
import { Paper, Box, TextField, Button, Typography, CircularProgress } from "@mui/material";

interface InlineCommentFormProps {
  show: boolean;
  yPos: number;
  selectedText: string;
  commentText: string;
  onCommentChange: (value: string) => void;
  onCreate: () => void;
  onCancel: () => void;
  isNarrowViewport?: boolean;
  isSubmitting?: boolean;
  // Optional outer ref used by the parent to measure the rendered form
  // height — needed so the bubble-stacking algorithm can include this form.
  outerRef?: (el: HTMLDivElement | null) => void;
}

export default function InlineCommentForm({
  show,
  yPos,
  selectedText,
  commentText,
  onCommentChange,
  onCreate,
  onCancel,
  isNarrowViewport = false,
  isSubmitting = false,
  outerRef,
}: InlineCommentFormProps) {
  const paperRef = useRef<HTMLDivElement>(null);

  // Stable ref callback. Defining it inline gives it a new identity on every
  // render, which makes React invoke it with (null) then (node) every time,
  // and when outerRef bumps a measure-tick state in the parent, that loops.
  const setRefs = useCallback(
    (el: HTMLDivElement | null) => {
      (paperRef as React.MutableRefObject<HTMLDivElement | null>).current = el;
      if (outerRef) outerRef(el);
    },
    [outerRef],
  );

  // Auto-scroll to ensure the comment form is visible after it appears
  useEffect(() => {
    if (show && paperRef.current) {
      // Small delay to ensure the element is rendered
      setTimeout(() => {
        paperRef.current?.scrollIntoView({
          behavior: "smooth",
          block: "nearest",
        });
      }, 100);
    }
  }, [show, yPos]);

  if (!show || !selectedText) return null;

  // On narrow viewports (tablets), render as a bottom sheet style overlay
  // On wide viewports, keep the original side positioning
  const narrowStyles = {
    position: "fixed" as const,
    left: "50%",
    bottom: "20px",
    transform: "translateX(-50%)",
    width: "calc(100% - 32px)",
    maxWidth: "500px",
    top: "auto",
  };

  const wideStyles = {
    position: "absolute" as const,
    left: "820px",
    top: `${yPos}px`,
    width: "300px",
    transform: "none",
    bottom: "auto",
  };

  return (
    <Paper
      ref={setRefs}
      sx={{
        ...(isNarrowViewport ? narrowStyles : wideStyles),
        p: 2,
        bgcolor: "background.paper",
        border: "2px solid",
        borderColor: "primary.main",
        boxShadow: "0 4px 12px rgba(0,0,0,0.2)",
        zIndex: 20,
      }}
    >
      <Typography variant="subtitle2" sx={{ mb: 1 }}>
        Add Comment
      </Typography>

      <Box
        sx={{
          bgcolor: "action.hover",
          p: 1,
          borderLeft: "3px solid",
          borderColor: "primary.main",
          mb: 1.5,
          fontStyle: "italic",
          fontSize: "0.75rem",
          maxHeight: isNarrowViewport ? "60px" : "none",
          overflow: "auto",
        }}
      >
        "
        {selectedText.length > 100
          ? selectedText.substring(0, 100) + "..."
          : selectedText}
        "
      </Box>

      <TextField
        fullWidth
        multiline
        rows={isNarrowViewport ? 2 : 3}
        value={commentText}
        onChange={(e) => onCommentChange(e.target.value)}
        onKeyDown={(e) => {
          // Cmd+Enter (Mac) or Ctrl+Enter (Windows/Linux) to submit
          if (
            e.key === "Enter" &&
            (e.metaKey || e.ctrlKey) &&
            commentText.trim()
          ) {
            e.preventDefault();
            onCreate();
          }
        }}
        placeholder="Add your comment... (Cmd+Enter to submit)"
        autoFocus
        sx={{ mb: 1.5 }}
      />

      <Box display="flex" gap={1} justifyContent="flex-end">
        <Button size="small" onClick={onCancel}>
          Cancel
        </Button>
        <Button
          size="small"
          variant="contained"
          onClick={onCreate}
          disabled={!commentText.trim() || isSubmitting}
          startIcon={isSubmitting ? <CircularProgress size={16} color="inherit" /> : undefined}
        >
          Comment
        </Button>
      </Box>
    </Paper>
  );
}
