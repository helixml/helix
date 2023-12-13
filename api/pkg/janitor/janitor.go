package janitor

import (
	"fmt"
	"strings"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type JanitorOptions struct {
	AppURL          string
	SlackEnabled    bool
	SlackWebhookURL string
}

type Janitor struct {
	Options JanitorOptions
}

func NewJanitor(opts JanitorOptions) *Janitor {
	return &Janitor{
		Options: opts,
	}
}

func (j *Janitor) SendMessage(message string) error {
	if !j.Options.SlackEnabled {
		return nil
	}
	if j.Options.SlackWebhookURL == "" {
		return nil
	}
	return sendSlackNotification(j.Options.SlackWebhookURL, message)
}

func (j *Janitor) WriteSessionEvent(eventName string, session *types.Session) error {
	sessionURL := fmt.Sprintf(`[%s](%s/sessions/%s)`, session.ID[:8], j.Options.AppURL, session.ID)
	title := fmt.Sprintf(`*%s*: %s`, eventName, sessionURL)
	sessionDesc := fmt.Sprintf(`name: %s, mode: %s, model: %s`, session.Name, session.Mode, session.ModelName)
	finalMessage := strings.Join([]string{
		title,
		sessionDesc,
	}, "\n")
	return j.SendMessage(finalMessage)
}
