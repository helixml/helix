/**
 * The two judgement calls behind the customer-facing chat embed.
 *
 * Both are one-liners at the call site and both are wrong in a way nobody sees
 * until it is in front of a customer — one hides a message that should be
 * visible, the other shows a system prompt that should not be. Pulled out here
 * so they can be tested without mounting a session.
 */

/**
 * Which rendered interaction is the session's opening briefing, or -1 for none.
 *
 * The first turn of an agent session is not something the customer said: it is
 * the prompt that told the agent who it is and what it can do, and it renders
 * as a user message like any other. On an embed it has to go.
 *
 * It is only safe to assume index 0 is that turn once pagination has reached
 * the oldest page. A thread that starts mid-conversation has a real customer
 * message at index 0, and blanking that would be worse than the problem.
 */
export function seedPromptIndex(
  minimal: boolean,
  hasOlderInteractions: boolean,
  hasBriefingTurn = true,
): number {
  if (!minimal) return -1
  // An org bot instance starts with NO briefing: its first interaction is the
  // customer's own first message, and hiding it deletes their question.
  if (!hasBriefingTurn) return -1
  if (hasOlderInteractions) return -1
  return 0
}

/**
 * Should the welcome screen stand in for the thread?
 *
 * One interaction means only the opening briefing exists — the agent was told
 * who it is talking to and replied with a greeting. Nobody has asked it
 * anything, so there is no conversation to show yet.
 *
 * `hasSent` covers the gap between the customer clicking send and the server
 * count catching up on the next poll. Without it the welcome screen flashes
 * back for a beat after they have already spoken.
 *
 * `isLoading` covers the first load and session switches, before the interaction
 * count is known. Until then, zero is only a fallback value, not an empty chat.
 */
export function shouldShowWelcome(
  minimal: boolean,
  hasSent: boolean,
  totalInteractions: number,
  isLoading = false,
  hasBriefingTurn = true,
): boolean {
  if (!minimal) return false
  if (isLoading) return false
  if (hasSent) return false
  // Without a briefing turn, one interaction IS a conversation. Treating it as
  // empty would put the welcome screen over a real exchange on every reload.
  return totalInteractions <= (hasBriefingTurn ? 1 : 0)
}

/**
 * How the composer delivers a message.
 *
 * The welcome screen and the thread render the composer in different parents,
 * so the first send REMOUNTS it. In queued mode that replays the message: the
 * new instance reloads the localStorage queue before the old one has marked
 * the entry sent, and sends it again. Observed on the Find AI embed — one
 * click, two identical turns 22ms apart, running concurrently in one agent.
 *
 * Spec-task embeds never hit this because the backend owns their queue and the
 * client pump is off. A minimal embed without a spec task has no use for an
 * offline queue anyway, so it sends directly.
 */
export function composerSendMode(minimal: boolean, hasBackendQueue: boolean): 'queued' | 'direct' {
  return minimal && !hasBackendQueue ? 'direct' : 'queued'
}
