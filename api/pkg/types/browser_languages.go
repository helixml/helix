package types

import (
	"regexp"
	"strings"
)

// maxBrowserLanguages caps how many languages are passed to the desktop browser.
const maxBrowserLanguages = 10

var browserLanguageTag = regexp.MustCompile(`^[A-Za-z]{2,3}(-[A-Za-z0-9]{1,8}){0,3}$`)

// SanitizeBrowserLanguages turns a comma-separated list of browser languages
// (navigator.languages) into a clean, de-duplicated list of BCP 47 tags.
// Anything that isn't a plain language tag is dropped, so the result is safe
// to put in a container environment variable and a Chrome policy file.
func SanitizeBrowserLanguages(raw string) string {
	var tags []string
	seen := map[string]bool{}
	for _, tag := range strings.Split(raw, ",") {
		tag = strings.TrimSpace(tag)
		// Accept-Language style input may carry quality values ("en;q=0.9")
		if i := strings.IndexByte(tag, ';'); i >= 0 {
			tag = strings.TrimSpace(tag[:i])
		}
		if !browserLanguageTag.MatchString(tag) || seen[strings.ToLower(tag)] {
			continue
		}
		seen[strings.ToLower(tag)] = true
		tags = append(tags, tag)
		if len(tags) == maxBrowserLanguages {
			break
		}
	}
	return strings.Join(tags, ",")
}
