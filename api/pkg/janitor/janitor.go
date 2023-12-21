package janitor

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/gorilla/mux"
	"github.com/lukemarsden/helix/api/pkg/system"
	"github.com/lukemarsden/helix/api/pkg/types"
)

type JanitorOptions struct {
	AppURL                  string
	SlackWebhookURL         string
	SentryDSNApi            string
	SentryDSNFrontend       string
	GoogleAnalyticsFrontend string
	IgnoreUsers             []string
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
	if j.Options.SentryDSNApi != "" {
		err = sentry.Init(sentry.ClientOptions{
			Dsn:              j.Options.SentryDSNApi,
			EnableTracing:    true,
			TracesSampleRate: 1.0,
		})
		if err != nil {
			return fmt.Errorf("Sentry initialization failed: %v\n", err)
		}
		system.SetHTTPErrorHandler(func(err *system.HTTPError, req *http.Request) {
			reportErrorWithRequest(err, req, map[string]interface{}{})
		})
		system.SetErrorHandler(func(err error, req *http.Request) {
			reportErrorWithRequest(err, req, map[string]interface{}{})
		})
	}
	return nil
}

// allows the janitor to attach middleware to the router
// before all the routes
func (j *Janitor) InjectMiddleware(router *mux.Router) error {
	if j.Options.SentryDSNApi != "" {
		router.Use(SentryMiddleware)
	}
	return nil
}

func (j *Janitor) getSessionURL(session *types.Session) string {
	return fmt.Sprintf(`%s/session/%s`, j.Options.AppURL, session.ID)
}

func (j *Janitor) CaptureError(err error) error {
	if j.Options.SentryDSNApi == "" {
		return nil
	}
	sentry.CaptureException(err)
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

func (j *Janitor) WriteSessionError(session *types.Session, sessionErr error) error {
	err := j.CaptureError(sessionErr)
	if err != nil {
		return err
	}
	message := fmt.Sprintf("❌ there was a session error %s %s", j.getSessionURL(session), sessionErr.Error())
	return sendSlackNotification(j.Options.SlackWebhookURL, message)
}

func (j *Janitor) WriteSessionEvent(eventType types.SessionEventType, ctx types.RequestContext, session *types.Session) error {
	message := ""
	if eventType == types.SessionEventTypeCreated {
		message = fmt.Sprintf("🚀 %s created a NEW session %s (mode=%s, model=%s)", ctx.Email, j.getSessionURL(session), session.Mode, session.ModelName)
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

func SentryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub := sentry.GetHubFromContext(r.Context())
		if hub == nil {
			hub = sentry.CurrentHub().Clone()
			r = r.WithContext(sentry.SetHubOnContext(r.Context(), hub))
		}

		defer func() {
			if err := recover(); err != nil {
				hub.Recover(err)
				// Optionally, write a custom response here
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func reportErrorWithRequest(err error, req *http.Request, extraData map[string]interface{}) {
	if err == nil || req == nil {
		return
	}

	// Create a new Sentry event.
	event := sentry.NewEvent()
	event.Level = sentry.LevelError
	event.Message = err.Error()

	// Add stack trace.
	event.Threads = []sentry.Thread{{
		Stacktrace: sentry.NewStacktrace(),
		Crashed:    false,
		Current:    true,
	}}

	// Add HTTP request information.
	event.Request = sentry.NewRequest(req)

	// Add additional labels or metadata.
	for key, value := range extraData {
		event.Extra[key] = value
	}

	// Capture the event.
	sentry.CaptureEvent(event)
}
