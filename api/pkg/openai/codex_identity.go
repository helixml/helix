package openai

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// codexJWKSURL publishes the RS256 keys OpenAI signs Codex id_tokens with.
// It is a var (not const) so tests can point it at an httptest server.
var codexJWKSURL = "https://auth.openai.com/.well-known/jwks.json"

const codexTokenIssuer = "https://auth.openai.com"

// codexAuthClaim is the namespaced claim OpenAI puts the ChatGPT subscription
// facts under, alongside the standard OIDC email/name claims.
const codexAuthClaim = "https://api.openai.com/auth"

// CodexIdentity is the ChatGPT account a Codex credential authenticates as.
// Every field is taken from claims OpenAI signed, never from user input.
type CodexIdentity struct {
	// AccountEmail is the ChatGPT account's email — the identity that gets
	// billed, distinct from the Helix user/org that connected it.
	AccountEmail string
	AccountName  string
	// PlanType is OpenAI's chatgpt_plan_type ("pro", "plus", "team", …).
	PlanType string
	// AccountID is chatgpt_account_id, present even when a token carries no
	// email — the Codex equivalent of Anthropic's organization uuid.
	AccountID string
}

type codexJWKS struct {
	Keys []struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	} `json:"keys"`
}

// JWKS rotation is infrequent; a short cache keeps a burst of subscription
// reads from hammering OpenAI while still picking up new keys same-day.
const codexJWKSCacheTTL = time.Hour

// A failed fetch is cached too, briefly. Without this, one unreachable
// auth.openai.com turns a list of N un-named subscriptions into N sequential
// 10s fetches, all serialized behind this mutex, blocking every other request
// that touches Codex identity.
const codexJWKSFailureTTL = 30 * time.Second

var (
	codexJWKSMu        sync.Mutex
	codexJWKSCache     map[string]*rsa.PublicKey
	codexJWKSFetched   time.Time
	codexJWKSCacheURL  string
	codexJWKSFailedAt  time.Time
	codexJWKSFailedErr error
)

func fetchCodexJWKS(ctx context.Context) (keys map[string]*rsa.PublicKey, err error) {
	codexJWKSMu.Lock()
	defer codexJWKSMu.Unlock()
	defer func() {
		if err != nil {
			codexJWKSFailedAt, codexJWKSFailedErr, codexJWKSCacheURL = time.Now(), err, codexJWKSURL
		}
	}()

	if codexJWKSCache != nil && codexJWKSCacheURL == codexJWKSURL && time.Since(codexJWKSFetched) < codexJWKSCacheTTL {
		return codexJWKSCache, nil
	}
	if codexJWKSFailedErr != nil && codexJWKSCacheURL == codexJWKSURL && time.Since(codexJWKSFailedAt) < codexJWKSFailureTTL {
		return nil, codexJWKSFailedErr
	}

	// Deliberately detached from the caller: this fills a process-wide cache, so
	// one client hanging up must not cancel the fetch everyone else is queued on.
	reqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, codexJWKSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build JWKS request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("network error fetching OpenAI JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching OpenAI JWKS", resp.StatusCode)
	}

	var parsed codexJWKS
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("failed to parse OpenAI JWKS: %w", err)
	}

	parsedKeys := map[string]*rsa.PublicKey{}
	for _, key := range parsed.Keys {
		if key.Kty != "RSA" || key.Kid == "" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
		if err != nil {
			continue
		}
		parsedKeys[key.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}
	if len(parsedKeys) == 0 {
		return nil, fmt.Errorf("OpenAI JWKS contained no usable RSA keys")
	}

	codexJWKSCache = parsedKeys
	codexJWKSFetched = time.Now()
	codexJWKSCacheURL = codexJWKSURL
	codexJWKSFailedErr = nil
	return parsedKeys, nil
}

// ParseCodexIdentity verifies a Codex id_token against OpenAI's published JWKS
// and returns the account it attests. The signature check is what makes this
// identity trustworthy: a Codex credential can be pasted in by hand
// (import auth.json), so unverified claims would be user-supplied text wearing
// a JWT costume — exactly the thing the Claude self-report form got wrong.
//
// Expiry is deliberately NOT enforced. An id_token lives about an hour, but the
// identity it attests does not expire with it: we want to keep displaying
// "which account is this" for a subscription connected last week. Signature and
// issuer are still required, so the claims remain OpenAI-attested. Liveness of
// the credential is a separate question, answered by the subscription's status.
func ParseCodexIdentity(ctx context.Context, idToken string) (*CodexIdentity, error) {
	if idToken == "" {
		return nil, fmt.Errorf("no id_token")
	}

	keys, err := fetchCodexJWKS(ctx)
	if err != nil {
		return nil, err
	}

	claims := jwt.MapClaims{}
	_, err = jwt.ParseWithClaims(idToken, claims, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		key, ok := keys[kid]
		if !ok {
			return nil, fmt.Errorf("id_token signed by unknown key %q", kid)
		}
		return key, nil
	},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(codexTokenIssuer),
		jwt.WithoutClaimsValidation(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to verify Codex id_token: %w", err)
	}
	// WithoutClaimsValidation also skips the issuer check, so assert it here.
	issuer, _ := claims["iss"].(string)
	if issuer != codexTokenIssuer {
		return nil, fmt.Errorf("id_token issuer %q is not OpenAI", issuer)
	}

	identity := &CodexIdentity{}
	if email, ok := claims["email"].(string); ok {
		identity.AccountEmail = email
	}
	if name, ok := claims["name"].(string); ok {
		identity.AccountName = name
	}
	if auth, ok := claims[codexAuthClaim].(map[string]any); ok {
		if plan, ok := auth["chatgpt_plan_type"].(string); ok {
			identity.PlanType = plan
		}
		if accountID, ok := auth["chatgpt_account_id"].(string); ok {
			identity.AccountID = accountID
		}
	}
	return identity, nil
}
