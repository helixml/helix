package janitor

import (
	"fmt"
	"strings"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type JanitorOptions struct {
	AppURL          string
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
	if j.Options.SlackWebhookURL == "" {
		return nil
	}
	return sendSlackNotification(j.Options.SlackWebhookURL, message)
}

func (j *Janitor) WriteSessionEvent(eventName string, ctx types.RequestContext, session *types.Session) error {
	if ctx.Owner == "" {
		return nil
	}
	sessionURL := fmt.Sprintf(`[%s](%s/sessions/%s)`, session.ID[:8], j.Options.AppURL, session.ID)
	title := fmt.Sprintf(`%s (%s) *%s*: %s`, ctx.FullName, ctx.Email, eventName, sessionURL)
	sessionDesc := fmt.Sprintf(` * name: %s\n * mode: %s\n * model: %s`, session.Name, session.Mode, session.ModelName)
	finalMessage := strings.Join([]string{
		title,
		sessionDesc,
	}, "\n")
	return j.SendMessage(finalMessage)
}
