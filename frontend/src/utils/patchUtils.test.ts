import { describe, it, expect } from 'vitest'
import {
  applyPatch,
  hasLostBaseline,
  hasPatchGap,
  isTerminalToolStatus,
} from './patchUtils'

describe('applyPatch', () => {
  it('takes the patch directly as the first chunk', () => {
    expect(applyPatch('', 0, 'Hello', 5)).toBe('Hello')
  })

  it('appends during normal streaming', () => {
    expect(applyPatch('Hello', 5, ' world', 11)).toBe('Hello world')
  })

  it('rewrites from the offset on a backwards edit', () => {
    expect(applyPatch('Hello world', 5, ' there', 11)).toBe('Hello there')
  })

  it('truncates when the content got shorter', () => {
    expect(applyPatch('Hello world', 5, ' you', 9)).toBe('Hello you')
  })
})

describe('hasPatchGap', () => {
  // The bug this exists to catch. A viewer watching someone else's session saw
  // only the LAST SENTENCE of each reply. The socket had dropped, the streaming
  // baseline was cleared on reconnect, and no catch-up snapshot arrived — so the
  // client held "" while the server was hundreds of characters in.
  //
  // applyPatch cannot tell that apart from an append: its
  // `patchOffset >= currentContent.length` branch is true for BOTH. It happily
  // concatenates onto nothing and the reply silently restarts mid-sentence.
  it('detects a baseline that is missing content', () => {
    expect(hasPatchGap('', 500)).toBe(true)
  })

  it('demonstrates what applyPatch does with that gap, which is why we check first', () => {
    // 500 >= 0, so this is treated as a plain append onto an empty string.
    // The first 500 characters are gone and nothing reports it.
    expect(applyPatch('', 500, 'and finally, in summary.', 524))
      .toBe('and finally, in summary.')
  })

  it('does not flag an ordinary append', () => {
    expect(hasPatchGap('Hello', 5)).toBe(false)
  })

  it('does not flag a backwards edit', () => {
    expect(hasPatchGap('Hello world', 5)).toBe(false)
  })

  it('does not flag the very first chunk', () => {
    expect(hasPatchGap('', 0)).toBe(false)
  })
})

// The shape of the real failure, documented as a unit so the reasoning survives.
//
// A delta stream is RELATIVE: "at offset N, add this" only means anything if you
// already hold the first N characters. That baseline is established either by
// being connected when the turn started (always true for whoever submitted the
// message) or by the server's catch-up snapshot (the only route available to
// someone watching a session they did not start).
//
// When neither happens, the client holds nothing and the deltas keep arriving.
describe('a lost baseline, as a viewer of someone else’s session sees it', () => {
  it('renders the tail as though it were the whole reply, with no error', () => {
    // The server is 300 characters into the reply. We hold nothing.
    const held = ''
    const delta = 'and that is why the migration finished early.'
    const rendered = applyPatch(held, 300, delta, 345)

    // No throw, no warning — just a reply that silently begins mid-thought.
    expect(rendered).toBe(delta)
    expect(rendered.length).toBeLessThan(345)

    // Which is precisely what the guard is for.
    expect(hasPatchGap(held, 300)).toBe(true)
  })

  it('is invisible to an offset check when the lost baseline meets a NEW entry', () => {
    // A brand-new entry legitimately starts at offset 0, so the offset tells us
    // nothing. The evidence is elsewhere: the EARLIER entries were never filled.
    // This is the case that made the reply render as blanks plus a final segment.
    expect(hasPatchGap('', 0)).toBe(false)

    const entriesAfterGrowingFromNothing = [
      { content: '' },   // never received — the server only patches what changed
      { content: '' },   // never received
      { content: 'the last segment, which is all anyone saw' },
    ]
    expect(
      hasLostBaseline(entriesAfterGrowingFromNothing, [
        { index: 2, patch_offset: 0 },
      ]),
    ).toBe(true)
  })
})

describe('hasLostBaseline', () => {
  it('passes ordinary streaming: one entry growing, the rest already filled', () => {
    const entries = [
      { content: 'I looked at the repository.' },
      { content: 'Reading transform.go' },
      { content: 'The advert fields are' },
    ]
    expect(
      hasLostBaseline(entries, [{ index: 2, patch_offset: 21 }]),
    ).toBe(false)
  })

  it('passes the very first patch of the very first entry', () => {
    // Empty, but it is the entry being patched right now — that is a start,
    // not a hole.
    expect(hasLostBaseline([{ content: '' }], [{ index: 0, patch_offset: 0 }]))
      .toBe(false)
  })

  it('flags a patch reaching past the end of what we hold', () => {
    expect(
      hasLostBaseline([{ content: 'Hello' }], [{ index: 0, patch_offset: 500 }]),
    ).toBe(true)
  })

  it('flags an entry the server says exists that we never received', () => {
    // Entry 0 is not in this patch message and has no content: the server only
    // patches what changed, so we should already be holding it. We are not.
    expect(
      hasLostBaseline(
        [{ content: '' }, { content: 'second' }],
        [{ index: 1, patch_offset: 0 }],
      ),
    ).toBe(true)
  })

  it('flags index drift when a patch lands on a different message', () => {
    // The accumulator omits empty-content entries, so entry_count can SHRINK
    // and shift every later index. Our array only grows, so without this check
    // the patch would overwrite the wrong entry with a plausible-looking offset.
    expect(
      hasLostBaseline(
        [{ content: 'first', message_id: 'msg-a' }],
        [{ index: 0, patch_offset: 5, message_id: 'msg-b' }],
      ),
    ).toBe(true)
  })

  it('does not cry drift when a message_id is not yet known on either side', () => {
    // Freshly grown entries carry an empty message_id until their first patch.
    expect(
      hasLostBaseline(
        [{ content: 'first', message_id: '' }],
        [{ index: 0, patch_offset: 5, message_id: 'msg-a' }],
      ),
    ).toBe(false)
  })

  it('ignores a patch for an entry beyond the array', () => {
    // The caller grows the array to entry_count first, so this only happens
    // when a patch names an index the server did not count. Not our failure.
    expect(hasLostBaseline([{ content: 'a' }], [{ index: 7, patch_offset: 0 }]))
      .toBe(false)
  })
})

describe('isTerminalToolStatus', () => {
  // The status is a human-readable label from Zed, not an enum this repo
  // controls, so the check is an in-progress denylist rather than a terminal
  // allowlist — see the note in patchUtils.ts.
  it('treats the known in-progress labels as not finished', () => {
    for (const status of ['In Progress', 'Running', 'Pending', 'Started']) {
      expect(isTerminalToolStatus(status)).toBe(false)
    }
  })

  it('is case and whitespace insensitive, because the label is prose', () => {
    expect(isTerminalToolStatus('  in progress  ')).toBe(false)
    expect(isTerminalToolStatus('RUNNING')).toBe(false)
  })

  it('treats finished labels as finished, however they are spelled', () => {
    for (const status of ['Completed', 'completed', 'failed', 'Error']) {
      expect(isTerminalToolStatus(status)).toBe(true)
    }
  })

  it('treats an unrecognised label as finished rather than swallowing it', () => {
    // Failing this way round means a host might refetch early. Failing the
    // other way means it is never told the tool finished at all.
    expect(isTerminalToolStatus('Finished with warnings')).toBe(true)
  })

  it('says nothing at all about a missing or empty status', () => {
    expect(isTerminalToolStatus(undefined)).toBe(false)
    expect(isTerminalToolStatus('')).toBe(false)
    expect(isTerminalToolStatus('   ')).toBe(false)
  })
})
