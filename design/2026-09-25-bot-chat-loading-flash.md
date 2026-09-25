# Bot chat loading flash

The `/orgs/:org_id/chat/bots/:bot_id` route rendered its "Meet your agent"
screen whenever the durable session was not yet in local React state. That
included the initial bot-list request and the render before the effect copied
an existing `session_id` into `readySessionID`. Slow requests made the flash
visible. The resolver also attempted activation for a running bot whose list
entry had not yet supplied a session ID.

The route now uses a neutral chat-opening indicator until the selected bot is
loaded. It shows the introduction only after the bot list confirms that this
bot has no session and is not running or starting. Session readiness and
activation errors are keyed to the selected bot, so switching bots cannot
carry over the previous bot's state or polling delay.

Follow-up after the flash was still visible on bot switches: the shared bot
list can return cached rows while it refreshes, so its old `session_id` and
status are not reliable for routing. The resolver now reads the selected bot's
detail query and waits for its current fetch before activation. The intro is
reserved for navigation immediately after organization creation. The bot API
also returns an error when the runtime state lookup fails; a read failure must
not be presented as a stopped bot with no session.

The bot chat workspace already saved the expanded split ratio and selected
view per organization. It now saves whether the right panel, chat panel, and
terminal are open. A collapsed layout does not overwrite the last expanded
ratio, so reopening the right panel restores its prior width. The outer chat
sidebar's width and collapsed state were already saved per organization in
`Layout.tsx`.

The full-size logo report is separate. The React loading components and the
bot route do not render a Helix logo. The image asset is 900×703 pixels, while
the direct in-app logo usages found so far have constrained display sizes.
The PWA manifest uses a separate 180×180 favicon. A capture of the large-logo
frame is needed to distinguish an app image layout issue from browser-owned
PWA or reload UI.

Verification: 13 resolver tests, TypeScript, and the production frontend
build pass. Live browser verification of this checkout is pending: the stack
at `localhost:8080`
mounts `/home/karolis/go/src/github.com/helixml/helix-standard-webhooks-main`,
not this checkout. A browser run against that stack reproduced the old dialog
and confirmed its served `OrgBotSessionResolver.tsx` lacks this change.

Follow-up verification: focused resolver, workspace, onboarding, and new-org
tests passed; TypeScript and the bot API tests passed. The local stack still
serves the other checkout, so it cannot validate this branch in the browser.
