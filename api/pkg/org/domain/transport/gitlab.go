package transport

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const KindGitLab Kind = "gitlab"

type GitLabConfig struct {
	Repo           string   `json:"repo,omitempty"`
	RepositoryID   string   `json:"repository_id,omitempty"`
	Events         []string `json:"events,omitempty"`
	WebhookID      int64    `json:"webhook_id,omitempty"`
	WebhookHTMLURL string   `json:"webhook_html_url,omitempty"`
}

func (g GitLabConfig) Validate() error {
	if g.RepositoryID == "" {
		return errors.New("gitlab transport: repository_id is required")
	}
	if g.Repo != "" && (strings.Trim(g.Repo, "/") == "" || !strings.Contains(strings.Trim(g.Repo, "/"), "/")) {
		return fmt.Errorf("gitlab transport: repo %q must be a namespace/project path", g.Repo)
	}
	if len(g.Events) == 0 {
		return errors.New("gitlab transport: events whitelist is required and must be non-empty")
	}
	for _, event := range g.Events {
		switch event {
		case "Merge Request Hook", "Note Hook", "Pipeline Hook", "Push Hook":
		default:
			return fmt.Errorf("gitlab transport: invalid event %q (expected an X-Gitlab-Event value such as Merge Request Hook)", event)
		}
	}
	return nil
}

func (t Transport) GitLabConfig() (GitLabConfig, error) {
	if t.Kind != KindGitLab {
		return GitLabConfig{}, fmt.Errorf("transport kind is %q, not gitlab", t.Kind)
	}
	return parseGitLabConfig(t.Config)
}

type gitlab struct{}

func (gitlab) ParseConfig(raw json.RawMessage) (Config, error) {
	return parseGitLabConfig(raw)
}

func parseGitLabConfig(raw json.RawMessage) (GitLabConfig, error) {
	var config GitLabConfig
	if len(raw) == 0 {
		return config, nil
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return GitLabConfig{}, fmt.Errorf("parse gitlab config: %w", err)
	}
	return config, nil
}
