import React, { FC, useState } from "react";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Typography from "@mui/material/Typography";
import { useTheme } from "@mui/material/styles";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import InfoOutlinedIcon from "@mui/icons-material/InfoOutlined";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";

const USER_REQUEST_SPLIT =
  /^([\s\S]*?)\n\n\*\*(User Request|Original Request[^*]*?):\*\*\n?([\s\S]*)$/;

export interface SplitResult {
  prefix: string | null;
  userText: string;
  label: string | null;
}

export function splitSystemPrefix(message: string): SplitResult {
  if (!message) return { prefix: null, userText: message, label: null };
  const match = message.match(USER_REQUEST_SPLIT);
  if (!match) return { prefix: null, userText: message, label: null };
  return {
    prefix: match[1].trim(),
    userText: match[3].trim(),
    label: match[2],
  };
}

interface CollapsibleSystemPrefixProps {
  prefix: string;
  label?: string;
}

export const CollapsibleSystemPrefix: FC<CollapsibleSystemPrefixProps> = ({
  prefix,
  label = "Planning Instructions",
}) => {
  const [expanded, setExpanded] = useState(false);
  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";

  return (
    <Box
      sx={{
        mb: 1,
        borderLeft: `3px solid ${isDark ? "rgba(255,255,255,0.15)" : "rgba(0,0,0,0.12)"}`,
        borderRadius: "4px",
        overflow: "hidden",
        alignSelf: "stretch",
        maxWidth: "100%",
      }}
    >
      <Box
        onClick={() => setExpanded(!expanded)}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.75,
          px: 1.5,
          py: 0.75,
          cursor: "pointer",
          backgroundColor: isDark
            ? "rgba(255,255,255,0.04)"
            : "rgba(0,0,0,0.03)",
          "&:hover": {
            backgroundColor: isDark
              ? "rgba(255,255,255,0.08)"
              : "rgba(0,0,0,0.06)",
          },
          transition: "background-color 0.15s ease",
          userSelect: "none",
        }}
      >
        <InfoOutlinedIcon
          sx={{
            fontSize: 16,
            color: isDark ? "rgba(255,255,255,0.5)" : "rgba(0,0,0,0.45)",
          }}
        />
        <Typography
          variant="body2"
          sx={{
            flex: 1,
            fontSize: "0.82rem",
            fontStyle: "italic",
            color: isDark ? "rgba(255,255,255,0.7)" : "rgba(0,0,0,0.6)",
          }}
        >
          {label}
        </Typography>
        <IconButton size="small" sx={{ p: 0, ml: 0.5 }} aria-label="toggle">
          {expanded ? (
            <ExpandLessIcon sx={{ fontSize: 18 }} />
          ) : (
            <ExpandMoreIcon sx={{ fontSize: 18 }} />
          )}
        </IconButton>
      </Box>

      {expanded && (
        <Box
          sx={{
            px: 1.5,
            py: 1,
            fontSize: "0.85rem",
            color: isDark ? "rgba(255,255,255,0.75)" : "rgba(0,0,0,0.7)",
            backgroundColor: isDark
              ? "rgba(255,255,255,0.02)"
              : "rgba(0,0,0,0.015)",
            borderTop: `1px solid ${isDark ? "rgba(255,255,255,0.06)" : "rgba(0,0,0,0.06)"}`,
            maxHeight: "400px",
            overflow: "auto",
            "& p": { my: 0.5 },
            "& pre": {
              backgroundColor: isDark
                ? "rgba(255,255,255,0.06)"
                : "rgba(0,0,0,0.05)",
              p: 1,
              borderRadius: "4px",
              overflow: "auto",
            },
            "& code": {
              fontFamily: "monospace",
              fontSize: "0.8rem",
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
