package tools

import (
	"encoding/json"
	"strings"
)

func AttemptFixJSON(data string) string {
	// sometimes LLM just gives us a single ``` line at the start; just strip that off
	if strings.HasPrefix(data, "```\n") {
		data = strings.Split(data, "```\n")[1]
	}

	if strings.Contains(data, "```json") {
		data = strings.Split(data, "```json")[1]
	}
	// sometimes LLMs in their wisdom puts a message after the enclosing ```json``` block
	parts := strings.Split(data, "```")
	data = parts[0]

	// LLMs are sometimes bad at correct JSON escaping, trying to escape
	// characters like _ that don't need to be escaped. Just remove all
	// backslashes for now...
	data = strings.Replace(data, "\\", "", -1)

	return data
}

func unmarshalJSON(data string, v interface{}) error {
	fixedData := AttemptFixJSON(data)
	return json.Unmarshal([]byte(fixedData), v)
}
