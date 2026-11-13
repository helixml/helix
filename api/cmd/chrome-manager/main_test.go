package main

import (
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/stretchr/testify/require"
)

const testBin = "/usr/lib/chromium/chromium-headless-shell"

func testPolicy(token string) *policy {
	return newPolicy(testBin, "/home/rod", token, launcher.New().Bin(testBin))
}

// clientLauncher mirrors what launcher.NewManaged builds from the manager's
// defaults, plus the crawler's own flag.
func clientLauncher() *launcher.Launcher {
	l := launcher.New().Bin(testBin)
	l.Set("force-webrtc-ip-handling-policy", "disable_non_proxied_udp")
	return l
}

func TestPolicyAllowsCrawlerLaunch(t *testing.T) {
	l := clientLauncher()
	require.NoError(t, testPolicy("").check(l))
	require.Equal(t, testBin, l.Get(flags.Bin))
}

func TestPolicyFillsUnsetBinaryProfileAndPort(t *testing.T) {
	l := clientLauncher()
	l.Delete(flags.Bin)
	l.Delete(flags.UserDataDir)
	l.Delete(flags.RemoteDebuggingPort)
	require.NoError(t, testPolicy("").check(l))
	require.Equal(t, testBin, l.Get(flags.Bin))
	require.Equal(t, "0", l.Get(flags.RemoteDebuggingPort))
	dir := l.Get(flags.UserDataDir)
	require.True(t, strings.HasPrefix(dir, launcher.DefaultUserDataDirPrefix+string(filepath.Separator)), dir)

	other := clientLauncher()
	other.Delete(flags.UserDataDir)
	require.NoError(t, testPolicy("").check(other))
	require.NotEqual(t, dir, other.Get(flags.UserDataDir), "each launch gets its own profile")
}

func TestPolicyRefuses(t *testing.T) {
	prefix := launcher.DefaultUserDataDirPrefix
	cases := map[string]func(l *launcher.Launcher){
		"nil flags":            func(l *launcher.Launcher) { l.Flags = nil },
		"unknown flag":         func(l *launcher.Launcher) { l.Set("renderer-cmd-prefix", "/bin/sh -c id") },
		"utility prefix":       func(l *launcher.Launcher) { l.Set("utility-cmd-prefix", "x") },
		"positional args":      func(l *launcher.Launcher) { l.Set(flags.Arguments, "file:///etc/passwd") },
		"env":                  func(l *launcher.Launcher) { l.Set(flags.Env, "LD_PRELOAD=/tmp/x.so") },
		"xvfb":                 func(l *launcher.Launcher) { l.Set(flags.XVFB) },
		"preferences":          func(l *launcher.Launcher) { l.Set(flags.Preferences, "{}") },
		"keep user data":       func(l *launcher.Launcher) { l.Set(flags.KeepUserDataDir) },
		"other binary":         func(l *launcher.Launcher) { l.Bin("/bin/sh") },
		"binary traversal":     func(l *launcher.Launcher) { l.Bin(testBin + "/../../../../bin/sh") },
		"empty binary":         func(l *launcher.Launcher) { l.Flags[flags.Bin] = nil },
		"blank binary":         func(l *launcher.Launcher) { l.Flags[flags.Bin] = []string{""} },
		"user data outside":    func(l *launcher.Launcher) { l.UserDataDir("/home/rod/.ssh") },
		"user data dotdot":     func(l *launcher.Launcher) { l.UserDataDir(filepath.Join(prefix, "x") + "/../../../../etc") },
		"user data = prefix":   func(l *launcher.Launcher) { l.UserDataDir(prefix) },
		"empty user data":      func(l *launcher.Launcher) { l.Flags[flags.UserDataDir] = []string{} },
		"working dir":          func(l *launcher.Launcher) { l.WorkingDir("/") },
		"empty working dir":    func(l *launcher.Launcher) { l.Flags[flags.WorkingDir] = nil },
		"fixed debug port":     func(l *launcher.Launcher) { l.RemoteDebuggingPort(9222) },
		"empty debugging port": func(l *launcher.Launcher) { l.Flags[flags.RemoteDebuggingPort] = nil },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			l := clientLauncher()
			mutate(l)
			require.Error(t, testPolicy("").check(l))
		})
	}
}

// launchRequest is a websocket upgrade like launcher.Launcher.Client sends.
func launchRequest(options, token string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Upgrade", "websocket")
	r.Header.Set("Connection", "Upgrade")
	if options != "" {
		r.Header.Set(string(launcher.HeaderName), options)
	}
	if token != "" {
		r.Header.Set(TokenHeader, token)
	}
	return r
}

func testHandler(p *policy) http.Handler {
	m := launcher.NewManager()
	m.Defaults = func(http.ResponseWriter, *http.Request) *launcher.Launcher { return launcher.New().Bin(testBin) }
	m.BeforeLaunch = p.beforeLaunch(log.New(io.Discard, "", 0))
	return p.handler(m, log.New(io.Discard, "", 0))
}

func TestHandlerRefusesWithFlushed400(t *testing.T) {
	bad := clientLauncher()
	bad.Set("renderer-cmd-prefix", "id")
	cases := map[string]struct {
		policy  *policy
		options string
		token   string
		want    string
	}{
		"missing token":     {testPolicy("s3cret"), string(clientLauncher().JSON()), "", "missing or wrong " + TokenHeader},
		"wrong token":       {testPolicy("s3cret"), string(clientLauncher().JSON()), "nope", "missing or wrong " + TokenHeader},
		"bad flag":          {testPolicy("s3cret"), string(bad.JSON()), "s3cret", "flags not allowed: renderer-cmd-prefix"},
		"null flags":        {testPolicy(""), `{"flags":null}`, "", "no launch flags"},
		"empty port":        {testPolicy(""), `{"flags":{"remote-debugging-port":[]}}`, "", "remote-debugging-port needs a value"},
		"empty bin":         {testPolicy(""), `{"flags":{"rod-bin":[]}}`, "", "rod-bin needs a value"},
		"empty profile dir": {testPolicy(""), `{"flags":{"user-data-dir":[""]}}`, "", "user-data-dir needs a value"},
		"invalid json":      {testPolicy(""), `{"flags":`, "", "invalid launch options"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			require.NotPanics(t, func() { testHandler(tc.policy).ServeHTTP(rec, launchRequest(tc.options, tc.token)) })
			require.Equal(t, http.StatusBadRequest, rec.Code)
			require.True(t, rec.Flushed, "refusal must be flushed")
			require.Contains(t, rec.Body.String(), tc.want)
		})
	}
}

func TestHandlerServesDefaultsWithoutToken(t *testing.T) {
	rec := httptest.NewRecorder()
	testHandler(testPolicy("s3cret")).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), testBin)
}

func TestBeforeLaunchFlushesBeforeAborting(t *testing.T) {
	l := clientLauncher()
	l.Set("renderer-cmd-prefix", "id")
	rec := httptest.NewRecorder()
	require.PanicsWithValue(t, http.ErrAbortHandler, func() {
		testPolicy("").beforeLaunch(log.New(io.Discard, "", 0))(l, rec, launchRequest("", ""))
	})
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.True(t, rec.Flushed)
	require.Contains(t, rec.Body.String(), "renderer-cmd-prefix")
}
