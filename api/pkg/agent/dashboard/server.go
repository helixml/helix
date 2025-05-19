package dashboard

import (
	"fmt"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strconv"
	"time"
)

const DashboardAssetRemoteURL = "https://dash-assets.agentpod.ai"

type DashboardUser struct {
	ID               string
	Name             string
	Email            string
	OrganizationName string
}

// DashboardConversation is a pair of user message and assistant message
type DashboardConversation struct {
	SessionID            string
	UserMessage          string
	UserMessageTime      time.Time
	AssistantMessage     string
	AssistantMessageTime time.Time
}

// Storage defines the data access methods needed by the dashboard package.
type Storage interface {
	GetDashboardUsers(limit int, offset int) ([]DashboardUser, error)
	GetDashboardConversations(userID string, limit int, offset int) ([]DashboardConversation, error)
}

// Dashboard represents the web dashboard for the application
type Dashboard struct {
	// RemoteURL is the URL of the hosted dashboard
	RemoteURL string
	storage   Storage
}

// NewDashboard creates a new Dashboard instance for proxying to a URL. By default, it will use https://dash-assets.agentpod.ai
func NewDashboard(frontendURL string, storage Storage) *Dashboard {
	// Check if the provided string does not look like a URL (does not start with http:// or https://)
	if !(len(frontendURL) > 7 && (frontendURL[:7] == "http://" || frontendURL[:8] == "https://")) {
		panic("Invalid URL: must start with http:// or https://")
	}

	return &Dashboard{
		RemoteURL: frontendURL,
		storage:   storage,
	}
}

// Serve starts the dashboard file server on the specified port
// This method is intended to be run in a goroutine
func (d *Dashboard) Serve(port int) error {
	portStr := strconv.Itoa(port)
	addr := fmt.Sprintf(":%s", portStr)

	targetURL, err := url.Parse(d.RemoteURL)
	if err != nil {
		return fmt.Errorf("error parsing target URL: %v", err)
	}

	// Create a new reverse proxy
	proxy := httputil.NewSingleHostReverseProxy(targetURL)

	// Modify the request headers before sending them to the target
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		originalDirector(req)
		req.Host = targetURL.Host
		req.Header.Set("X-Forwarded-Host", req.Header.Get("Host"))
		req.Header.Set("X-Forwarded-Proto", "http")
	}

	// Register API handlers
	http.HandleFunc("/api/users", d.HandleUsers())
	http.HandleFunc("/api/user/", d.HandleUserConversations())

	// Default handler for all other requests - proxies to the frontend
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r)
	})

	log.Printf("Dashboard server started on http://localhost%s", addr)

	// ListenAndServe blocks until an error occurs
	return http.ListenAndServe(addr, nil)
}
