package janitor

import (
	"fmt"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/gorilla/mux"
	"github.com/lukemarsden/helix/api/pkg/types"
)

type JanitorOptions struct {
	AppURL          string
	SlackWebhookURL string
	SentryDSN       string
	IgnoreUsers     []string
}

type Janitor struct {
	Options JanitorOptions
	// don't log "created" then "updated" right after each other
	recentlyCreatedSubscriptionMap   map[string]bool
	recentlyCreatedSubscriptionMutex sync.Mutex
}

func NewJanitor(opts JanitorOptions) *Janitor {
	return &Janitor{
		Options:                        opts,
		recentlyCreatedSubscriptionMap: map[string]bool{},
	}
}

func (j *Janitor) Initialize() error {
	var err error
	if j.Options.SentryDSN != "" {
		err = sentry.Init(sentry.ClientOptions{
			Dsn:              j.Options.SentryDSN,
			EnableTracing:    true,
			TracesSampleRate: 1.0,
		})
		if err != nil {
			return fmt.Errorf("Sentry initialization failed: %v\n", err)
		}
	}
	return nil
}

// allows the janitor to attach middleware to the router
// before all the routes
func (j *Janitor) InjectMiddleware(router *mux.Router) error {
	if j.Options.SentryDSN != "" {
		sentryHandler := sentryhttp.New(sentryhttp.Options{})
		router.Use(sentryHandler.Handle)
	}
	return nil
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
		j.recentlyCreatedSubscriptionMutex.Lock()
		defer j.recentlyCreatedSubscriptionMutex.Unlock()
		if eventType == types.SubscriptionEventTypeCreated {
			j.recentlyCreatedSubscriptionMap[user.Email] = true
			go func() {
				time.Sleep(10 * time.Second)
				j.recentlyCreatedSubscriptionMutex.Lock()
				defer j.recentlyCreatedSubscriptionMutex.Unlock()
				delete(j.recentlyCreatedSubscriptionMap, user.Email)
			}()
			return fmt.Sprintf("💰 %s created a NEW subscription %s", user.Email, user.SubscriptionURL)
		} else if eventType == types.SubscriptionEventTypeUpdated {
			_, ok := j.recentlyCreatedSubscriptionMap[user.Email]
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
