package sharepoint

import "testing"

func TestParseSiteURL(t *testing.T) {
	valid := []string{
		"https://contoso.sharepoint.com/sites/MySite",
		"https://Contoso.SharePoint.com/sites/MySite/SitePages/Home.aspx",
		"https://contoso-my.sharepoint.com/personal/someone",
		"https://contoso.sharepoint.com:443/teams/eng",
		"https://contoso.sharepoint.com",
	}
	for _, raw := range valid {
		if _, err := ParseSiteURL(raw); err != nil {
			t.Errorf("ParseSiteURL(%q) = %v, want ok", raw, err)
		}
	}

	invalid := []string{
		"",
		"not a url",
		"https://sharepoint.com/sites/x",
		"https://evilsharepoint.com/sites/x",
		"https://contoso.sharepoint.com.evil.example/sites/x",
		"https://contoso.sharepoint.com.evil.example",
		"https://a.b.sharepoint.com/sites/x",
		"https://-contoso.sharepoint.com/sites/x",
		"https://contoso.sharepoint.com./sites/x",
		"https://contoso_x.sharepoint.com/sites/x",
		"http://contoso.sharepoint.com/sites/x",
		"ftp://contoso.sharepoint.com/sites/x",
		"https://user:pass@contoso.sharepoint.com/sites/x",
		"https://contoso.sharepoint.com:8443/sites/x",
		"https://evil.example/?u=contoso.sharepoint.com",
		"https://contoso.sharepoint.com/sites/../../me",
		"//contoso.sharepoint.com/sites/x",
	}
	for _, raw := range invalid {
		if _, err := ParseSiteURL(raw); err == nil {
			t.Errorf("ParseSiteURL(%q) accepted, want rejected", raw)
		}
	}
}
