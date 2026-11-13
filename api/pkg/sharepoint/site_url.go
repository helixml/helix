package sharepoint

import (
	"errors"
	"net/url"
	"strings"
)

// siteHostDomain is the domain every SharePoint Online tenant host lives under
// (contoso.sharepoint.com, contoso-my.sharepoint.com).
const siteHostDomain = "sharepoint.com"

var errInvalidSiteURL = errors.New("invalid SharePoint URL: must be an https://<tenant>.sharepoint.com URL (e.g., https://contoso.sharepoint.com/sites/MySite)")

// ParseSiteURL parses a SharePoint Online site URL and checks that its host is
// a tenant host directly under sharepoint.com. The host is compared on label
// boundaries, so look-alikes such as evilsharepoint.com or
// contoso.sharepoint.com.example.net are refused, as are embedded credentials,
// non-default ports, trailing-dot hosts and dot-segment paths.
func ParseSiteURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, errInvalidSiteURL
	}
	if !strings.EqualFold(u.Scheme, "https") || u.User != nil || u.Opaque != "" {
		return nil, errInvalidSiteURL
	}
	if port := u.Port(); port != "" && port != "443" {
		return nil, errInvalidSiteURL
	}
	if !isTenantHost(strings.ToLower(u.Hostname())) {
		return nil, errInvalidSiteURL
	}
	for _, segment := range strings.Split(u.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, errInvalidSiteURL
		}
	}
	return u, nil
}

// ValidateSiteURL reports whether raw is an acceptable SharePoint Online site URL.
func ValidateSiteURL(raw string) error {
	_, err := ParseSiteURL(raw)
	return err
}

// isTenantHost reports whether host is exactly one DNS label followed by
// ".sharepoint.com".
func isTenantHost(host string) bool {
	tenant, ok := strings.CutSuffix(host, "."+siteHostDomain)
	if !ok || tenant == "" || len(tenant) > 63 {
		return false
	}
	if tenant[0] == '-' || tenant[len(tenant)-1] == '-' {
		return false
	}
	for _, ch := range tenant {
		if (ch < 'a' || ch > 'z') && (ch < '0' || ch > '9') && ch != '-' {
			return false
		}
	}
	return true
}
