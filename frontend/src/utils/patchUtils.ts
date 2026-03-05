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

/**
 * Manages patch-based content accumulation for streaming responses.
 * Maintains internal state and provides the reconstructed content.
 */
export class PatchAccumulator {
  private content: string = "";

  /**
   * Apply a patch and return the new content.
   */
  apply(patchOffset: number, patch: string, totalLength: number): string {
    this.content = applyPatch(this.content, patchOffset, patch, totalLength);
    return this.content;
  }

  /**
   * Get the current accumulated content.
   */
  getContent(): string {
    return this.content;
  }

  /**
   * Set content directly (e.g., from a full interaction update).
   */
  setContent(content: string): void {
    this.content = content;
  }

  /**
   * Reset the accumulator for a new streaming session.
   */
  reset(): void {
    this.content = "";
  }
}
