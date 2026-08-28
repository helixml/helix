package slack

import "fmt"

// replyHint is the transport-authored guidance the ingest stamps onto
// every inbound Message (Message.ReplyHint). It is rendered into the
// recipient Worker's activation prompt and tells the agent how to act on
// the message through Slack. The concrete coordinates are baked in so a
// Worker reached via a processor needs nothing else; nothing about Slack
// lives in the Worker's Role.
//
// There is exactly one way out: a Trigger is inbound-only, so replying
// means fetching the granted Slack token with get_secret and calling
// Slack's own API. That is deliberate — the org does not proxy provider
// actions, and having one route means the agent has no wrong choice to
// make.
//
// The trigger already carries the current message and routing
// correlation. History APIs are fallbacks for tasks that genuinely need
// older context.
//
// A top-level DM stays top-level. Existing DM threads and channel
// messages use their thread root so replies remain correlated.
func replyHint(triggerID string, exact bool, teamID, channel, channelType, ts, threadTS string) string {
	threadRoot := threadTS
	if threadRoot == "" {
		threadRoot = ts
	}
	replyTarget := fmt.Sprintf("thread_ts=%s (reply in this thread)", threadRoot)
	threadContext := fmt.Sprintf(
		"Only when earlier thread context is necessary, call conversations.replies with channel=%s and ts=%s. ",
		channel, threadRoot,
	)
	if channelType == "im" && threadTS == "" {
		replyTarget = "omit thread_ts and reply at the DM root"
		threadContext = ""
	}
	scope := fmt.Sprintf("workspace-wide ingress Trigger %s", triggerID)
	if exact {
		scope = fmt.Sprintf("channel-bound ingress Trigger %s", triggerID)
	}
	return fmt.Sprintf(
		"This message is from Slack (workspace team %[1]s, channel %[2]s), delivered by the %[3]s.\n"+
			"- To reply: call get_secret for the granted Slack token, then call Slack's chat.postMessage "+
			"with channel=%[2]s and %[5]s. The Trigger is inbound-only — there is no Helix tool that "+
			"posts to Slack for you.\n"+
			"- Context: use the triggering message and routing details already in this prompt, plus any "+
			"context in your existing conversation. Do not fetch Slack history by default. %[4]sOnly "+
			"when channel-root context is genuinely necessary, call conversations.history with channel=%[2]s.\n"+
			"- Rich actions or a different channel: the same token covers reactions.add, files.upload, "+
			"edits, and lookups. Match the binding's non-secret workspace metadata to team_id %[1]s.",
		teamID, channel, scope, threadContext, replyTarget,
	)
}
