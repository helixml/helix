import { TypesElicitationRespondResponse } from "../api/api";
import api from "../api/api";

/**
 * Answers a question the agent asked.
 *
 * Uses the generated client (never a hand-rolled fetch). The endpoint returns
 * `submitting` — the authoritative terminal status arrives from the agent over the
 * session WebSocket once it has actually applied the answer.
 *
 * A 409 here is normal and meaningful: the question was already answered, skipped, or
 * cancelled (for example by the turn ending, or by another browser tab). The card surfaces
 * the message rather than retrying.
 */
export async function respondToElicitation(
  sessionId: string,
  elicitationId: string,
  action: "accept" | "decline",
  content: Record<string, unknown>,
): Promise<TypesElicitationRespondResponse> {
  const client = api.getApiClient();
  const response = await client.v1SessionsElicitationsRespondCreate(
    sessionId,
    elicitationId,
    { action, content },
  );
  return response.data;
}

/** Questions on this session that can still be answered. */
export async function listSessionElicitations(sessionId: string) {
  const client = api.getApiClient();
  const response = await client.v1SessionsElicitationsDetail(sessionId);
  return response.data;
}
