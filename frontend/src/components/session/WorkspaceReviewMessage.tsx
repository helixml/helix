import { useMemo, useState, type FC, type MouseEvent } from "react";
import Box from "@mui/material/Box";
import Chip from "@mui/material/Chip";
import Popover from "@mui/material/Popover";
import Tooltip from "@mui/material/Tooltip";
import Typography from "@mui/material/Typography";
import { MessageCircle } from "lucide-react";

import { TypesSession } from "../../api/api";
import { APP_MONO_FONT_FAMILY, TYPOGRAPHY } from "../../styles/typography";
import Markdown from "./Markdown";
import {
  parseWorkspaceReviewMessage,
  type WorkspaceReviewMessageComment,
} from "./workspaceReviewMessage";

interface WorkspaceReviewMessageProps {
  text: string;
  session: TypesSession;
  getFileURL: (filename: string) => string;
}

function basename(filePath: string): string {
  return filePath.replace(/\\/g, "/").split("/").at(-1) || filePath;
}

const WorkspaceReviewCommentContext: FC<{
  comment: WorkspaceReviewMessageComment;
}> = ({ comment }) => {
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null);
  const label = [basename(comment.filePath), comment.rangeLabel]
    .filter(Boolean)
    .join(" · ");

  const handleOpen = (event: MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  return (
    <>
      <Tooltip title={`View context in ${comment.filePath}`}>
        <Chip
          icon={<MessageCircle size={14} />}
          label={label}
          size="small"
          variant="outlined"
          onClick={handleOpen}
          aria-label={`View ${comment.sectionTitle.toLowerCase()} context: ${label}`}
          sx={{
            alignSelf: "flex-start",
            maxWidth: "100%",
            height: 26,
            borderRadius: 1,
            color: "text.secondary",
            borderColor: "divider",
            bgcolor: "transparent",
            fontFamily: APP_MONO_FONT_FAMILY,
            fontSize: TYPOGRAPHY.codeChromeFontSize,
            "&:hover": {
              color: "text.primary",
              bgcolor: "action.hover",
            },
            "& .MuiChip-icon": {
              ml: 0.75,
              color: "inherit",
            },
            "& .MuiChip-label": {
              px: 0.75,
              overflow: "hidden",
              textOverflow: "ellipsis",
            },
          }}
        />
      </Tooltip>
      <Popover
        open={Boolean(anchorEl)}
        anchorEl={anchorEl}
        onClose={() => setAnchorEl(null)}
        disableScrollLock
        anchorOrigin={{ vertical: "bottom", horizontal: "left" }}
        transformOrigin={{ vertical: "top", horizontal: "left" }}
        slotProps={{
          paper: {
            sx: {
              width: 440,
              maxWidth: "calc(100vw - 32px)",
              mt: 0.5,
              p: 1.5,
              border: "1px solid",
              borderColor: "divider",
              borderRadius: 1.5,
              boxShadow: 4,
              bgcolor: "background.paper",
            },
          },
        }}
      >
        <Typography
          title={comment.filePath}
          sx={{
            overflow: "hidden",
            textOverflow: "ellipsis",
            whiteSpace: "nowrap",
            fontFamily: APP_MONO_FONT_FAMILY,
            fontSize: TYPOGRAPHY.codeFontSize,
            fontWeight: 600,
          }}
        >
          {comment.filePath}
        </Typography>
        <Typography variant="caption" color="text.secondary">
          {[comment.sectionTitle, comment.rangeLabel].filter(Boolean).join(" · ")}
        </Typography>
        {comment.contents && (
          <Box
            component="pre"
            sx={{
              m: 0,
              mt: 1.25,
              pl: 1.25,
              maxHeight: 240,
              overflow: "auto",
              borderLeft: "2px solid",
              borderColor: "divider",
              color: "text.secondary",
              fontFamily: APP_MONO_FONT_FAMILY,
              fontSize: TYPOGRAPHY.codeFontSize,
              lineHeight: TYPOGRAPHY.codeLineHeight,
              whiteSpace: "pre-wrap",
              overflowWrap: "anywhere",
            }}
          >
            {comment.contents}
          </Box>
        )}
      </Popover>
    </>
  );
};

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
      sx={{ display: "flex", flexDirection: "column", gap: 1 }}
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
              display: "flex",
              minWidth: 0,
              flexDirection: "column",
              alignItems: "flex-start",
              gap: 0.75,
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
            <WorkspaceReviewCommentContext comment={segment} />
          </Box>
        );
      })}
    </Box>
  );
};

export default WorkspaceReviewMessage;
