package slack

import "fmt"

// replyHint is the transport-authored guidance the ingest stamps onto
// every inbound Message (Message.ReplyHint). It is rendered into the
// recipient Worker's activation prompt and tells the agent how to act on
// the message through Slack: publish basic text through a Topic configured
// for the same channel, or mint a workspace-scoped token for rich actions
// and other channels. The concrete coordinates are baked in so a Worker
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
	replyTarget := fmt.Sprintf("threadId=%s (reply in this thread)", threadRoot)
	threadContext := fmt.Sprintf(
		"Only when earlier thread context is necessary, call conversations.replies with channel=%s and ts=%s. ",
		channel, threadRoot,
	)
	if channelType == "im" && threadTS == "" {
		replyTarget = "omit threadId and reply at the DM root"
		threadContext = ""
	}
	return fmt.Sprintf(
		"This message is from Slack (workspace team %[1]s, channel %[2]s).\n"+
			"- Basic text reply: if this Topic's configured channel_id is %[2]s, call publish on this Topic with %[3]s. A delivered receipt confirms Slack accepted it.\n"+
			"- Context: use the triggering message and routing details already in this prompt, plus any "+
			"context in your existing conversation. Do not fetch Slack history by default. %[4]sOnly "+
			"when channel-root context is genuinely necessary, call conversations.history with channel=%[2]s.\n"+
			"- Rich actions or a different/unconfigured channel: call mint_credential with provider=\"slack\" and resource=\"%[1]s\", then use the Slack API directly for chat.postMessage, reactions.add, files.upload, edits, or lookups.",
		teamID, channel, replyTarget, threadContext,
	)
}
