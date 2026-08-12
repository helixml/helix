import React, {
  FC,
  useEffect,
  useState,
  useMemo,
  useCallback,
  useRef,
} from "react";

// Throttle interval for scroll updates during streaming (ms)
const SCROLL_THROTTLE_MS = 200;
import useLiveInteraction from "../../hooks/useLiveInteraction";
import { MessageWithToolCalls } from "./InteractionInference";
import { TypesServerConfigForFrontend } from "../../api/api";
import {
  TypesInteraction,
  TypesSession,
} from "../../api/api";
import ToolStepsWidget from "./ToolStepsWidget";
import ActivitySummary from "./ActivitySummary";
import { getInteractionRequestTimeMs } from "./interactionDuration";

export const InteractionLiveStream: FC<{
  session_id: string;
  interaction: TypesInteraction;
  serverConfig?: TypesServerConfigForFrontend;
  session: TypesSession;
  onMessageChange?: {
    (message: string): void;
  };
  onMessageUpdate?: () => void;
  onFilterDocument?: (docId: string) => void;
}> = ({
  session_id,
  serverConfig,
  session,
  interaction,
  onMessageChange,
  onMessageUpdate,
  onFilterDocument,
}) => {
  const { message, responseEntries, durationMs, stepInfos, isComplete } = useLiveInteraction(
    session_id,
    interaction,
  );

  // Track if we're still in streaming mode or completed
  const [isActivelyStreaming, setIsActivelyStreaming] = useState(true);
  const activityStartedAt = useMemo(
    () => getInteractionRequestTimeMs(interaction, Date.now()),
    [interaction.id, interaction.created],
  );
  const effectiveDurationMs =
    durationMs ||
    (!isActivelyStreaming
      ? Math.max(0, Date.now() - activityStartedAt)
      : 0);

  const useClientURL = useCallback(
    (url: string) => {
      if (!url) return "";
      if (!serverConfig) return "";
      return `${serverConfig.filestore_prefix}/${url}?redirect_urls=true`;
    },
    [serverConfig],
  );

  const toolSteps = useMemo(
    () =>
      stepInfos.map((step, index) => ({
        id: `step-${index}`,
        name: step.name,
        icon: step.icon,
        type: step.type,
        message: step.message,
        details: {
          arguments: {},
        },
        created: step.created || "",
      })),
    [stepInfos],
  );

  // Reset streaming state when interaction ID changes
  useEffect(() => {
    setIsActivelyStreaming(true);
  }, [interaction?.id]);

  // Detect completion from the server (WebSocket)
  useEffect(() => {
    if (isComplete && isActivelyStreaming) {
      setIsActivelyStreaming(false);
    }
  }, [isComplete, isActivelyStreaming]);

  useEffect(() => {
    if (!message) return;
    if (!onMessageChange) return;
    onMessageChange(message);
  }, [message, onMessageChange]);

  // Throttle scroll updates during streaming to reduce CPU usage
  const scrollThrottleRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingScrollRef = useRef(false);

  // Trigger scroll on either message or entries change
  const hasContent = !!(message || (responseEntries && responseEntries.length > 0));

  useEffect(() => {
    if (!hasContent || !onMessageUpdate) return;

    if (!scrollThrottleRef.current) {
      onMessageUpdate();
      scrollThrottleRef.current = setTimeout(() => {
        scrollThrottleRef.current = null;
        if (pendingScrollRef.current) {
          pendingScrollRef.current = false;
          onMessageUpdate();
        }
      }, SCROLL_THROTTLE_MS);
    } else {
      pendingScrollRef.current = true;
    }
  }, [hasContent, message, responseEntries, onMessageUpdate]);

  useEffect(() => {
    return () => {
      if (scrollThrottleRef.current) {
        clearTimeout(scrollThrottleRef.current);
      }
    };
  }, []);

  if (!serverConfig || !serverConfig.filestore_prefix) {
    return null;
  }

  return (
    <>
      {stepInfos.length > 0 && (
        <ToolStepsWidget
          steps={toolSteps}
          isLiveStreaming={isActivelyStreaming}
        />
      )}

      {/* Show thinking indicator when waiting and no content yet */}
      {interaction.state === "waiting" && !hasContent && (
        <ActivitySummary
          hasActivity={false}
          isStreaming
          startedAt={activityStartedAt}
        />
      )}

      {hasContent && (
        <div>
          <MessageWithToolCalls
            text={message}
            responseEntries={responseEntries}
            session={session}
            getFileURL={useClientURL}
            showBlinker={true}
            isStreaming={isActivelyStreaming}
            durationMs={effectiveDurationMs}
            activityStartedAt={activityStartedAt}
            onFilterDocument={onFilterDocument}
            includeTaskChecklist
          />
        </div>
      )}
    </>
  );
};

export default React.memo(InteractionLiveStream);
