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
// The trigger already carries the current message and routing correlation.
// History APIs are fallbacks for tasks that genuinely need older context.
//
// A top-level DM stays top-level. Existing DM threads and channel messages
// use their thread root so replies remain correlated.
func replyHint(teamID, channel, channelType, ts, threadTS string) string {
	threadRoot := threadTS
	if threadRoot == "" {
		threadRoot = ts
	}
	replyTarget := fmt.Sprintf("channel=%s, thread_ts=%s (reply in this thread)", channel, threadRoot)
	threadContext := fmt.Sprintf(
		"Only when earlier thread context is necessary, call conversations.replies with channel=%s and ts=%s. ",
		channel, threadRoot,
	)
	if channelType == "im" && threadTS == "" {
		replyTarget = fmt.Sprintf("channel=%s; omit thread_ts and reply at the DM root", channel)
		threadContext = ""
	}
	return fmt.Sprintf(
		"This message is from Slack (workspace team %[1]s, channel %[2]s). Mint a bot token: call "+
			"mint_credential with provider=\"slack\" and resource=\"%[1]s\", then use it for all the "+
			"Slack Web API calls below.\n"+
			"- Reply: POST https://slack.com/api/chat.postMessage with %[3]s, username set to your "+
			"own worker name (so people see which worker replied), and icon_url=%[4]s (the Helix avatar).\n"+
			"- Context: use the triggering message and routing details already in this prompt, plus any "+
			"context in your existing conversation. Do not fetch Slack history by default. %[5]sOnly "+
			"when channel-root context is genuinely necessary, call conversations.history with channel=%[2]s.\n"+
			"- Richer responses: reactions.add, files.upload, etc. with the same token and channel.\n"+
			"Do NOT use the publish tool to reply to Slack - it only routes inside Helix.",
		teamID, channel, replyTarget, helixSlackIconURL, threadContext,
	)
}
