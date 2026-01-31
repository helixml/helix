package openai

import "strings"

var openaiTextModels = map[string]int{
	"gpt-3.5-turbo":               4096,
	"gpt-3.5-turbo-16k":           16384,
	"gpt-3.5-turbo-1106":          16384,
	"gpt-3.5-turbo-0125":          16384,
	"gpt-3.5-turbo-instruct":      4096,
	"gpt-3.5-turbo-instruct-0914": 4096,
	"gpt-4":                       8192,
	"gpt-4-0613":                  8192,
	"gpt-4-1106-preview":          128000,
	"gpt-4-0125-preview":          128000,
	"gpt-4-turbo":                 128000,
	"gpt-4-turbo-preview":         128000,
	"gpt-4o":                      128000,
	"gpt-4o-mini":                 128000,
	"chatgpt-4o-latest":           128000,
	"gpt-4.5-preview":             128000,
	"o1":                          128000,
	"o1-2024-12-17":               128000,
	"o1-preview":                  128000,
	"o1-mini":                     128000,
	"o1-pro":                      128000,
	"o3-mini":                     200000,
	"o3":                          200000,
	"o4-mini":                     200000,
	"gpt-4o-mini-search-preview":  128000,
	"gpt-4o-search-preview":       128000,
}

func getOpenAIModelContextLength(model string) int {
	length, ok := openaiTextModels[model]
	if ok {
		return length
	}

	// Otherwise look for a model with the same prefix, this is to cope with
	// the fact that OpenAI has a lot of models that are similar but not
	// exactly the same. For example o3-mini-2025-01-31 has the same context
	// length as o3-mini.
	for prefix, length := range openaiTextModels {
		if strings.HasPrefix(model, prefix) {
			return length
		}
	}

	return 0
}
