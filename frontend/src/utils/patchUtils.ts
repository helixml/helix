/**
 * Utility functions for applying patch-based streaming updates.
 * Used by both the main streaming context and design review comment streaming.
 */

/**
 * Applies a patch to content, reconstructing the full string from a delta update.
 * This matches the Go server's computePatch output format.
 *
 * The server computes each patch against its own accumulator, so a correct patch always
 * satisfies `newContent === currentContent.slice(0, patchOffset) + patch`. That single
 * expression covers every case the server emits: a first patch (offset 0 on empty
 * content), a pure append (offset === length), and a backwards edit such as a tool-call
 * status change (offset < length).
 *
 * The transport is best-effort core NATS — drop-on-slow-consumer, no redelivery — so a
 * patch can go missing. When it does, the next patch's offset is beyond what we hold and
 * the reconstruction is impossible. This function returns `null` in that case rather than
 * silently splicing the two ends together: appending across the hole produced permanently
 * corrupt text (verified live — 131 characters vanished mid-word, no error anywhere).
 * `totalLength` is the server's length of the reconstructed content and is used as a
 * checksum; a mismatch also means we have diverged.
 *
 * A `null` return means "resync from a server snapshot", never "keep going".
 *
 * @param currentContent - The current content before applying the patch
 * @param patchOffset - UTF-16 code unit offset where the patch starts
 * @param patch - The new content to insert at patchOffset
 * @param totalLength - Expected total length after applying the patch (checksum)
 * @returns The reconstructed full content, or null if the patch cannot be applied
 */
export function applyPatch(
  currentContent: string,
  patchOffset: number,
  patch: string,
  totalLength: number
): string | null {
  // The Go side marshals these with omitempty, so a zero value arrives as undefined.
  const offset = patchOffset || 0;
  const delta = patch || "";
  const expectedLength = totalLength || 0;

  // A gap: the server patched from an offset past everything we hold, so the bytes
  // between are lost and cannot be reconstructed from this event.
  if (offset > currentContent.length) {
    return null;
  }

  const newContent = currentContent.slice(0, offset) + delta;

  if (expectedLength !== newContent.length) {
    return null;
  }

  return newContent;
}
