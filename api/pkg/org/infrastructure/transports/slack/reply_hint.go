package slack

import "fmt"

// helixSlackIconURL is a public Helix logo used as the bot avatar on
// outbound messages (chat.postMessage icon_url). It is a public URL —
// Slack's servers fetch it — so it renders even when this deployment is
// only reachable on localhost. Requires the chat:write.customize scope.
//
// It must be a DIRECT image link, not a redirect: Slack's icon fetcher
// does not follow redirects, so github.com/helixml.png (a 302 to the CDN)
// silently fails and falls back to the app's default icon. This is the
// helixml GitHub org avatar's direct CDN URL.
const helixSlackIconURL = "https://avatars.githubusercontent.com/u/149581110?v=4"

// replyHint is the transport-authored guidance the ingest stamps onto
// every inbound Message (Message.ReplyHint). It is rendered into the
// recipient Worker's activation prompt and tells the agent how to act on
// the message through Slack: mint a workspace-scoped bot token and drive
// the Slack Web API directly (Slack has no outbound emitter — egress is
// the agent's job). The concrete coordinates are baked in so a Worker
// reached via a processor needs nothing else; nothing about Slack lives in
// the Worker's Role.
//
// A Worker is activated with ONLY the triggering message — not the
// surrounding thread/channel. Rather than pre-loading a fixed history
// window into every prompt, the hint tells the Worker how to read the rest
// on demand (conversations.replies / conversations.history), so it pulls
// exactly the context a task needs.
//
// threadTS is the thread root when the message is already in a thread,
// empty otherwise; threadRoot (threadTS or the message ts) is what both a
// threaded reply and a thread read key off, so replies land in the
// existing thread rather than starting a sub-thread under one reply.
func replyHint(teamID, channel, ts, threadTS string) string {
	threadRoot := threadTS
	if threadRoot == "" {
		threadRoot = ts
	}
	return fmt.Sprintf(
		"This message is from Slack (workspace team %[1]s, channel %[2]s). Mint a bot token: call "+
			"mint_credential with provider=\"slack\" and resource=\"%[1]s\", then use it for all the "+
			"Slack Web API calls below.\n"+
			"• Reply: POST https://slack.com/api/chat.postMessage with channel=%[2]s, thread_ts=%[3]s "+
			"(reply in this thread; omit thread_ts to post at the channel root), username set to your "+
			"own worker name (so people see which worker replied), and icon_url=%[4]s (the Helix avatar).\n"+
			"• Read earlier messages: you are given ONLY this one message. To see the conversation so "+
			"far, call conversations.replies with channel=%[2]s and ts=%[3]s (this thread), or "+
			"conversations.history with channel=%[2]s (recent channel messages) — same token.\n"+
			"• Richer responses: reactions.add, files.upload, etc. with the same token and channel.\n"+
			"Do NOT use the publish tool to reply to Slack — it only routes inside Helix.",
		teamID, channel, threadRoot, helixSlackIconURL,
	)
}
