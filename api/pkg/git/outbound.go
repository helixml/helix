package git

import (
	"net/url"
	"os"

	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"strings"

	"golang.org/x/net/http/httpproxy"

	"github.com/helixml/helix/api/pkg/netguard"
)

// AllowedProtocols are the transports git may use for a user-supplied
// remote. Set as GIT_ALLOW_PROTOCOL, it also covers redirects and submodules,
// so local paths, file:// and ext:: (which runs a command) are refused.
const AllowedProtocols = "http:https"

// ResolvePin resolves rawURL's host under policy and returns an
// http.curloptResolve value ("host:port:addr[,addr...]") that pins git to the
// approved addresses, so it cannot re-resolve the host to another address.
// It returns "" when there is nothing to pin: the host is an IP literal, or
// it does not resolve locally (left to an egress proxy). The error wraps
// netguard.ErrBlockedDestination when the URL is not http(s) or the policy
// refuses the destination.
func ResolvePin(ctx context.Context, policy netguard.Policy, rawURL string) (string, error) {
	u, err := netguard.ValidateHTTPURL(rawURL)
	if err != nil {
		if !errors.Is(err, netguard.ErrBlockedDestination) {
			err = fmt.Errorf("%w: %v", netguard.ErrBlockedDestination, err)
		}
		return "", err
	}
	ips, err := policy.ResolveHost(ctx, u.Hostname())
	if errors.Is(err, netguard.ErrBlockedDestination) {
		return "", err
	}
	if err != nil {
		// git would resolve the name itself, and a resolver can answer it
		// differently than it answered us. That is only acceptable when git
		// goes through an operator proxy, which then resolves and controls
		// egress; otherwise an unresolvable name is refused.
		if usesProxy(u) {
			return "", nil
		}
		return "", fmt.Errorf("%w: cannot resolve %s: %v", netguard.ErrBlockedDestination, u.Hostname(), err)
	}
	if net.ParseIP(u.Hostname()) != nil {
		return "", nil
	}
	port := u.Port()
	if port == "" {
		port = "80"
		if u.Scheme == "https" {
			port = "443"
		}
	}
	addrs := make([]string, 0, len(ips))
	for _, ip := range ips {
		if ip.To4() == nil {
			addrs = append(addrs, "["+ip.String()+"]")
		} else {
			addrs = append(addrs, ip.String())
		}
	}
	return u.Hostname() + ":" + port + ":" + strings.Join(addrs, ","), nil
}

// usesProxy reports whether git would reach u through a proxy configured in
// the environment (http_proxy, https_proxy, all_proxy and no_proxy, as git
// and curl read them).
func usesProxy(u *url.URL) bool {
	cfg := httpproxy.FromEnvironment()
	if cfg.HTTPProxy == "" && cfg.HTTPSProxy == "" {
		for _, key := range []string{"ALL_PROXY", "all_proxy"} {
			if v := os.Getenv(key); v != "" {
				cfg.HTTPProxy, cfg.HTTPSProxy = v, v
				break
			}
		}
	}
	proxyURL, err := cfg.ProxyFunc()(u)
	return err == nil && proxyURL != nil
}

// configEnv encodes git config entries as GIT_CONFIG_* environment
// variables, which keeps values such as credentials out of argv.
func configEnv(cfg [][2]string) []string {
	env := []string{fmt.Sprintf("GIT_CONFIG_COUNT=%d", len(cfg))}
	for i, kv := range cfg {
		env = append(env,
			fmt.Sprintf("GIT_CONFIG_KEY_%d=%s", i, kv[0]),
			fmt.Sprintf("GIT_CONFIG_VALUE_%d=%s", i, kv[1]),
		)
	}
	return env
}

// basicAuthHeader returns an http.extraHeader value for username/password.
func basicAuthHeader(username, password string) string {
	return "Authorization: Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}
