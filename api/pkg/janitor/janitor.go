package janitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/types"
)

type Janitor struct {
	cfg config.Janitor
	// don't log "created" then "updated" right after each other
	recentlyCreatedSubscriptionMap   map[string]bool
	recentlyCreatedSubscriptionMutex sync.Mutex

	// keep track of the sessions we've already pinged slack about
	// so retry buttons don't spam the channel
	seenErrorSessionMap   map[string]bool
	seenErrorSessionMutex sync.Mutex
}

func NewJanitor(cfg config.Janitor) *Janitor {
	return &Janitor{
		cfg:                            cfg,
		recentlyCreatedSubscriptionMap: map[string]bool{},
		seenErrorSessionMap:            map[string]bool{},
	}
}

func (j *Janitor) getSessionURL(session *types.Session) string {
	return fmt.Sprintf(`%s/session/%s`, j.cfg.AppURL, session.ID)
}

func (j *Janitor) SendMessage(userEmail string, message string) error {
	if j.cfg.SlackWebhookURL == "" {
		return nil
	}
	for _, ignoredUser := range j.cfg.SlackIgnoreUser {
		if ignoredUser == userEmail {
			return nil
		}
	}
	return sendSlackNotification(j.cfg.SlackWebhookURL, message)
}

func (j *Janitor) SendSubscriptionMessage(message string) error {
	if j.cfg.SubscriptionsSlackWebhookURL == "" {
		return nil
	}
	return sendSlackNotification(j.cfg.SubscriptionsSlackWebhookURL, message)
}

func (j *Janitor) WriteSessionError(session *types.Session, sessionErr error) error {
	j.seenErrorSessionMutex.Lock()
	defer j.seenErrorSessionMutex.Unlock()
	_, ok := j.seenErrorSessionMap[session.ID]

	if !ok {
		j.seenErrorSessionMap[session.ID] = true
		message := fmt.Sprintf("❌ there was a session error %s %s", j.getSessionURL(session), sessionErr.Error())
		return sendSlackNotification(j.cfg.SlackWebhookURL, message)
	}
	return nil
}

func (j *Janitor) WriteSessionEvent(eventType types.SessionEventType, user *types.User, session *types.Session) error {
	message := ""
	if eventType == types.SessionEventTypeCreated {
		message = fmt.Sprintf("🚀 %s created a NEW session %s (mode=%s, model=%s)", user.Email, j.getSessionURL(session), session.Mode, session.ModelName)
	}
	return j.SendMessage(user.Email, message)
}

func (j *Janitor) WriteSubscriptionEvent(eventType types.SubscriptionEventType, user types.StripeUser) error {
	message := func() string {
		j.recentlyCreatedSubscriptionMutex.Lock()
		defer j.recentlyCreatedSubscriptionMutex.Unlock()
		switch eventType {
		case types.SubscriptionEventTypeCreated:
			j.recentlyCreatedSubscriptionMap[user.Email] = true
			go func() {
				time.Sleep(10 * time.Second)
				j.recentlyCreatedSubscriptionMutex.Lock()
				defer j.recentlyCreatedSubscriptionMutex.Unlock()
				delete(j.recentlyCreatedSubscriptionMap, user.Email)
			}()
			return fmt.Sprintf("💰 %s created a NEW subscription %s", user.Email, user.SubscriptionURL)
		case types.SubscriptionEventTypeUpdated:
			_, ok := j.recentlyCreatedSubscriptionMap[user.Email]
			if ok {
				return ""
			}
			return fmt.Sprintf("🎉 %s UPDATED their subscription %s", user.Email, user.SubscriptionURL)
		case types.SubscriptionEventTypeDeleted:
			return fmt.Sprintf("🛑 %s CANCELLED their subscription %s", user.Email, user.SubscriptionURL)
		default:
			return ""
		}
	}()

	return j.SendMessage(user.Email, message)
}
