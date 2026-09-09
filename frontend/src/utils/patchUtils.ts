/**
 * Utility functions for applying patch-based streaming updates.
 * Used by both the main streaming context and design review comment streaming.
 */

/**
 * Applies a patch to content, reconstructing the full string from a delta update.
 * This matches the Go server's computePatch output format.
 *
 * @param currentContent - The current content before applying the patch
 * @param patchOffset - UTF-16 code unit offset where the patch starts
 * @param patch - The new content to insert at patchOffset
 * @param totalLength - Expected total length after applying patch (for truncation)
 * @returns The reconstructed full content
 */
/**
 * Reports whether applying this patch would silently lose content.
 *
 * A delta is only meaningful against the baseline it was computed from. If the
 * client's baseline is SHORTER than the patch's offset, the bytes in between
 * were never received — usually because the socket dropped and the streaming
 * baseline was cleared on reconnect without a catch-up snapshot arriving.
 *
 * applyPatch cannot recover from that: its `patchOffset >= currentContent.length`
 * branch treats the gap as a plain append, so the content silently restarts from
 * the newest fragment and the reader sees only the tail of the response. That was
 * a real, reported bug — a viewer watching someone else's session saw only the
 * last sentence of every reply.
 *
 * Callers should treat `true` as "resync from the database", not as an error.
 */
export function hasPatchGap(currentContent: string, patchOffset: number): boolean {
  return patchOffset > currentContent.length;
}

export function applyPatch(
  currentContent: string,
  patchOffset: number,
  patch: string,
  totalLength: number
): string {
  let newContent: string;

  if (patchOffset === 0 && currentContent.length === 0) {
    // First patch — just use the patch directly
    newContent = patch;
  } else if (patchOffset >= currentContent.length) {
    // Pure append — most common case during streaming
    newContent = currentContent + patch;
  } else {
    // Backwards edit — tool call status change, etc.
    newContent = currentContent.slice(0, patchOffset) + patch;
  }

  // Truncate if totalLength indicates content got shorter
  if (totalLength < newContent.length) {
    newContent = newContent.slice(0, totalLength);
  }

  return newContent;
}


