package llm

import "strings"

/*
StripThinkingTags removes the <think>...</think> tags from the response to deal with chain-of-thought responses from the LLMs.

```
<think>
Okay, so I need to come up with a concise title for the user's question,
</think>

AI's Creator Origin
```

The function will return "AI's Creator Origin"
*/
func StripThinkingTags(response string) string {
	// First, validate whether we have both start and end tags, if not, just return the response
	if !strings.Contains(response, "<think>") || !strings.Contains(response, "</think>") {
		return response
	}

	// Find the index of the first occurrence of "<think>...</think>"
	start := strings.Index(response, "<think>")
	if start == -1 {
		return response
	}

	// Find the index of the first occurrence of "</think>"
	end := strings.Index(response, "</think>")
	if end == -1 {
		return response
	}

	// Return the response without the thinking tags
	return strings.TrimSpace(response[end+len("</think>"):])
}
