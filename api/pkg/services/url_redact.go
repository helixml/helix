package services

import (
	"errors"
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

// redactURLSecretInError removes the password of rawURL from err's message, so
// an error that echoes an authenticated URL does not leak the credential. The
// original error is returned unchanged when it does not contain the secret.
func redactURLSecretInError(err error, rawURL string) error {
	if err == nil {
		return nil
	}
	parsed, perr := url.Parse(rawURL)
	if perr != nil || parsed.User == nil {
		return err
	}
	secret, ok := parsed.User.Password()
	if !ok || secret == "" || !strings.Contains(err.Error(), secret) {
		return err
	}
	msg := strings.ReplaceAll(err.Error(), rawURL, RedactURLCredentials(rawURL))
	msg = strings.ReplaceAll(msg, secret, "REDACTED")
	return errors.New(msg)
}
