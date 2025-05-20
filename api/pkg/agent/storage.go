package agent

// Storage is an interface that abstracts the user storage layer. For agent, a conversation is a pair of
// user messages and assistant messages.
// type Storage interface {
// 	// conversation related
// 	// GetConversations should return the conversations in the order of creating them.
// 	// The first message in the returned list must be older than the second message in the list.
// 	// Be careful on applying limit and offset. If the limit is 10 and offset is 5, it means
// 	// we'll do the offset from the end of the conversation (i.e., skip the last 5 conversations
// 	// in the whole chat history) and then take the 10 messages from that point backwards and
// 	// return a list of those 10 messages arranged in the described order.
// 	GetConversations(meta Meta, limit int, offset int) (*MessageList, error)
// 	// CreateConversation(meta Meta, userMessage string) error
// 	FinishConversation(meta Meta, assistantMessage string) error
// }
