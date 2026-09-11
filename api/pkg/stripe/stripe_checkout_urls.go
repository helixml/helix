package stripe

import (
	"fmt"
	"net/url"
	"strings"
)

func checkoutReturnURLs(appURL, returnURL, defaultSuccessURL, defaultCancelURL string) (string, string, error) {
	if returnURL == "" {
		return defaultSuccessURL, defaultCancelURL, nil
	}
	parsed, err := ValidateCheckoutReturnURL(returnURL)
	if err != nil {
		return "", "", err
	}
	query := parsed.Query()
	query.Del("success")
	query.Del("canceled")
	query.Del("session_id")
	parsed.RawQuery = query.Encode()

	success := *parsed
	success.RawQuery = appendRawQuery(success.RawQuery, "success=true&session_id={CHECKOUT_SESSION_ID}")
	canceled := *parsed
	canceled.RawQuery = appendRawQuery(canceled.RawQuery, "canceled=true")

	return appURL + success.String(), appURL + canceled.String(), nil
}

func ValidateCheckoutReturnURL(returnURL string) (*url.URL, error) {
	if returnURL == "" {
		return nil, nil
	}
	if !strings.HasPrefix(returnURL, "/") ||
		strings.HasPrefix(returnURL, "//") ||
		strings.ContainsAny(returnURL, `\#`) {
		return nil, fmt.Errorf("return URL must be an app-relative path")
	}

	parsed, err := url.ParseRequestURI(returnURL)
	if err != nil || parsed.IsAbs() || parsed.Host != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("return URL must be an app-relative path")
	}
	return parsed, nil
}

func appendRawQuery(existing, added string) string {
	if existing == "" {
		return added
	}
	return existing + "&" + added
}
