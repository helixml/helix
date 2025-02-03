package llm

import (
	"testing"

	"gotest.tools/v3/assert"
)

func TestStripThinkingTags(t *testing.T) {
	var resp = `<think>
Okay, so I need to come up with a concise title for the user's question, "who made you?" The rules say it should be exactly 3-5 words and capture the essence of the query. Let me think about this.

First, the question is asking about the creator or the origin of the AI. So, the main keywords here are "creator" and "origin." Maybe I can combine these into a short phrase. 

Looking at the examples provided, like "Roman Empire's formation" and "Perfect steak cooking techniques," they both use possessive forms and are concise. So, perhaps "AI Creator Origin" would work, but that's a bit choppy. Maybe "AI's Creator Origin" sounds better because it uses the possessive form, making it clearer.

Wait, does that fit the 3-5 word rule? "AI's Creator Origin" is three words, so that's within the limit. It also captures the essence of the question, which is about who made the AI. 

I think that's a good fit. It's concise, uses the right structure, and clearly conveys the topic. I don't see any need for additional words, and it follows the examples given. So, I'll go with that.
</think>

AI's Creator Origin`

	stripped := StripThinkingTags(resp)
	assert.Equal(t, "AI's Creator Origin", stripped)
}

func TestStripThinkingTagsNoTags(t *testing.T) {
	var resp = "AI's Creator Origin"

	stripped := StripThinkingTags(resp)
	assert.Equal(t, "AI's Creator Origin", stripped)
}

func TestNoStartTag(t *testing.T) {
	var resp = "</think>AI's Creator Origin"

	stripped := StripThinkingTags(resp)
	assert.Equal(t, resp, stripped, "should not try to strip tags if there is no start tag")
}

func TestNoEndTag(t *testing.T) {
	var resp = "<think>AI's Creator Origin"

	stripped := StripThinkingTags(resp)
	assert.Equal(t, resp, stripped, "should not try to strip tags if there is no end tag")
}
