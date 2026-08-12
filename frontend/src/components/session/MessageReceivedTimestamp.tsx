import { FC } from "react";
import Box from "@mui/material/Box";
import Tooltip from "@mui/material/Tooltip";

export interface FormattedMessageReceivedAt {
  full: string;
  short: string;
}

export const formatMessageReceivedAt = (
  completedAt?: string,
): FormattedMessageReceivedAt | null => {
  if (!completedAt) return null;

  const date = new Date(completedAt);
  if (!Number.isFinite(date.getTime()) || date.getUTCFullYear() <= 1) return null;

  return {
    short: date.toLocaleTimeString(undefined, {
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }),
    full: date.toLocaleString(undefined, {
      dateStyle: "full",
      timeStyle: "long",
    }),
  };
};

const MessageReceivedTimestamp: FC<{ completedAt?: string }> = ({
  completedAt,
}) => {
  const formatted = formatMessageReceivedAt(completedAt);
  if (!formatted) return null;

  return (
    <Tooltip title={formatted.full} placement="top">
      <Box
        component="time"
        dateTime={completedAt}
        aria-label={`Received ${formatted.full}`}
        sx={{
          alignItems: "center",
          color: "text.disabled",
          display: "flex",
          flexShrink: 0,
          fontSize: "0.7rem",
          fontVariantNumeric: "tabular-nums",
          height: 28,
          lineHeight: 1,
          mt: 0.5,
          userSelect: "none",
        }}
      >
        {formatted.short}
      </Box>
    </Tooltip>
  );
};

export default MessageReceivedTimestamp;
