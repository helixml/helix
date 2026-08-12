import React, { FC, useCallback } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";

import ElicitationCard, { ElicitationPayload } from "./ElicitationCard";
import { respondToElicitation } from "../../services/elicitationService";
import useApi from "../../hooks/useApi";

/**
 * Owns the answer mutation for one question card.
 *
 * The card itself stays presentational so it can be rendered from a transcript entry
 * without dragging query-client wiring into the timeline builder.
 */
const ElicitationCardContainer: FC<{
  sessionId: string;
  elicitation: ElicitationPayload;
}> = ({ sessionId, elicitation }) => {
  const queryClient = useQueryClient();
  const api = useApi();

  const mutation = useMutation({
    mutationFn: (variables: {
      elicitationId: string;
      action: "accept" | "decline";
      content: Record<string, unknown>;
    }) =>
      respondToElicitation(
        api.getApiClient(),
        sessionId,
        variables.elicitationId,
        variables.action,
        variables.content,
      ),
    onSuccess: () => {
      // The agent reports the authoritative terminal status back over the session
      // WebSocket; refetching keeps a client that missed the event honest.
      queryClient.invalidateQueries({ queryKey: ["session", sessionId] });
      queryClient.invalidateQueries({ queryKey: ["elicitations", sessionId] });
    },
  });

  const handleRespond = useCallback(
    async (
      elicitationId: string,
      action: "accept" | "decline",
      content: Record<string, unknown>,
    ) => {
      await mutation.mutateAsync({ elicitationId, action, content });
    },
    [mutation],
  );

  return (
    <ElicitationCard
      elicitation={elicitation}
      onRespond={sessionId ? handleRespond : undefined}
    />
  );
};

export default ElicitationCardContainer;
