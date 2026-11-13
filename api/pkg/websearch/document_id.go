package websearch

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"strings"
)

// NormalizeURL canonicalises a URL for caching and id derivation: lower-case
// scheme and host, no fragment, no default port, "/" for an empty path.
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", err
	}
	u.Scheme = strings.ToLower(u.Scheme)
	host := strings.ToLower(u.Hostname())
	port := u.Port()
	if (u.Scheme == "http" && port == "80") || (u.Scheme == "https" && port == "443") {
		port = ""
	}
	if port != "" {
		host += ":" + port
	}
	u.Host = host
	u.Fragment = ""
	u.RawFragment = ""
	if u.Path == "" {
		u.Path = "/"
	}
	return u.String(), nil
}

// DocumentID is the id returned in results for a normalised URL, accepted
// back by /contents.
func DocumentID(normalizedURL string) string {
	sum := sha256.Sum256([]byte(normalizedURL))
	return "doc_" + hex.EncodeToString(sum[:16])
}
