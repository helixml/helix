# Requirements: Disable Zed Pro Advertisement on Startup

## User Story

As a Helix user opening Zed for the first time, I should not see an advertisement for Zed Pro, because Helix manages AI configuration centrally and the upsell is irrelevant and confusing.

## Acceptance Criteria

- [ ] On first launch of Zed (fresh install, no KVP store entry for `dismissed-trial-upsell`), the "Welcome to Zed AI" / Zed Pro upsell panel does **not** appear in the agent panel
- [ ] The upsell does not appear on any subsequent launch either
- [ ] This suppression applies only to Helix builds (`external_websocket_sync` feature enabled); upstream Zed is unaffected
- [ ] No other UI behaviour is changed
