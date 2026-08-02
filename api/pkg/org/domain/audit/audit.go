package audit

import (
	"context"
	"encoding/json"
	"strings"
)

type EventType string

const (
	EventMCPCall       EventType = "mcp_call"
	EventSSHConnection EventType = "ssh_connection"
	EventSSHCommand    EventType = "ssh_command"
)

type Status string

const (
	StatusAttempted Status = "attempted"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
)

type ActorType string

const (
	ActorBot  ActorType = "bot"
	ActorUser ActorType = "user"
)

type Metadata struct {
	Arguments     json.RawMessage `json:"arguments,omitempty"`
	AssetRef      string          `json:"asset_ref,omitempty"`
	Command       string          `json:"command,omitempty"`
	CommandID     string          `json:"command_id,omitempty"`
	Error         string          `json:"error,omitempty"`
	RemoteAddress string          `json:"remote_address,omitempty"`
	LocalAddress  string          `json:"local_address,omitempty"`
	SSHUser       string          `json:"ssh_user,omitempty"`
	ClientVersion string          `json:"client_version,omitempty"`
	DurationMS    int64           `json:"duration_ms,omitempty"`
}

type Entry struct {
	OrganizationID string
	ProjectID      string
	UserID         string
	ActorID        string
	ActorType      ActorType
	AssetID        string
	EventType      EventType
	Action         string
	Status         Status
	Metadata       Metadata
}

type Recorder interface {
	Record(ctx context.Context, entry Entry) error
}

type ProjectResolver func(ctx context.Context, orgID, actorID string) (string, error)

func RedactArguments(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage(`{}`)
	}
	redacted, ok := redactValue(raw)
	if !ok {
		return json.RawMessage(`{}`)
	}
	return redacted
}

func redactValue(raw json.RawMessage) (json.RawMessage, bool) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err == nil && object != nil {
		for key, value := range object {
			if sensitiveKey(key) {
				object[key] = json.RawMessage(`"[REDACTED]"`)
				continue
			}
			if child, ok := redactValue(value); ok {
				object[key] = child
			}
		}
		out, err := json.Marshal(object)
		return out, err == nil
	}

	var array []json.RawMessage
	if err := json.Unmarshal(raw, &array); err == nil && array != nil {
		for idx, value := range array {
			if child, ok := redactValue(value); ok {
				array[idx] = child
			}
		}
		out, err := json.Marshal(array)
		return out, err == nil
	}

	if json.Valid(raw) {
		return append(json.RawMessage(nil), raw...), true
	}
	return nil, false
}

func sensitiveKey(key string) bool {
	normalized := strings.Map(func(r rune) rune {
		if r >= 'A' && r <= 'Z' {
			return r + ('a' - 'A')
		}
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.TrimSpace(key))
	switch normalized {
	case "auth", "authorization", "credential", "credentials", "encryptedpassword", "encryptedprivatekey", "password", "passphrase", "privatekey", "secret", "token":
		return true
	default:
		return strings.HasSuffix(normalized, "apikey") ||
			strings.HasSuffix(normalized, "password") ||
			strings.HasSuffix(normalized, "passphrase") ||
			strings.HasSuffix(normalized, "privatekey") ||
			strings.HasSuffix(normalized, "secret") ||
			strings.HasSuffix(normalized, "token")
	}
}
