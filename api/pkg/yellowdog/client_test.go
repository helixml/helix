package yellowdog

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCredentialsAuthHeader(t *testing.T) {
	c := Credentials{KeyID: "ABC123", Secret: "supersecret"}
	got := c.authHeader()
	want := "yd-key ABC123:supersecret"
	if got != want {
		t.Errorf("authHeader() = %q, want %q", got, want)
	}
}

func TestCredentialsValid(t *testing.T) {
	cases := []struct {
		name  string
		creds Credentials
		want  bool
	}{
		{"both empty", Credentials{}, false},
		{"only key", Credentials{KeyID: "k"}, false},
		{"only secret", Credentials{Secret: "s"}, false},
		{"both set", Credentials{KeyID: "k", Secret: "s"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.creds.Valid(); got != tc.want {
				t.Errorf("Valid() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestNewClientRejectsInvalidCreds(t *testing.T) {
	_, err := NewClient(Credentials{})
	if err == nil {
		t.Fatal("expected error from NewClient with empty credentials, got nil")
	}
}

// fakeServer stands in for portal.yellowdog.co. The test cases assert
// against the request the client emitted (headers, path, body) and
// reply with whatever shape the test wants.
func fakeServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *Client) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	c, err := NewClient(
		Credentials{KeyID: "k", Secret: "s"},
		WithBaseURL(srv.URL),
	)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	return srv, c
}

func TestListNamespacesSendsAuthHeader(t *testing.T) {
	var gotAuth string
	_, c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{}, "nextSliceId": nil})
	})

	if _, err := c.ListNamespaces(context.Background()); err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	want := "yd-key k:s"
	if gotAuth != want {
		t.Errorf("Authorization header = %q, want %q", gotAuth, want)
	}
}

func TestListNamespacesHitsCorrectPath(t *testing.T) {
	var gotPath string
	_, c := fakeServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[],"nextSliceId":null}`))
	})
	if _, err := c.ListNamespaces(context.Background()); err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if gotPath != "/namespaces" {
		t.Errorf("path = %q, want %q", gotPath, "/namespaces")
	}
}

func TestListNamespacesDecodesItems(t *testing.T) {
	_, c := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"items":[
				{"id":"18048B","namespace":"yd-demo","deletable":false},
				{"id":"5C0252","namespace":"development","deletable":false}
			],
			"nextSliceId":null
		}`))
	})

	page, err := c.ListNamespaces(context.Background())
	if err != nil {
		t.Fatalf("ListNamespaces: %v", err)
	}
	if len(page.Items) != 2 {
		t.Fatalf("len(Items) = %d, want 2", len(page.Items))
	}
	if page.Items[0].ID != "18048B" || page.Items[0].Namespace != "yd-demo" {
		t.Errorf("first item = %+v, want id=18048B namespace=yd-demo", page.Items[0])
	}
	if page.HasMore() {
		t.Errorf("HasMore() = true, want false when nextSliceId is null")
	}
}

func TestAPIErrorDecodedOn4xx(t *testing.T) {
	_, c := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{
			"type":"about:blank",
			"title":"Unauthorized",
			"status":401,
			"detail":"key not recognised",
			"instance":"/api/namespaces"
		}`))
	})

	_, err := c.ListNamespaces(context.Background())
	if err == nil {
		t.Fatal("expected error on 401, got nil")
	}
	if !IsUnauthorized(err) {
		t.Errorf("IsUnauthorized(err) = false, want true (err = %v)", err)
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type = %T, want *APIError", err)
	}
	if apiErr.Status != 401 {
		t.Errorf("Status = %d, want 401", apiErr.Status)
	}
	if !strings.Contains(apiErr.Error(), "key not recognised") {
		t.Errorf("Error() missing detail; got %q", apiErr.Error())
	}
}

func TestAPIErrorOnNon4xxNonJSONResponse(t *testing.T) {
	_, c := fakeServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream is sad")) // not JSON
	})

	_, err := c.ListNamespaces(context.Background())
	if err == nil {
		t.Fatal("expected error on 502, got nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("err type = %T, want *APIError", err)
	}
	if apiErr.Status != http.StatusBadGateway {
		t.Errorf("Status = %d, want %d", apiErr.Status, http.StatusBadGateway)
	}
}
