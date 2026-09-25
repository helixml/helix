# Bot chat loading flash and layout state

The bot chat route showed "Meet your agent" while it waited for the selected
bot's durable session. On a bot switch, the shared bot list could supply stale
status and session fields before its next fetch. The resolver now queries the
selected bot's detail, waits for that fetch before activation, and shows the
introduction only for navigation immediately after organization creation.
A runtime state read error now fails the bot API request instead of reporting
a stopped bot with no session.

The workspace already stored the expanded split ratio and selected view per
organization. It now stores whether the right panel, chat panel, and terminal
are open. Collapsing the right panel keeps the last expanded ratio, so
reopening it restores the prior width. The outer chat sidebar already stores
its width and collapsed state per organization in `Layout.tsx`.

The initial fix was pushed to PR #3296 from the wrong checkout. The QA stack at
`localhost:8080` runs `feat/org-bot-instances` (PR #3293), so those changes
were not served there. This fix is being applied to PR #3293 and verified
against that stack.

The full-size logo report remains a separate investigation. The React loading
components and bot route do not render a Helix logo. A capture of the large
logo frame is needed to distinguish app rendering from browser reload UI.

Verification on PR #3293: 55 focused frontend tests, TypeScript, frontend
production build, and bot API tests passed. In the browser at
`http://100.108.100.25:8080`, switching Chief of Staff → Dubai Properties
Broker → Chief of Staff opened the existing chat without the introduction.
The closed right panel stayed closed across the switch and a refresh; reopening
it restored the saved 38/62 split.
