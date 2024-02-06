package tools

import (
	"encoding/json"
	"strings"
)

func unmarshalJSON(data string, v interface{}) error {
	// LLMs are sometimes bad at correct JSON escaping, trying to escape
	// characters like _ that don't need to be escaped. Just remove all
	// backslashes for now...
	data = strings.Replace(data, "\\", "", -1)

	return json.Unmarshal([]byte(data), v)
}
