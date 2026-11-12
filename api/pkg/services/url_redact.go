package services

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// RedactURLCredentials returns rawURL with any userinfo (username and
// password, e.g. an API key embedded in an authenticated clone URL) removed,
// for use in logs and error messages. Unparseable input is replaced entirely
// rather than returned as-is, since it may still carry a secret.
func RedactURLCredentials(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "[unparseable URL redacted]"
	}
	if parsed.User == nil {
		return parsed.String()
	}
	parsed.User = nil
	return parsed.String()
}

// nonSecretURLUsernames are usernames that conventionally carry no secret
// when a URL has no password (e.g. git@host).
var nonSecretURLUsernames = map[string]bool{"git": true, "oauth2": true, "x-access-token": true, "api": true}

// urlSecrets returns the credential parts of rawURL's userinfo: the password,
// or the username when it is the only credential (a token used as username).
func urlSecrets(rawURL string) []string {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.User == nil {
		return nil
	}
	if password, ok := parsed.User.Password(); ok {
		if password == "" {
			return nil
		}
		return []string{password}
	}
	if username := parsed.User.Username(); username != "" && !nonSecretURLUsernames[strings.ToLower(username)] {
		return []string{username}
	}
	return nil
}

// redactedError carries a redacted message while keeping the original error
// in the chain for errors.Is / errors.As.
type redactedError struct {
	msg string
	err error
}

func (e *redactedError) Error() string { return e.msg }
func (e *redactedError) Unwrap() error { return e.err }

// redactURLSecretInError rewrites err's message so it does not leak the
// credentials of rawURL: every occurrence of rawURL is replaced with its
// RedactURLCredentials form, and any remaining copy of its password (or
// token-as-username) becomes REDACTED. err is returned unchanged when its
// message contains neither.
func redactURLSecretInError(err error, rawURL string) error {
	if err == nil || rawURL == "" {
		return err
	}
	orig := err.Error()
	msg := strings.ReplaceAll(orig, rawURL, RedactURLCredentials(rawURL))
	for _, secret := range urlSecrets(rawURL) {
		msg = strings.ReplaceAll(msg, secret, "REDACTED")
		if escaped := url.PathEscape(secret); escaped != secret {
			msg = strings.ReplaceAll(msg, escaped, "REDACTED")
		}
	}
	if msg == orig {
		return err
	}
	return &redactedError{msg: msg, err: err}
}

// redactURLParseError drops the URL that *url.Error embeds in its message,
// keeping only the operation and the underlying parse error.
func redactURLParseError(err error) error {
	var uerr *url.Error
	if errors.As(err, &uerr) {
		return fmt.Errorf("%s URL: %w", uerr.Op, uerr.Err)
	}
	return err
}
