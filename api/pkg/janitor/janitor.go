package janitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/lukemarsden/helix/api/pkg/types"
)

type JanitorOptions struct {
	AppURL          string
	SlackWebhookURL string
	IgnoreUsers     []string
}

type Janitor struct {
	Options JanitorOptions
	// don't log "created" then "updated" right after each other
	recentlyCreatedMap   map[string]bool
	recentlyCreatedMutex sync.Mutex
}

func NewJanitor(opts JanitorOptions) *Janitor {
	return &Janitor{
		Options:            opts,
		recentlyCreatedMap: map[string]bool{},
	}
}

func (j *Janitor) SendMessage(userEmail string, message string) error {
	if j.Options.SlackWebhookURL == "" {
		return nil
	}
	for _, ignoredUser := range j.Options.IgnoreUsers {
		if ignoredUser == userEmail {
			return nil
		}
	}
	return sendSlackNotification(j.Options.SlackWebhookURL, message)
}

func (j *Janitor) WriteSessionEvent(eventType types.SessionEventType, ctx types.RequestContext, session *types.Session) error {
	message := ""
	if eventType == types.SessionEventTypeCreated {
		sessionLink := fmt.Sprintf(`%s/session/%s`, j.Options.AppURL, session.ID)
		message = fmt.Sprintf("🚀 %s created a NEW session %s (mode=%s, model=%s)", ctx.Email, sessionLink, session.Mode, session.ModelName)
	}
	return j.SendMessage(ctx.Email, message)
}

func (j *Janitor) WriteSubscriptionEvent(eventType types.SubscriptionEventType, user types.StripeUser) error {
	message := func() string {
		j.recentlyCreatedMutex.Lock()
		defer j.recentlyCreatedMutex.Unlock()
		if eventType == types.SubscriptionEventTypeCreated {
			j.recentlyCreatedMap[user.Email] = true
			go func() {
				time.Sleep(10 * time.Second)
				j.recentlyCreatedMutex.Lock()
				defer j.recentlyCreatedMutex.Unlock()
				delete(j.recentlyCreatedMap, user.Email)
			}()
			return fmt.Sprintf("💰 %s created a NEW subscription %s", user.Email, user.SubscriptionURL)
		} else if eventType == types.SubscriptionEventTypeUpdated {
			_, ok := j.recentlyCreatedMap[user.Email]
			if ok {
				return ""
			}
			return fmt.Sprintf("🎉 %s UPDATED their subscription %s", user.Email, user.SubscriptionURL)
		} else if eventType == types.SubscriptionEventTypeDeleted {
			return fmt.Sprintf("🛑 %s CANCELLED their subscription %s", user.Email, user.SubscriptionURL)
		} else {
			return ""
		}
	}()

	return j.SendMessage(user.Email, message)
}
