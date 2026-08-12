import { FC, useMemo } from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import { FileCode2 } from "lucide-react";

import { TypesSession } from "../../api/api";
import { APP_MONO_FONT_FAMILY } from "../../styles/typography";
import Markdown from "./Markdown";
import MarkdownCodeBlock from "./MarkdownCodeBlock";
import { parseWorkspaceReviewMessage } from "./workspaceReviewMessage";

interface WorkspaceReviewMessageProps {
  text: string;
  session: TypesSession;
  getFileURL: (filename: string) => string;
}

const WorkspaceReviewMessage: FC<WorkspaceReviewMessageProps> = ({
  text,
  session,
  getFileURL,
}) => {
  const segments = useMemo(() => parseWorkspaceReviewMessage(text), [text]);
  const hasComments = segments.some((segment) => segment.type === "comment");

  if (!hasComments) {
    return (
      <Markdown
        text={text}
        session={session}
        getFileURL={getFileURL}
        showBlinker={false}
        isStreaming={false}
      />
    );
  }

  return (
    <Box
      data-workspace-review-message
      sx={{ display: "flex", flexDirection: "column", gap: 1.25 }}
    >
      {segments.map((segment, index) => {
        if (segment.type === "text") {
          return (
            <Markdown
              key={`text-${index}`}
              text={segment.text}
              session={session}
              getFileURL={getFileURL}
              showBlinker={false}
              isStreaming={false}
            />
          );
        }

        return (
          <Box
            key={`comment-${index}-${segment.filePath}-${segment.rangeLabel}`}
            data-workspace-review-comment
            sx={{
              width: 640,
              maxWidth: "100%",
              overflow: "hidden",
              border: "1px solid",
              borderColor: "divider",
              borderRadius: 1.5,
              bgcolor: "background.default",
              boxShadow: 1,
            }}
          >
            <Box
              sx={{
                minHeight: 42,
                display: "flex",
                alignItems: "center",
                gap: 1,
                px: 1.25,
                borderBottom: "1px solid",
                borderColor: "divider",
                bgcolor: "action.hover",
              }}
            >
              <Box
                sx={{
                  width: 26,
                  height: 26,
                  display: "grid",
                  placeItems: "center",
                  flexShrink: 0,
                  borderRadius: 1,
                  color: "text.secondary",
                  bgcolor: "action.selected",
                }}
              >
                <FileCode2 size={15} />
              </Box>
              <Box sx={{ minWidth: 0, flex: 1 }}>
                <Typography
                  variant="caption"
                  color="text.secondary"
                  sx={{ display: "block", lineHeight: 1.2 }}
                >
                  File comment
                </Typography>
                <Typography
                  variant="body2"
                  title={segment.filePath}
                  sx={{
                    overflow: "hidden",
                    textOverflow: "ellipsis",
                    whiteSpace: "nowrap",
                    fontFamily: APP_MONO_FONT_FAMILY,
                    fontSize: "0.78rem",
                    fontWeight: 600,
                  }}
                >
                  {segment.filePath}
                </Typography>
              </Box>
              {segment.rangeLabel && (
                <Box
                  component="span"
                  sx={{
                    px: 0.75,
                    py: 0.25,
                    flexShrink: 0,
                    border: "1px solid",
                    borderColor: "divider",
                    borderRadius: 0.75,
                    color: "text.secondary",
                    bgcolor: "background.paper",
                    fontFamily: APP_MONO_FONT_FAMILY,
                    fontSize: "0.68rem",
                    fontWeight: 600,
                    lineHeight: 1.4,
                  }}
                >
                  {segment.rangeLabel}
                </Box>
              )}
            </Box>
            <Box
              sx={{
                p: 1.25,
                "& .interactionMessage > :first-of-type": { mt: 0 },
                "& .interactionMessage > :last-of-type": { mb: 0 },
                "& [data-chat-code-block]": { mb: 0, borderRadius: 1 },
              }}
            >
              {segment.text && (
                <Markdown
                  text={segment.text}
                  session={session}
                  getFileURL={getFileURL}
                  showBlinker={false}
                  isStreaming={false}
                />
              )}
              {segment.contents && (
                <MarkdownCodeBlock language={segment.language}>
                  {segment.contents}
                </MarkdownCodeBlock>
              )}
            </Box>
          </Box>
        );
      })}
    </Box>
  );
};

export default WorkspaceReviewMessage;
