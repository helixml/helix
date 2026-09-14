import React, { useCallback, useRef, useEffect } from "react";
import { Box } from "@mui/material";

import ReviewCommentAnnotation from "../workspace-inspector/ReviewCommentAnnotation";

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
  submitLabel?: string;
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
  submitLabel = "Comment",
  outerRef,
}: InlineCommentFormProps) {
  const paperRef = useRef<HTMLDivElement>(null);

  // Stash the latest outerRef in a ref so setRefs can stay identity-stable
  // ([] deps) regardless of how the parent declares its callback. If setRefs
  // gets a new identity on every render, React invokes it with (null) then
  // (node) each time; combined with outerRef bumping parent state (e.g. a
  // measure tick), that produces an infinite update loop.
  const outerRefRef = useRef(outerRef);
  useEffect(() => {
    outerRefRef.current = outerRef;
  }, [outerRef]);

  const setRefs = useCallback((el: HTMLDivElement | null) => {
    (paperRef as React.MutableRefObject<HTMLDivElement | null>).current = el;
    outerRefRef.current?.(el);
  }, []);

  // Keep the compact annotation visible when it opens below the selection.
  useEffect(() => {
    if (show && paperRef.current) {
      paperRef.current.scrollIntoView({ behavior: "smooth", block: "nearest" });
    }
  }, [show, yPos]);

  if (!show || !selectedText) return null;

  const narrowStyles = {
    position: "absolute" as const,
    left: "50%",
    top: `${yPos + 8}px`,
    width: "calc(100% - 32px)",
    maxWidth: 480,
    transform: "translateX(-50%)",
    bottom: "auto",
  };

  const wideStyles = {
    position: "absolute" as const,
    left: "820px",
    top: `${yPos}px`,
    width: "360px",
    transform: "none",
    bottom: "auto",
  };

  return (
    <Box
      ref={setRefs}
      data-plan-comment-draft
      sx={{
        ...(isNarrowViewport ? narrowStyles : wideStyles),
        bgcolor: "background.default",
        borderLeft: "2px solid",
        borderColor: "primary.main",
        zIndex: 20,
      }}
    >
      <ReviewCommentAnnotation
        kind="draft"
        text={commentText}
        rangeLabel="selected text"
        onTextChange={onCommentChange}
        onCancel={onCancel}
        onSubmit={onCreate}
        submitLabel={submitLabel}
        isSubmitting={isSubmitting}
      />
    </Box>
  );
}
