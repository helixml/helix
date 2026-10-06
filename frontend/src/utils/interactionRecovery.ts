import { TypesInteraction, TypesInteractionState } from "../api/api";

/**
 * Index of the last interaction that finished successfully, or -1.
 *
 * An errored interaction that sits BEFORE this index has been overtaken: the
 * session went on to do useful work afterwards, so the failure is history. It
 * must not keep an alarm and a Retry button on screen — clicking Retry would
 * re-dispatch a stale prompt into a session that has already moved past it,
 * and the alarm makes a recovered session look broken.
 *
 * Computed once per interaction list rather than per row, so rendering stays
 * linear.
 */
export function lastSuccessfulInteractionIndex(
  interactions: TypesInteraction[] | undefined,
): number {
  if (!interactions) return -1;
  for (let i = interactions.length - 1; i >= 0; i--) {
    const interaction = interactions[i];
    if (
      interaction?.state === TypesInteractionState.InteractionStateComplete &&
      !interaction.error
    ) {
      return i;
    }
  }
  return -1;
}
