package slack

import "fmt"

// replyHint is the transport-authored guidance the ingest stamps onto
// every inbound Message (Message.ReplyHint). It is rendered into the
// recipient Worker's activation prompt and tells the agent how to respond
// through Slack: mint a workspace-scoped bot token and drive the Slack Web
// API directly (Slack has no outbound emitter — egress is the agent's
// job). The concrete coordinates are baked in so a Worker reached via a
// processor needs nothing else; nothing about Slack lives in the Worker's
// Role.
func replyHint(teamID, channel, ts string) string {
	return fmt.Sprintf(
		"This message is from Slack (workspace team %s, channel %s). "+
			"To respond, get a bot token from the mint_credential tool with "+
			"provider=\"slack\" and resource=\"%s\", then drive the Slack Web "+
			"API as the bot: POST https://slack.com/api/chat.postMessage with "+
			"channel=%s and thread_ts=%s to reply in this thread (omit thread_ts "+
			"to post at the channel root). Use the same token and channel for "+
			"richer responses (reactions.add, files.upload, …). Do NOT use the "+
			"publish tool to reply to Slack — that only routes inside Helix.",
		teamID, channel, teamID, channel, ts,
	)
}
