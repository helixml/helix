package main

import (
	"path/filepath"
	"testing"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
	"github.com/stretchr/testify/require"
)

const testBin = "/usr/lib/chromium/chromium-headless-shell"

func testPolicy() *policy {
	return newPolicy(testBin, "/home/rod", launcher.New().Bin(testBin))
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
	require.NoError(t, testPolicy().check(l))
	require.Equal(t, testBin, l.Get(flags.Bin))
}

func TestPolicyPinsBinaryWhenUnset(t *testing.T) {
	l := clientLauncher()
	l.Delete(flags.Bin)
	require.NoError(t, testPolicy().check(l))
	require.Equal(t, testBin, l.Get(flags.Bin))
}

func TestPolicyRefuses(t *testing.T) {
	prefix := launcher.DefaultUserDataDirPrefix
	cases := map[string]func(l *launcher.Launcher){
		"unknown flag":       func(l *launcher.Launcher) { l.Set("renderer-cmd-prefix", "/bin/sh -c id") },
		"utility prefix":     func(l *launcher.Launcher) { l.Set("utility-cmd-prefix", "x") },
		"positional args":    func(l *launcher.Launcher) { l.Set(flags.Arguments, "file:///etc/passwd") },
		"env":                func(l *launcher.Launcher) { l.Set(flags.Env, "LD_PRELOAD=/tmp/x.so") },
		"xvfb":               func(l *launcher.Launcher) { l.Set(flags.XVFB) },
		"preferences":        func(l *launcher.Launcher) { l.Set(flags.Preferences, "{}") },
		"keep user data":     func(l *launcher.Launcher) { l.Set(flags.KeepUserDataDir) },
		"other binary":       func(l *launcher.Launcher) { l.Bin("/bin/sh") },
		"binary traversal":   func(l *launcher.Launcher) { l.Bin(testBin + "/../../../../bin/sh") },
		"user data outside":  func(l *launcher.Launcher) { l.UserDataDir("/home/rod/.ssh") },
		"user data dotdot":   func(l *launcher.Launcher) { l.UserDataDir(filepath.Join(prefix, "x") + "/../../../../etc") },
		"user data = prefix": func(l *launcher.Launcher) { l.UserDataDir(prefix) },
		"working dir":        func(l *launcher.Launcher) { l.WorkingDir("/") },
		"fixed debug port":   func(l *launcher.Launcher) { l.RemoteDebuggingPort(9222) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			l := clientLauncher()
			mutate(l)
			require.Error(t, testPolicy().check(l))
		})
	}
}
