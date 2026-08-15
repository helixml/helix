import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";
import { useTheme } from "@mui/material/styles";
import { Check, Copy, WrapText } from "lucide-react";
import React, { FC, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Prism as SyntaxHighlighterTS } from "react-syntax-highlighter";
import { oneDark, oneLight } from "react-syntax-highlighter/dist/esm/styles/prism";

import { APP_MONO_FONT_FAMILY, TYPOGRAPHY } from "../../styles/typography";
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
    const syntaxTheme = theme.palette.mode === "dark" ? oneDark : oneLight;
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

    const actionSx = {
      width: 24,
      height: 24,
      borderRadius: "6px",
      color: chatColors.codeChrome,
      "&:hover": {
        color: chatColors.codeForeground,
        backgroundColor: chatColors.codeActionHover,
      },
    } as const;

    return (
      <Box
        data-chat-code-block
        data-language={language}
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
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 1,
            pt: 0.75,
            pr: 0.75,
            pb: 0,
            pl: 1.5,
            userSelect: "none",
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
              fontSize: TYPOGRAPHY.codeChromeFontSize,
              lineHeight: 1,
            }}
          >
            {language}
          </Box>
          <Box
            role="toolbar"
            aria-label="Code block actions"
            sx={{ display: "flex", alignItems: "center", gap: 0.25 }}
          >
            <Tooltip title={wrapped ? "Disable line wrap" : "Wrap lines"}>
              <IconButton
                aria-label={wrapped ? "Disable line wrap" : "Wrap lines"}
                aria-pressed={wrapped}
                onClick={() => setWrapped((value) => !value)}
                size="small"
                sx={{
                  ...actionSx,
                  ...(wrapped
                    ? {
                        color: chatColors.codeForeground,
                        backgroundColor: chatColors.codeActionActive,
                      }
                    : {}),
                }}
              >
                <WrapText size={14} />
              </IconButton>
            </Tooltip>
            <Tooltip title={copied ? "Copied" : "Copy code"}>
              <IconButton
                aria-label={copied ? "Copied" : "Copy code"}
                onClick={handleCopy}
                size="small"
                sx={{
                  ...actionSx,
                  ...(copied ? { color: chatColors.codeForeground } : {}),
                }}
              >
                {copied ? <Check size={14} /> : <Copy size={14} />}
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
            style={syntaxTheme}
            wrapLongLines={wrapped}
            customStyle={{
              margin: 0,
              padding: "0.6rem 0.9rem 0.8rem",
              border: "none",
              borderRadius: 0,
              overflow: "visible",
              background: "transparent",
              color: chatColors.codeForeground,
              textShadow: "none",
              fontFamily: APP_MONO_FONT_FAMILY,
              fontSize: TYPOGRAPHY.codeFontSize,
              lineHeight: TYPOGRAPHY.codeLineHeight,
            }}
            codeTagProps={{
              style: {
                background: "transparent",
                textShadow: "none",
                fontFamily: APP_MONO_FONT_FAMILY,
                fontSize: "inherit",
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
