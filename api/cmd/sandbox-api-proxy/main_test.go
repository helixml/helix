package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAPIProxyForwardsRequest(t *testing.T) {
	var expectedHost string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/base/v1/test" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.RawQuery != "stream=true" {
			t.Fatalf("query = %q", r.URL.RawQuery)
		}
		if r.Header.Get("Authorization") != "Bearer test" {
			t.Fatalf("authorization header not forwarded")
		}
		if r.Host != expectedHost {
			t.Fatalf("host = %q, want %q", r.Host, expectedHost)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, "proxied")
	}))
	defer upstream.Close()
	expectedHost = strings.TrimPrefix(upstream.URL, "http://")

	handler, err := newAPIProxy(upstream.URL + "/base")
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler)
	defer server.Close()

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/test?stream=true", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusCreated || string(body) != "proxied" {
		t.Fatalf("status/body = %d/%q", resp.StatusCode, body)
	}
}

func TestAPIProxyRejectsInvalidUpstream(t *testing.T) {
	for _, upstream := range []string{"", "api:8080", "ftp://api.example.com"} {
		if _, err := newAPIProxy(upstream); err == nil {
			t.Fatalf("newAPIProxy(%q) succeeded", upstream)
		}
	}
}
