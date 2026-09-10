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

/** The parts of a rendered entry that hasLostBaseline needs. */
export interface BaselineEntry {
  content: string;
  message_id?: string;
}

/** The parts of an entry patch that hasLostBaseline needs. */
export interface BaselinePatch {
  index: number;
  patch_offset: number;
  message_id?: string;
}

/**
 * Reports whether the entries we hold are still a usable baseline for this
 * patch message. `true` means resync from the database — NOT an error, and not
 * something more deltas can repair.
 *
 * Three ways a baseline goes bad, and all three have been seen in the wild:
 *
 *   1. A patch whose offset is past the end of what we hold. applyPatch cannot
 *      tell this from an append — its `patchOffset >= currentContent.length`
 *      branch is true for both — so the entry silently restarts mid-sentence.
 *
 *   2. Entries we never received any content for. The server only patches
 *      entries that CHANGED, usually just the one streaming. Growing the array
 *      to entry_count leaves the earlier ones as empty strings, so the reader
 *      sees blanks followed by the last segment. This is what a viewer of
 *      someone else's session actually reported on 9 Sept 2026.
 *
 *   3. Index drift. The accumulator omits empty-content entries, so entry_count
 *      can SHRINK and shift every later index. Our array only ever grows, so a
 *      patch would then land on the wrong entry and corrupt it.
 *
 * Case 2 is the reason an offset check alone is not enough: a lost baseline
 * that coincides with a NEW entry starting has offset 0, which looks perfectly
 * normal. Case 3 is the reason an offset check alone is not SAFE: a shifted
 * index can have a plausible offset and still be the wrong entry.
 *
 * The invariant: after applying a patch message, every entry the server says
 * exists should hold content, under the message_id the server has for it.
 */
export function hasLostBaseline(
  entries: readonly BaselineEntry[],
  entryPatches: readonly BaselinePatch[]
): boolean {
  const patchedNow = new Set(entryPatches.map((ep) => ep.index));

  const misaligned = entryPatches.some((ep) => {
    if (ep.index >= entries.length) return false;
    const entry = entries[ep.index];
    // Both ids known and different — our array no longer corresponds to the
    // server's, so this patch belongs to an entry we are not holding here.
    if (entry.message_id && ep.message_id && entry.message_id !== ep.message_id) {
      return true;
    }
    return hasPatchGap(entry.content, ep.patch_offset);
  });

  const unfilled = entries.some(
    (entry, i) => !patchedNow.has(i) && entry.content === ""
  );

  return misaligned || unfilled;
}

// Tool statuses that mean "still working". Everything else is treated as
// terminal.
//
// This vocabulary is a human-readable label produced by Zed, not an enum we
// control from this repo, so it cannot be exhaustively allowlisted. Hence the
// inverse: only a known in-progress label suppresses the notification, and an
// unrecognised status is treated as terminal. Failing that way round means a
// host might occasionally refetch early, rather than never being told a tool
// finished at all.
const IN_PROGRESS_TOOL_STATUSES = new Set([
  "in progress",
  "in_progress",
  "running",
  "pending",
  "started",
  "waiting",
]);

/**
 * Reports whether a tool_call entry's status means the call has finished
 * (successfully or not), and is therefore worth telling an embedding host about.
 */
export function isTerminalToolStatus(status: string | undefined): boolean {
  if (!status) return false;
  const normalised = status.trim().toLowerCase();
  if (normalised === "") return false;
  return !IN_PROGRESS_TOOL_STATUSES.has(normalised);
}
