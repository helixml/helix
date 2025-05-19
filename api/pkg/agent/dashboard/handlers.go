package dashboard

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

// HandleUsers returns a handler function that serves the /api/users endpoint
//
// API Endpoint: GET /api/users
//
// Query Parameters:
//   - limit: (optional) Maximum number of users to return. Default: 20
//   - offset: (optional) Number of users to skip. Default: 0
//
// Response:
//   - 200 OK: Returns a JSON array of DashboardUser objects with the following structure:
//     [
//     {
//     "ID": "string",
//     "Name": "string",
//     "Email": "string",
//     "OrganizationName": "string"
//     },
//     ...
//     ]
//   - 405 Method Not Allowed: If the request method is not GET
//   - 500 Internal Server Error: If there's an error retrieving or encoding the data
func (d *Dashboard) HandleUsers() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse query parameters for pagination
		limit := 20 // Default limit
		offset := 0 // Default offset

		limitParam := r.URL.Query().Get("limit")
		if limitParam != "" {
			if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
				limit = parsedLimit
			}
		}

		offsetParam := r.URL.Query().Get("offset")
		if offsetParam != "" {
			if parsedOffset, err := strconv.Atoi(offsetParam); err == nil && parsedOffset >= 0 {
				offset = parsedOffset
			}
		}

		// Get users from storage
		users, err := d.storage.GetDashboardUsers(limit, offset)
		if err != nil {
			log.Printf("Error getting dashboard users: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Encode and return users as JSON
		if err := json.NewEncoder(w).Encode(users); err != nil {
			log.Printf("Error encoding users to JSON: %v", err)
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
			return
		}
	}
}

// HandleUserConversations returns a handler function that serves the /api/user/<userid>/conversations endpoint
//
// API Endpoint: GET /api/user/<userid>/conversations
//
// URL Parameters:
//   - userid: (required) The ID of the user whose conversations to retrieve
//
// Query Parameters:
//   - limit: (optional) Maximum number of conversations to return. Default: 20
//   - offset: (optional) Number of conversations to skip. Default: 0
//
// Response:
//   - 200 OK: Returns a JSON array of DashboardConversation objects with the following structure:
//     [
//     {
//     "SessionID": "string",
//     "UserMessage": "string",
//     "UserMessageTime": "RFC3339 timestamp",
//     "AssistantMessage": "string",
//     "AssistantMessageTime": "RFC3339 timestamp"
//     },
//     ...
//     ]
//   - 400 Bad Request: If the URL path is invalid or userID is missing
//   - 405 Method Not Allowed: If the request method is not GET
//   - 500 Internal Server Error: If there's an error retrieving or encoding the data
func (d *Dashboard) HandleUserConversations() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Extract user ID from the URL path
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/user/") {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		// Remove the '/api/user/' prefix
		path = strings.TrimPrefix(path, "/api/user/")

		// Check if the path contains '/conversations'
		if !strings.HasSuffix(path, "/conversations") {
			http.Error(w, "Invalid path", http.StatusBadRequest)
			return
		}

		// Extract user ID
		userID := strings.TrimSuffix(path, "/conversations")
		if userID == "" {
			http.Error(w, "User ID is required", http.StatusBadRequest)
			return
		}

		// Parse query parameters for pagination
		limit := 20 // Default limit
		offset := 0 // Default offset

		limitParam := r.URL.Query().Get("limit")
		if limitParam != "" {
			if parsedLimit, err := strconv.Atoi(limitParam); err == nil && parsedLimit > 0 {
				limit = parsedLimit
			}
		}

		offsetParam := r.URL.Query().Get("offset")
		if offsetParam != "" {
			if parsedOffset, err := strconv.Atoi(offsetParam); err == nil && parsedOffset >= 0 {
				offset = parsedOffset
			}
		}

		// Get conversations from storage
		conversations, err := d.storage.GetDashboardConversations(userID, limit, offset)
		if err != nil {
			log.Printf("Error getting conversations for user %s: %v", userID, err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}

		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Encode and return conversations as JSON
		if err := json.NewEncoder(w).Encode(conversations); err != nil {
			log.Printf("Error encoding conversations to JSON: %v", err)
			http.Error(w, "Error encoding response", http.StatusInternalServerError)
			return
		}
	}
}
