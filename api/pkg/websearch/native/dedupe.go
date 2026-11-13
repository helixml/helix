package native

import (
	"net/url"
	"sort"
	"strings"
)

// trackingParams are query parameters that identify a referrer, not a page.
var trackingParams = map[string]bool{
	"fbclid": true, "gclid": true, "dclid": true, "msclkid": true, "yclid": true,
	"mc_cid": true, "mc_eid": true, "ref": true, "ref_src": true,
}

// dedupeKey identifies the page behind a normalised URL, so the same page
// reached by http and https, with or without "www.", a trailing slash or
// tracking parameters counts once. The URL itself is kept as the result id.
func dedupeKey(u *url.URL) string {
	host := strings.TrimPrefix(u.Host, "www.")
	path := strings.TrimSuffix(u.EscapedPath(), "/")

	q := u.Query()
	for name := range q {
		if strings.HasPrefix(name, "utm_") || trackingParams[name] {
			q.Del(name)
		}
	}
	keys := make([]string, 0, len(q))
	for k := range q {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	b.WriteString(host)
	b.WriteString(path)
	for i, k := range keys {
		sep := "&"
		if i == 0 {
			sep = "?"
		}
		for _, v := range q[k] {
			b.WriteString(sep + url.QueryEscape(k) + "=" + url.QueryEscape(v))
			sep = "&"
		}
	}
	return b.String()
}
