// slackManifest builds the Slack app manifest for the single,
// deployment-wide Helix Slack app (consumed by SlackAppSetup). Kept as a
// pure module so the manifest contract — the thing Slack validates — is
// unit-testable without rendering the dialog.

// Bot scopes the global app requests. The backend's defaultSlackBotScopes
// (helix_org_slack.go) is authoritative — it's what the OAuth install
// actually requests — so this manifest list must stay a superset of it.
export const BOT_SCOPES = [
  'app_mentions:read',
  'channels:history',
  'channels:read',
  'channels:join',
  'groups:history',
  'groups:read',
  'im:history',
  'im:write',
  'chat:write',
  'chat:write.customize',
  'reactions:write',
  'files:write',
  'users:read',
  'users:read.email',
]

// Each subscribed message.* event requires its matching *:history scope,
// so the event list is derived from BOT_SCOPES rather than maintained by
// hand — adding (or dropping) a *:history scope updates both at once.
const SCOPE_EVENT: Record<string, string> = {
  'app_mentions:read': 'app_mention',
  'channels:history': 'message.channels',
  'groups:history': 'message.groups',
  'im:history': 'message.im',
  'mpim:history': 'message.mpim',
}
export const BOT_EVENTS = BOT_SCOPES.map((s) => SCOPE_EVENT[s]).filter(Boolean)

// buildManifest returns a Slack app manifest pre-filled for this
// deployment. REST delivers events over HTTPS so the manifest MUST declare
// the Events Request URL — Slack rejects bot_events without either a
// request_url or Socket Mode, and verifies the URL at create time via the
// url_verification handshake (which the events endpoint answers before any
// signing secret exists). Socket Mode needs no URL (the socket is the
// channel), so socket_mode_enabled satisfies Slack instead.
export const buildManifest = (mode: 'rest' | 'socket', redirectURL: string, eventsURL: string, appName?: string): string => {
  // Slack caps the app name at 35 chars and the bot display name at 80;
  // fall back to "Helix" when no connection name was given.
  const name = (appName || '').trim().slice(0, 35) || 'Helix'
  const eventSubscriptions: any = { bot_events: BOT_EVENTS }
  if (mode === 'rest') {
    eventSubscriptions.request_url = eventsURL
  }
  const manifest: any = {
    display_information: {
      name,
      description: 'Helix AI — connect your Slack workspace to Helix agents.',
      // The richest branding the manifest can carry. Slack does NOT
      // support an app icon in the manifest (it's uploaded by hand in
      // Basic Information → Display Information after the app is created),
      // so name / description / long_description / background_color are
      // all the Helix identity we can pre-fill.
      long_description:
        'Helix connects this Slack workspace to your Helix AI agents and org-chart Workers. ' +
        'Mention a Worker or post in a connected channel and the right agent picks the message up, ' +
        'reads the surrounding thread, and replies right here in Slack — all backed by your own ' +
        'Helix deployment. Learn more at https://helix.ml.',
      // Helix brand dark — matches the app icon background and the
      // product's own theme (frontend/src/themes.tsx darkBackgroundColor).
      background_color: '#121214',
    },
    features: { bot_user: { display_name: name, always_online: true } },
    oauth_config: { scopes: { bot: BOT_SCOPES } },
    settings: {
      event_subscriptions: eventSubscriptions,
      org_deploy_enabled: false,
      socket_mode_enabled: mode === 'socket',
      token_rotation_enabled: false,
    },
  }
  manifest.oauth_config.redirect_urls = [redirectURL]
  return JSON.stringify(manifest, null, 2)
}
