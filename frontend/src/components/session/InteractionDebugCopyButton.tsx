import React, { FC, useState } from "react";
import BugReportOutlinedIcon from "@mui/icons-material/BugReportOutlined";
import CheckIcon from "@mui/icons-material/Check";
import IconButton from "@mui/material/IconButton";
import Tooltip from "@mui/material/Tooltip";

import {
  TypesInteraction,
  TypesServerConfigForFrontend,
  TypesSession,
} from "../../api/api";
import { useStreaming } from "../../contexts/streaming";
import { copyTextToClipboard } from "../../utils/clipboard";
import { buildInteractionDebugContext } from "./interactionDebugContext";

interface InteractionDebugCopyButtonProps {
  interaction: TypesInteraction;
  session: TypesSession;
  sessionSteps: unknown[];
  serverConfig: TypesServerConfigForFrontend;
}

const InteractionDebugCopyButton: FC<InteractionDebugCopyButtonProps> = ({
  interaction,
  session,
  sessionSteps,
  serverConfig,
}) => {
  const [copied, setCopied] = useState(false);
  const { currentResponses } = useStreaming();

  const liveInteraction = currentResponses.get(session.id || "");
  const interactionForCopy =
    liveInteraction?.id === interaction.id
      ? ({ ...interaction, ...liveInteraction } as unknown as TypesInteraction)
      : interaction;

  const handleCopy = async (event: React.MouseEvent) => {
    event.preventDefault();
    event.stopPropagation();

    const context = buildInteractionDebugContext(
      interactionForCopy,
      session,
      sessionSteps.filter(
        (step) =>
          typeof step === "object" &&
          step !== null &&
          "interaction_id" in step &&
          (step as { interaction_id?: unknown }).interaction_id === interaction.id,
      ),
      serverConfig,
      {
        capturedAt: new Date().toISOString(),
        sourceUrl: window.location.href,
        userAgent: navigator.userAgent,
      },
    );

    await copyTextToClipboard(context);

    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Tooltip
      title={copied ? "Debug context copied" : "Copy interaction debug context"}
      placement="bottom"
    >
      <IconButton
        size="small"
        onClick={handleCopy}
        aria-label="Copy interaction debug context"
        sx={(theme) => ({
          mt: 0.5,
          p: "2px",
          color: theme.palette.mode === "light" ? "#888" : "#bbb",
          "&:hover": {
            color: theme.palette.mode === "light" ? "#000" : "#fff",
          },
        })}
      >
        {copied ? (
          <CheckIcon sx={{ fontSize: 20 }} />
        ) : (
          <BugReportOutlinedIcon sx={{ fontSize: 20 }} />
        )}
      </IconButton>
    </Tooltip>
  );
};

export default InteractionDebugCopyButton;
