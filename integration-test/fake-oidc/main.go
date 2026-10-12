// Command fake-oidc is a TEST-ONLY OpenID Connect provider for exercising
// Helix's OIDC login flow (AUTH_PROVIDER=oidc) without Google/Keycloak.
//
// It auto-approves any identity: the authorize endpoint shows a tiny form where
// you pick the email and whether it is verified, then redirects back with a code.
// Access tokens are opaque base64 blobs of the chosen claims, so tests can also
// mint bearer tokens directly via GET /mint?email=...&verified=true and call the
// Helix API with them (the API validates OIDC tokens via the userinfo endpoint).
//
// NEVER run this anywhere real: it signs in whoever asks.
//
// Usage:
//
//	go run ./integration-test/fake-oidc -listen :5556 \
//	  -issuer http://host.docker.internal:5556 -public-url http://localhost:5556
//
// and point the API at it with OIDC_URL=<issuer>, OIDC_CLIENT_ID=<anything>,
// OIDC_CLIENT_SECRET=<anything>, OIDC_AUDIENCE=<anything>, AUTH_PROVIDER=oidc.
// -issuer must be the URL the API uses; -public-url the one the browser uses.
package main

import (
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"gopkg.in/go-jose/go-jose.v2"
	"gopkg.in/go-jose/go-jose.v2/jwt"
)

type identity struct {
	Sub           string `json:"sub"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	Name          string `json:"name"`
	Nonce         string `json:"nonce,omitempty"`
}

type server struct {
	issuer    string
	publicURL string
	key       *rsa.PrivateKey
	signer    jose.Signer
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func newIdentity(email string, verified bool) identity {
	email = strings.TrimSpace(email)
	return identity{
		Sub:           "fake-" + strings.Trim(nonAlnum.ReplaceAllString(strings.ToLower(email), "-"), "-"),
		Email:         email,
		EmailVerified: verified,
		Name:          email,
	}
}

func encode(prefix string, id identity) string {
	b, _ := json.Marshal(id)
	return prefix + base64.RawURLEncoding.EncodeToString(b)
}

func decode(prefix, s string) (identity, error) {
	var id identity
	if !strings.HasPrefix(s, prefix) {
		return id, fmt.Errorf("bad token")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(s, prefix))
	if err != nil {
		return id, err
	}
	return id, json.Unmarshal(b, &id)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func (s *server) discovery(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, map[string]any{
		"issuer":                                s.issuer,
		"authorization_endpoint":                s.publicURL + "/authorize",
		"token_endpoint":                        s.issuer + "/token",
		"userinfo_endpoint":                     s.issuer + "/userinfo",
		"jwks_uri":                              s.issuer + "/jwks",
		"end_session_endpoint":                  s.publicURL + "/logout",
		"response_types_supported":              []string{"code"},
		"subject_types_supported":               []string{"public"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
	})
}

func (s *server) jwks(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{
		Key: &s.key.PublicKey, KeyID: "fake", Algorithm: "RS256", Use: "sig",
	}}})
}

var loginForm = template.Must(template.New("login").Parse(`<!doctype html>
<title>Fake OIDC login</title>
<h1>Fake OIDC (test only)</h1>
<form method="get" action="/authorize">
  {{range $k, $v := .Params}}{{range $v}}<input type="hidden" name="{{$k}}" value="{{.}}">{{end}}{{end}}
  <label>Email <input name="fake_email" id="fake_email" value="someone@gmail.com" size="40"></label><br>
  <label><input type="checkbox" name="fake_verified" id="fake_verified" value="true" checked> email verified</label><br>
  <button type="submit" id="fake_login">Sign in</button>
</form>`))

func (s *server) authorize(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	email := q.Get("fake_email")
	if email == "" {
		_ = loginForm.Execute(w, map[string]any{"Params": q})
		return
	}
	id := newIdentity(email, q.Get("fake_verified") == "true")
	id.Nonce = q.Get("nonce")
	redirect, err := url.Parse(q.Get("redirect_uri"))
	if err != nil || redirect.String() == "" {
		http.Error(w, "missing redirect_uri", http.StatusBadRequest)
		return
	}
	rq := redirect.Query()
	rq.Set("code", encode("code.", id))
	rq.Set("state", q.Get("state"))
	redirect.RawQuery = rq.Encode()
	http.Redirect(w, r, redirect.String(), http.StatusFound)
}

func (s *server) token(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var (
		id  identity
		err error
	)
	switch r.PostForm.Get("grant_type") {
	case "refresh_token":
		id, err = decode("refresh.", r.PostForm.Get("refresh_token"))
	default:
		id, err = decode("code.", r.PostForm.Get("code"))
	}
	if err != nil {
		http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
		return
	}
	clientID := r.PostForm.Get("client_id")
	if user, _, ok := r.BasicAuth(); ok {
		clientID, _ = url.QueryUnescape(user)
	}
	now := time.Now()
	idToken, err := jwt.Signed(s.signer).Claims(jwt.Claims{
		Issuer:   s.issuer,
		Subject:  id.Sub,
		Audience: jwt.Audience{clientID},
		IssuedAt: jwt.NewNumericDate(now),
		Expiry:   jwt.NewNumericDate(now.Add(time.Hour)),
	}).Claims(map[string]any{
		"email": id.Email, "email_verified": id.EmailVerified, "name": id.Name, "nonce": id.Nonce,
	}).CompactSerialize()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	id.Nonce = ""
	writeJSON(w, map[string]any{
		"access_token":  encode("fake.", id),
		"refresh_token": encode("refresh.", id),
		"id_token":      idToken,
		"token_type":    "Bearer",
		"expires_in":    3600,
	})
}

func (s *server) userinfo(w http.ResponseWriter, r *http.Request) {
	id, err := decode("fake.", strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}
	writeJSON(w, id)
}

// mint returns an access token for the given identity without a browser flow.
func (s *server) mint(w http.ResponseWriter, r *http.Request) {
	id := newIdentity(r.URL.Query().Get("email"), r.URL.Query().Get("verified") == "true")
	writeJSON(w, map[string]string{"access_token": encode("fake.", id), "sub": id.Sub})
}

func main() {
	listen := flag.String("listen", ":5556", "listen address")
	issuer := flag.String("issuer", "http://localhost:5556", "issuer URL (as reached by the Helix API)")
	publicURL := flag.String("public-url", "", "browser-facing base URL (defaults to -issuer)")
	flag.Parse()
	if *publicURL == "" {
		*publicURL = *issuer
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithHeader("kid", "fake"))
	if err != nil {
		log.Fatal(err)
	}
	s := &server{issuer: strings.TrimRight(*issuer, "/"), publicURL: strings.TrimRight(*publicURL, "/"), key: key, signer: signer}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", s.discovery)
	mux.HandleFunc("/jwks", s.jwks)
	mux.HandleFunc("/authorize", s.authorize)
	mux.HandleFunc("/token", s.token)
	mux.HandleFunc("/userinfo", s.userinfo)
	mux.HandleFunc("/mint", s.mint)
	mux.HandleFunc("/logout", func(w http.ResponseWriter, r *http.Request) { fmt.Fprintln(w, "logged out") })

	log.Printf("fake-oidc (TEST ONLY) listening on %s, issuer %s, public %s", *listen, s.issuer, s.publicURL)
	log.Fatal(http.ListenAndServe(*listen, mux))
}
