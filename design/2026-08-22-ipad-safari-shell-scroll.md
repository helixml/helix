# iPad Safari shell scroll investigation

## Symptom

After focusing the new-chat composer, iPad Safari could scroll the document.
The app shell then moved down with the visual viewport, leaving a blank strip
above the sidebar header and producing visible jitter.

## Cause

Commit `fa13a7d15523058e4a49b62e969cf40e7e6a3326` compensated for iPhone
Safari viewport panning by translating the full-height `#root` and MUI portal
roots by `visualViewport.offsetTop`. The workaround was global, so it also ran
on iPad.

A transform extends scrollable overflow to include both the original and
translated bounds. Once iPad reported a positive visual viewport offset, the
translated full-height shell extended below the document. That created the
page scrollbar; scrolling changed `offsetTop`, which changed the transform,
forming the visible feedback loop.

This is the vertical equivalent of the notifications-panel overflow fixed in
`d6236cfbb7856d3a86372247aa36a1a22ac3a98b`.

## Physical iPad follow-up

Restricting both fixed positioning and translation to iPhone removed the
transform overflow, but it did not fully anchor the iPad shell. A physical iPad
then reported `scrollHeight/clientHeight=900/900` while its document was still
at `scrollTop=71`. That is viewport panning, not ordinary content overflow. The
static root moved with the document and took the sidebar header off screen.

## Fix

The root shell remains fixed on both iPhone and iPad, so WebKit panning the
document cannot move the app. Only Apple touch devices with a phone-sized
screen receive the `visualViewport.offsetTop` transform. iPad therefore has a
fixed but untransformed root whose height still follows the visual viewport for
the software keyboard. Portalled drawers and dialogs likewise resolve to
`transform: none` outside the iPhone path.

## Verification

- Physical iPad screenshot before the final anchoring change: document
  `900/900` at `scrollTop=71`, proving WebKit can pan a zero-range document.
- iPad Pro 13-inch simulator, iOS 26.5, Safari: mode `standard`, root
  `position: fixed` / `transform: none` at top `0`, document `900/900`, scroll
  `0,0`.
- iPhone 17 Pro simulator, iOS 26.5, Safari: mode `ios-phone`, root
  `position: fixed` with the viewport transform, document `714/714`, scroll
  `0,0`.
- T3 browser at the iPad Pro landscape viewport (`1366x1024`): focused the
  composer and attempted a 1200px page scroll. The document remained
  `1024/1024` at `scrollY=0`; root remained fixed at top `0` with no transform.
- Synthetic iPad `visualViewport.offsetTop=100`: no viewport transform was
  installed, root top remained `0`, and document height remained `1024/1024`.
- iPad MUI drawer and dialog portal roots both computed to `transform: none`.
- `cd frontend && yarn build` completed successfully.
