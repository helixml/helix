import React, { FC, useState } from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import { useTheme } from "@mui/material/styles";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { preserveDisclosureExpansion } from "./disclosureScroll";

const USER_REQUEST_SPLIT =
  /^([\s\S]*?)\n\n\*\*(User Request|Original Request[^*]*?):\*\*\n?([\s\S]*)$/;

// The post-approval implementation prompt is rendered by
// approvalPromptTemplate in api/pkg/services/agent_instruction_service.go
// and always starts with this literal heading. Anchoring on `^` prevents
// a user message that happens to quote the phrase from being eaten.
const APPROVAL_PROMPT_ANCHOR = /^## CURRENT PHASE: IMPLEMENTATION\b/;

export type SplitKind = "user-request" | "approval" | null;

export interface SplitResult {
  prefix: string | null;
  userText: string;
  label: string | null;
  kind: SplitKind;
}

export function splitSystemPrefix(message: string): SplitResult {
  if (!message) {
    return { prefix: null, userText: message, label: null, kind: null };
  }

  const match = message.match(USER_REQUEST_SPLIT);
  if (match) {
    return {
      prefix: match[1].trim(),
      userText: match[3].trim(),
      label: match[2],
      kind: "user-request",
    };
  }

  if (APPROVAL_PROMPT_ANCHOR.test(message)) {
    return {
      prefix: message.trim(),
      userText: "",
      label: null,
      kind: "approval",
    };
  }

  return { prefix: null, userText: message, label: null, kind: null };
}

interface CollapsibleSystemPrefixProps {
  prefix: string;
  label?: string;
  tone?: "neutral" | "success";
}

export const CollapsibleSystemPrefix: FC<CollapsibleSystemPrefixProps> = ({
  prefix,
  label = "Planning Instructions",
  tone = "neutral",
}) => {
  const [expanded, setExpanded] = useState(false);
  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";

  return (
    <Box
      sx={{
        mb: 0.75,
        alignSelf: "stretch",
        maxWidth: "100%",
        fontFamily: "inherit",
      }}
    >
      <Box
        component="button"
        type="button"
        aria-expanded={expanded}
        onClick={(event) => {
          if (!expanded) preserveDisclosureExpansion(event.currentTarget);
          setExpanded(!expanded);
        }}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.75,
          width: "100%",
          px: 0.25,
          py: 0.5,
          border: 0,
          cursor: "pointer",
          color: "text.secondary",
          font: "inherit",
          textAlign: "left",
          backgroundColor: "transparent",
          "&:hover": {
            color: "text.primary",
            backgroundColor: "transparent",
          },
          "&:focus-visible": {
            outline: `1px solid ${theme.palette.primary.main}`,
            outlineOffset: 2,
            borderRadius: 0.5,
          },
          transition: "color 0.15s ease",
          userSelect: "none",
        }}
      >
        {tone === "success" ? (
          <CheckCircleOutlineIcon
            sx={{ fontSize: 15, color: "success.main", flexShrink: 0 }}
          />
        ) : (
          <InfoOutlinedIcon sx={{ fontSize: 15, flexShrink: 0 }} />
        )}
        <Typography
          variant="body2"
          sx={{
            flex: 1,
            fontSize: "0.8rem",
            lineHeight: 1.4,
            fontWeight: 500,
            fontFamily: "inherit",
            color: "inherit",
          }}
        >
          {label}
        </Typography>
        {expanded ? (
          <ExpandLessIcon sx={{ fontSize: 16, flexShrink: 0 }} />
        ) : (
          <ExpandMoreIcon sx={{ fontSize: 16, flexShrink: 0 }} />
        )}
      </Box>

      {expanded && (
        <Box
          sx={{
            ml: 0.9,
            pl: 2,
            pr: 0.5,
            pt: 0.25,
            pb: 0.75,
            borderLeft: `1px solid ${isDark ? "rgba(255,255,255,0.1)" : "rgba(0,0,0,0.1)"}`,
            fontSize: "0.8rem",
            lineHeight: 1.55,
            fontFamily: "inherit",
            color: "text.secondary",
            backgroundColor: "transparent",
            maxHeight: "320px",
            overflow: "auto",
            "& > :first-of-type": { mt: 0 },
            "& > :last-child": { mb: 0 },
            "& h1, & h2, & h3": {
              mt: 1.25,
              mb: 0.5,
              fontSize: "0.72rem",
              lineHeight: 1.4,
              fontWeight: 600,
              fontFamily: "inherit",
              letterSpacing: "0.06em",
              textTransform: "uppercase",
              color: "text.primary",
            },
            "& p": { my: 0.5 },
            "& ul, & ol": { my: 0.5, pl: 2.5 },
            "& li": { mb: 0.25, pl: 0.25 },
            "& hr": {
              my: 1.25,
              border: 0,
              borderTop: `1px solid ${isDark ? "rgba(255,255,255,0.08)" : "rgba(0,0,0,0.08)"}`,
            },
            "& pre": {
              backgroundColor: isDark
                ? "rgba(255,255,255,0.06)"
                : "rgba(0,0,0,0.05)",
              p: 1,
              borderRadius: 1,
              overflow: "auto",
            },
            "& code": {
              fontFamily: "monospace",
              fontSize: "0.76rem",
            },
          }}
        >
          <ReactMarkdown remarkPlugins={[remarkGfm]}>{prefix}</ReactMarkdown>
        </Box>
      )}
    </Box>
  );
};

export default CollapsibleSystemPrefix;
