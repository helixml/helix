package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/proto"
)

// StoredCookies represents the structure for saving cookies to disk
// including the URL they belong to and when they were stored
type StoredCookies struct {
	URL     string                      `json:"url"`
	Cookies []*proto.NetworkCookieParam `json:"cookies"`
	Time    time.Time                   `json:"time"`
}

// CookieStore handles saving and loading cookies from disk
type CookieStore struct {
	filePath string
}

// NewCookieStore creates a new cookie store, using a default path if none provided
func NewCookieStore(filePath string) *CookieStore {
	if filePath == "" {
		dir, _ := os.Getwd()
		filePath = filepath.Join(dir, "helix-smoke-test-cookies.json")
	}
	return &CookieStore{filePath: filePath}
}

// Save captures the current cookies from the browser page and writes them to disk
// Returns error if no auth cookies found or if writing fails
func (cs *CookieStore) Save(page *rod.Page, serverURL string) error {
	cookies := page.MustCookies(fmt.Sprintf("%s/auth/realms/helix/", serverURL))

	if len(cookies) == 0 {
		return fmt.Errorf("no auth cookies found after login")
	}

	stored := StoredCookies{
		URL:     serverURL,
		Cookies: convertCookies(cookies),
		Time:    time.Now(),
	}

	data, err := json.Marshal(stored)
	if err != nil {
		return fmt.Errorf("failed to marshal cookies: %w", err)
	}

	return os.WriteFile(cs.filePath, data, 0600)
}

// Load reads cookies from disk and restores them to the browser page
// Validates that cookies aren't expired (24h) and match the server URL
func (cs *CookieStore) Load(page *rod.Page, serverURL string) error {
	data, err := os.ReadFile(cs.filePath)
	if err != nil {
		return fmt.Errorf("no saved cookies found: %w", err)
	}

	var stored StoredCookies
	if err := json.Unmarshal(data, &stored); err != nil {
		return fmt.Errorf("failed to unmarshal cookies: %w", err)
	}

	// Check if cookies are expired (24 hours)
	if time.Since(stored.Time) > 24*time.Hour {
		return fmt.Errorf("cookies are expired")
	}

	// Check if cookies are for the right URL
	if stored.URL != serverURL {
		return fmt.Errorf("cookies are for different URL")
	}

	logStep("Setting cookies")
	return cs.setCookies(page, stored.Cookies)
}

// setCookies applies the stored cookies to the browser page
// If cookie domain is empty, uses the current server's domain
func (cs *CookieStore) setCookies(page *rod.Page, cookies []*proto.NetworkCookieParam) error {
	for _, cookie := range cookies {
		if cookie.Domain == "" {
			serverURL := getServerURL()
			parsedURL, err := url.Parse(serverURL)
			if err == nil {
				cookie.Domain = parsedURL.Host
			}
		}

		if err := page.SetCookies([]*proto.NetworkCookieParam{cookie}); err != nil {
			return fmt.Errorf("failed to set cookie: %w", err)
		}
	}
	return nil
}

// convertCookies transforms rod's NetworkCookie objects into NetworkCookieParam objects
// which are needed for storing and restoring cookies
func convertCookies(cookies []*proto.NetworkCookie) []*proto.NetworkCookieParam {
	var params []*proto.NetworkCookieParam
	for _, c := range cookies {
		params = append(params, &proto.NetworkCookieParam{
			Name:     c.Name,
			Value:    c.Value,
			Domain:   c.Domain,
			Path:     c.Path,
			Secure:   c.Secure,
			HTTPOnly: c.HTTPOnly,
			SameSite: c.SameSite,
			Expires:  c.Expires,
		})
	}
	return params
}
