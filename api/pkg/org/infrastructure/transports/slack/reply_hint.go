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
func replyHint(topicID string, exact bool, teamID, channel, channelType, ts, threadTS string) string {
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
	basicReply := fmt.Sprintf(
		"- Basic text reply: this workspace-wide ingress Topic %s is inbound-only. Call list_topics, find this ingress Topic by ID, and identify its service_connection_id. If publish is available, publish to a configured Slack Topic whose service_connection_id matches that ingress Topic and whose channel_id is %s with %s. If publish is unavailable or no matching Topic exists, call mint_credential and use chat.postMessage.",
		topicID, channel, replyTarget,
	)
	if exact {
		basicReply = fmt.Sprintf(
			"- Basic text reply: if publish is available, call publish on Topic %s with %s. A delivered receipt confirms Slack accepted it. Otherwise call mint_credential and use chat.postMessage.",
			topicID, replyTarget,
		)
	}
	return fmt.Sprintf(
		"This message is from Slack (workspace team %[1]s, channel %[2]s).\n"+
			"%[3]s\n"+
			"- Context: use the triggering message and routing details already in this prompt, plus any "+
			"context in your existing conversation. Do not fetch Slack history by default. %[4]sOnly "+
			"when channel-root context is genuinely necessary, call conversations.history with channel=%[2]s.\n"+
			"- Rich actions or a different/unconfigured channel: call mint_credential with provider=\"slack\" and resource=\"%[1]s\", then use the Slack API directly for chat.postMessage, reactions.add, files.upload, edits, or lookups.",
		teamID, channel, basicReply, threadContext,
	)
}
