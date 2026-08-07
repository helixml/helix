import CheckIcon from "@mui/icons-material/Check";
import ContentCopyIcon from "@mui/icons-material/ContentCopy";
import WrapTextIcon from "@mui/icons-material/WrapText";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { useTheme } from "@mui/material/styles";
import React, { FC, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Prism as SyntaxHighlighterTS } from "react-syntax-highlighter";
import { oneDark } from "react-syntax-highlighter/dist/esm/styles/prism";

import { APP_MONO_FONT_FAMILY } from "../../styles/typography";
import { getChatColors } from "./chatStyles";

const SyntaxHighlighter = SyntaxHighlighterTS as any;

interface MarkdownCodeBlockProps {
  children: string;
  language?: string;
}

const MarkdownCodeBlock: FC<MarkdownCodeBlockProps> = React.memo(
  ({ children, language = "text" }) => {
    const [copied, setCopied] = useState(false);
    const [wrapped, setWrapped] = useState(false);
    const copiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
    const theme = useTheme();
    const chatColors = getChatColors(theme);
    const code = useMemo(() => children.replace(/\n$/, ""), [children]);

    const handleCopy = useCallback(async () => {
      try {
        await navigator.clipboard.writeText(code);
        if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current);
        setCopied(true);
        copiedTimerRef.current = setTimeout(() => {
          setCopied(false);
          copiedTimerRef.current = null;
        }, 1200);
      } catch (error) {
        console.error("Failed to copy code", error);
      }
    }, [code]);

    useEffect(
      () => () => {
        if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current);
      },
      [],
    );

    return (
      <Box
        data-chat-code-block
        data-wrap={wrapped ? "true" : "false"}
        sx={{
          my: "0.65rem",
          overflow: "hidden",
          border: "1px solid",
          borderColor: chatColors.codeBorder,
          borderRadius: "10px",
          backgroundColor: chatColors.codeSurface,
          color: chatColors.codeForeground,
        }}
      >
        <Box
          data-chat-code-block-header
          sx={{
            minHeight: 34,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 1,
            pl: 1.5,
            pr: 0.5,
            borderBottom: "1px solid",
            borderColor: chatColors.codeBorder,
            backgroundColor: chatColors.codeHeaderSurface,
            color: chatColors.codeChrome,
          }}
        >
          <Box
            component="span"
            sx={{
              minWidth: 0,
              overflow: "hidden",
              textOverflow: "ellipsis",
              whiteSpace: "nowrap",
              fontFamily: APP_MONO_FONT_FAMILY,
              fontSize: "0.6875rem",
              lineHeight: 1,
            }}
          >
            {language}
          </Box>
          <Box role="toolbar" aria-label="Code block actions" sx={{ display: "flex", alignItems: "center" }}>
            <Tooltip title={wrapped ? "Disable line wrap" : "Wrap lines"}>
              <IconButton
                aria-label={wrapped ? "Disable line wrap" : "Wrap lines"}
                aria-pressed={wrapped}
                onClick={() => setWrapped((value) => !value)}
                size="small"
                sx={{
                  width: 28,
                  height: 28,
                  borderRadius: "7px",
                  color: wrapped ? chatColors.codeForeground : chatColors.codeChrome,
                  backgroundColor: wrapped ? chatColors.codeActionActive : "transparent",
                  "&:hover": {
                    color: chatColors.codeForeground,
                    backgroundColor: chatColors.codeActionHover,
                  },
                }}
              >
                <WrapTextIcon sx={{ fontSize: 16 }} />
              </IconButton>
            </Tooltip>
            <Tooltip title={copied ? "Copied" : "Copy code"}>
              <IconButton
                aria-label={copied ? "Copied" : "Copy code"}
                onClick={handleCopy}
                size="small"
                sx={{
                  width: 28,
                  height: 28,
                  borderRadius: "7px",
                  color: copied ? chatColors.codeForeground : chatColors.codeChrome,
                  "&:hover": {
                    color: chatColors.codeForeground,
                    backgroundColor: chatColors.codeActionHover,
                  },
                }}
              >
                {copied ? <CheckIcon sx={{ fontSize: 15 }} /> : <ContentCopyIcon sx={{ fontSize: 15 }} />}
              </IconButton>
            </Tooltip>
          </Box>
        </Box>
        <Box
          data-chat-code-block-body
          sx={{
            overflowX: wrapped ? "hidden" : "auto",
            overflowY: "clip",
            scrollbarWidth: "thin",
          }}
        >
          <SyntaxHighlighter
            language={language}
            style={oneDark}
            PreTag="div"
            wrapLongLines={wrapped}
            customStyle={{
              margin: 0,
              padding: "14px 16px 16px",
              overflow: "visible",
              background: "transparent",
              color: chatColors.codeForeground,
              fontFamily: APP_MONO_FONT_FAMILY,
              fontSize: "0.8125rem",
              lineHeight: 1.55,
            }}
            codeTagProps={{
              style: {
                fontFamily: APP_MONO_FONT_FAMILY,
              },
            }}
          >
            {code}
          </SyntaxHighlighter>
        </Box>
      </Box>
    );
  },
);

MarkdownCodeBlock.displayName = "MarkdownCodeBlock";

export default MarkdownCodeBlock;
