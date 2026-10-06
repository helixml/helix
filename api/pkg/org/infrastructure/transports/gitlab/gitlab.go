package gitlab

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/application/configregistry"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// Publisher appends one inbound delivery to its Trigger and routes it.
// *publishing.Publishing satisfies it; declared here so the transport
// does not import the application package.
type Publisher interface {
	PublishDelivery(ctx context.Context, orgID, triggerID string, eventID streaming.EventID, msg streaming.Message) (streaming.Event, error)
}

type Transport struct {
	orgID     string
	registry  *configregistry.Registry
	store     *store.Store
	publisher Publisher
	logger    *slog.Logger
	now       func() time.Time
}

func New(orgID string, registry *configregistry.Registry, st *store.Store, publisher Publisher, logger *slog.Logger) *Transport {
	return &Transport{orgID: orgID, registry: registry, store: st, publisher: publisher, logger: logger, now: time.Now}
}

type Config struct {
	SigningToken string `json:"signing_token"`
	SecretToken  string `json:"secret_token"`
}

func (t *Transport) config(ctx context.Context) (Config, error) {
	var config Config
	if err := t.registry.GetObject(ctx, t.orgID, "transport.gitlab", &config); err != nil {
		return Config{}, err
	}
	if config.SigningToken == "" && config.SecretToken == "" {
		return Config{}, fmt.Errorf("transport.gitlab authentication is not configured")
	}
	return config, nil
}

const maxBody = 25 << 20

func (t *Transport) HandleInboundForTrigger(triggerID string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		auth, err := t.config(r.Context())
		if err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		rows, err := t.store.Triggers.Find(r.Context(), store.WithOrg(t.orgID), store.WithID(triggerID), store.WithLimit(1))
		if err != nil || len(rows) == 0 {
			http.Error(w, "trigger not found", http.StatusNotFound)
			return
		}
		trg := rows[0]
		if trg.Kind != transport.KindGitLab {
			http.Error(w, "trigger is not a gitlab transport", http.StatusBadRequest)
			return
		}
		config, err := trg.Transport().GitLabConfig()
		if err != nil {
			http.Error(w, "trigger config invalid", http.StatusInternalServerError)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
		if err != nil {
			http.Error(w, "read body: "+err.Error(), http.StatusBadRequest)
			return
		}
		signature := r.Header.Get("webhook-signature")
		authenticated := signature != "" && verifySignature(auth.SigningToken, r.Header.Get("webhook-id"), r.Header.Get("webhook-timestamp"), body, signature, t.now())
		if signature == "" {
			authenticated = auth.SecretToken != "" && hmac.Equal([]byte(auth.SecretToken), []byte(r.Header.Get("X-Gitlab-Token")))
		}
		if !authenticated {
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			http.Error(w, "parse json body: "+err.Error(), http.StatusBadRequest)
			return
		}
		eventType := r.Header.Get("X-Gitlab-Event")
		if !contains(config.Events, eventType) || !strings.EqualFold(config.Repo, projectPath(payload)) {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		deliveryID := firstNonEmpty(r.Header.Get("webhook-id"), r.Header.Get("Idempotency-Key"), r.Header.Get("X-Gitlab-Event-UUID"))
		if deliveryID == "" {
			http.Error(w, "missing delivery id", http.StatusBadRequest)
			return
		}
		message := messageFor(eventType, payload)
		message.MessageID = deliveryID
		message.Extra = json.RawMessage(body)
		if _, err := t.publisher.PublishDelivery(r.Context(), t.orgID, trg.ID, eventID(t.orgID, trg.ID, deliveryID), message); err != nil {
			if errors.Is(err, store.ErrConflict) {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			http.Error(w, "publish delivery", http.StatusInternalServerError)
			return
		}
		t.logger.Info("gitlab.inbound", "trigger", trg.ID, "repo", config.Repo, "delivery", message.MessageID)
		w.WriteHeader(http.StatusNoContent)
	})
}

func verifySignature(token, id, timestamp string, body []byte, signatures string, now time.Time) bool {
	if id == "" || timestamp == "" || signatures == "" || !strings.HasPrefix(token, "whsec_") {
		return false
	}
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || now.Sub(time.Unix(seconds, 0)).Abs() > 5*time.Minute {
		return false
	}
	key, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(token, "whsec_"))
	if err != nil || len(key) == 0 {
		return false
	}
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "%s.%s.", id, timestamp)
	_, _ = mac.Write(body)
	expected := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	for signature := range strings.FieldsSeq(signatures) {
		if hmac.Equal([]byte(expected), []byte(signature)) {
			return true
		}
	}
	return false
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == "*" || candidate == value {
			return true
		}
	}
	return false
}

func nestedString(payload map[string]any, object, key string) string {
	nested, _ := payload[object].(map[string]any)
	value, _ := nested[key].(string)
	return value
}

func projectPath(payload map[string]any) string {
	return nestedString(payload, "project", "path_with_namespace")
}

func mergeRequestThread(payload map[string]any) string {
	nested, _ := payload["object_attributes"].(map[string]any)
	if iid, ok := nested["iid"].(float64); ok {
		return "!" + strconv.FormatInt(int64(iid), 10)
	}
	return ""
}

func messageFor(eventType string, payload map[string]any) streaming.Message {
	message := streaming.Message{From: nestedString(payload, "user", "username")}
	switch eventType {
	case "Merge Request Hook":
		message.Subject = nestedString(payload, "object_attributes", "title")
		message.Body = nestedString(payload, "object_attributes", "description")
		message.ThreadID = mergeRequestThread(payload)
	case "Note Hook":
		message.Body = nestedString(payload, "object_attributes", "note")
		message.ThreadID = nestedIID(payload, "merge_request")
	}
	return message
}

func nestedIID(payload map[string]any, object string) string {
	nested, _ := payload[object].(map[string]any)
	if iid, ok := nested["iid"].(float64); ok {
		return "!" + strconv.FormatInt(int64(iid), 10)
	}
	return ""
}

func eventID(orgID string, topicID streaming.StreamID, deliveryID string) streaming.EventID {
	digest := sha256.Sum256([]byte(orgID + "\x00" + string(topicID) + "\x00gitlab\x00" + deliveryID))
	return streaming.EventID("e-gitlab-" + hex.EncodeToString(digest[:]))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
