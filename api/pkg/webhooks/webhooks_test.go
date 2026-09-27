package webhooks

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/config"
	helixcrypto "github.com/helixml/helix/api/pkg/crypto"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
	standardwebhooks "github.com/standard-webhooks/standard-webhooks/libraries/go"
	"github.com/stretchr/testify/require"
)

type deliveryTestStore struct {
	store.Store
	event    *types.WebhookEvent
	endpoint *types.WebhookEndpoint
	update   *store.WebhookDeliveryUpdate
}

func (s *deliveryTestStore) GetWebhookEvent(_ context.Context, _ string) (*types.WebhookEvent, error) {
	return s.event, nil
}

func (s *deliveryTestStore) GetWebhookEndpoint(_ context.Context, _, _ string) (*types.WebhookEndpoint, error) {
	return s.endpoint, nil
}

func (s *deliveryTestStore) CompleteWebhookDelivery(_ context.Context, update *store.WebhookDeliveryUpdate) error {
	s.update = update
	return nil
}

func TestValidateEndpointURL(t *testing.T) {
	tests := []struct {
		name         string
		url          string
		allowPrivate bool
		wantErr      bool
	}{
		{name: "public HTTPS", url: "https://hooks.example.com/helix"},
		{name: "credentials", url: "https://user:pass@example.com/hook", wantErr: true},
		{name: "HTTP rejected", url: "http://hooks.example.com/hook", wantErr: true},
		{name: "loopback rejected", url: "https://127.0.0.1/hook", wantErr: true},
		{name: "private rejected", url: "https://10.0.0.1/hook", wantErr: true},
		{name: "local development opt in", url: "http://127.0.0.1:9000/hook", allowPrivate: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateEndpointURL(tt.url, tt.allowPrivate)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestSignIncludesCurrentAndUnexpiredPreviousSecret(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	current := base64.StdEncoding.EncodeToString([]byte("current standard webhook test key"))
	previous := base64.StdEncoding.EncodeToString([]byte("previous standard webhook test key"))
	currentEncrypted, err := helixcrypto.EncryptAES256GCM([]byte(current), key)
	require.NoError(t, err)
	previousEncrypted, err := helixcrypto.EncryptAES256GCM([]byte(previous), key)
	require.NoError(t, err)
	now := time.Unix(1_800_000_000, 0).UTC()
	expires := now.Add(time.Hour)
	dispatcher := NewDispatcher(nil, func() ([]byte, error) { return key, nil }, config.Webhooks{})
	signature, err := dispatcher.sign(&types.WebhookEndpoint{
		SecretEncrypted:         currentEncrypted,
		PreviousSecretEncrypted: previousEncrypted,
		PreviousSecretExpiresAt: &expires,
	}, "whd_test", now, []byte(`{"ok":true}`))
	require.NoError(t, err)

	headers := http.Header{}
	headers.Set(standardwebhooks.HeaderWebhookID, "whd_test")
	headers.Set(standardwebhooks.HeaderWebhookTimestamp, "1800000000")
	headers.Set(standardwebhooks.HeaderWebhookSignature, signature)
	currentVerifier, err := standardwebhooks.NewWebhook(current)
	require.NoError(t, err)
	previousVerifier, err := standardwebhooks.NewWebhook(previous)
	require.NoError(t, err)
	require.NoError(t, currentVerifier.VerifyIgnoringTimestamp([]byte(`{"ok":true}`), headers))
	require.NoError(t, previousVerifier.VerifyIgnoringTimestamp([]byte(`{"ok":true}`), headers))
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 9, 22, 12, 0, 0, 0, time.UTC)
	require.Equal(t, now.Add(90*time.Second), parseRetryAfter("90", now))
	require.Equal(t, now.Add(time.Hour), parseRetryAfter(now.Add(time.Hour).Format(http.TimeFormat), now))
	require.True(t, parseRetryAfter("invalid", now).IsZero())
}

func TestDeliveryUsesStandardWebhookHeaders(t *testing.T) {
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("standard webhook delivery test key"))
	key := []byte("01234567890123456789012345678901")
	encrypted, err := helixcrypto.EncryptAES256GCM([]byte(secret), key)
	require.NoError(t, err)
	received := make(chan *http.Request, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		require.NoError(t, readErr)
		verifier, verifyErr := standardwebhooks.NewWebhook(secret)
		require.NoError(t, verifyErr)
		require.NoError(t, verifier.Verify(body, r.Header))
		require.Equal(t, "spec_task.status_changed", r.Header.Get("webhook-type"))
		received <- r
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	now := time.Now().UTC().Truncate(time.Second)
	st := &deliveryTestStore{
		event: &types.WebhookEvent{
			ID: "whevt_test", APIVersion: "v1", Type: types.WebhookEventSpecTaskStatusChanged,
			OrganizationID: "org", ProjectID: "project", Data: []byte(`{"spec_task_id":"task"}`), CreatedAt: now,
		},
		endpoint: &types.WebhookEndpoint{
			ID: "whep_test", OrganizationID: "org", URL: server.URL,
			Status: types.WebhookEndpointStatusActive, SecretEncrypted: encrypted,
		},
	}
	dispatcher := NewDispatcher(st, func() ([]byte, error) { return key, nil }, config.Webhooks{
		AllowPrivateEndpoints: true, MaxAttempts: 3,
	})
	dispatcher.now = func() time.Time { return now }
	require.NoError(t, dispatcher.deliver(context.Background(), &types.WebhookDelivery{
		ID: "whd_test", EventID: "whevt_test", EndpointID: "whep_test",
	}))
	select {
	case request := <-received:
		require.Equal(t, "whd_test", request.Header.Get(standardwebhooks.HeaderWebhookID))
	case <-time.After(time.Second):
		t.Fatal("receiver did not observe delivery")
	}
	require.NotNil(t, st.update)
	require.Equal(t, types.WebhookDeliveryStatusDelivered, st.update.Status)
	require.Equal(t, 1, st.update.AttemptCount)
}
