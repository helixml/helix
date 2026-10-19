package server

import (
	"net/url"
	"strconv"
)

// queryIntAtLeast parses an integer query parameter, returning def when the
// parameter is missing, malformed, or below min.
func queryIntAtLeast(values url.Values, key string, min, def int) int {
	n, err := strconv.Atoi(values.Get(key))
	if err != nil || n < min {
		return def
	}
	return n
}
