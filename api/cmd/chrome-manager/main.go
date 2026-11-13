// chrome-manager serves go-rod's launcher manager protocol (the same one
// rod-manager serves) for the crawler browser image, Dockerfile.chrome.
//
// Unlike rod-manager it launches only the configured browser binary, with the
// flags the manager itself hands out as defaults plus a short allowlist of
// flags the Helix crawler adds. Anything else (another binary, extra
// environment, positional arguments, a user data dir outside the temp
// prefix, ...) is refused before a browser starts.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/launcher/flags"
)

// clientFlags are flags Helix clients add on top of the manager defaults
// (api/pkg/controller/knowledge/browser and launcher.MustNewManaged).
var clientFlags = []flags.Flag{
	"force-webrtc-ip-handling-policy",
	"disable-http2",
}

type policy struct {
	bin        string
	workingDir string
	allowed    map[flags.Flag]bool
}

func newPolicy(bin, workingDir string, defaults *launcher.Launcher) *policy {
	p := &policy{bin: bin, workingDir: workingDir, allowed: map[flags.Flag]bool{}}
	for f := range defaults.Flags {
		p.allowed[f] = true
	}
	for _, f := range clientFlags {
		p.allowed[f] = true
	}
	// Positional arguments, extra environment, xvfb-run, preferences files
	// and a kept profile are never needed by the crawler.
	for _, f := range []flags.Flag{flags.Arguments, flags.Env, flags.XVFB, flags.Preferences, flags.KeepUserDataDir, flags.App} {
		delete(p.allowed, f)
	}
	return p
}

// check validates a launch request and pins the browser binary.
func (p *policy) check(l *launcher.Launcher) error {
	var refused []string
	for f := range l.Flags {
		if !p.allowed[f] {
			refused = append(refused, string(f))
		}
	}
	if len(refused) > 0 {
		sort.Strings(refused)
		return fmt.Errorf("flags not allowed: %s", strings.Join(refused, ", "))
	}

	if bin := l.Get(flags.Bin); bin != "" && bin != p.bin {
		return fmt.Errorf("browser binary not allowed: %s", bin)
	}
	l.Bin(p.bin)

	if dir := l.Get(flags.WorkingDir); dir != "" && dir != p.workingDir {
		return fmt.Errorf("working dir not allowed: %s", dir)
	}

	if dir := l.Get(flags.UserDataDir); dir != "" {
		prefix := launcher.DefaultUserDataDirPrefix + string(filepath.Separator)
		if filepath.Clean(dir) != dir || !strings.HasPrefix(dir, prefix) {
			return fmt.Errorf("user data dir not allowed: %s", dir)
		}
	}

	if port := l.Get(flags.RemoteDebuggingPort); port != "" && port != "0" {
		return fmt.Errorf("remote debugging port not allowed: %s", port)
	}
	return nil
}

func (p *policy) beforeLaunch(logger *log.Logger) func(*launcher.Launcher, http.ResponseWriter, *http.Request) {
	return func(l *launcher.Launcher, w http.ResponseWriter, r *http.Request) {
		err := p.check(l)
		if err == nil {
			return
		}
		logger.Printf("refused launch from %s: %v", r.RemoteAddr, err)
		http.Error(w, "[chrome-manager] "+err.Error(), http.StatusBadRequest)
		// Stops the manager's handler before it launches a browser, the same
		// way rod-manager's own path check does.
		panic(http.ErrAbortHandler)
	}
}

func main() {
	addr := flag.String("address", ":7317", "the address to listen on")
	bin := flag.String("bin", "/usr/lib/chromium/chromium-headless-shell", "the browser binary to launch")
	flag.Parse()

	if _, err := os.Stat(*bin); err != nil {
		log.Fatalf("browser binary: %v", err)
	}
	wd, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	logger := log.New(os.Stdout, "", 0)
	defaults := func() *launcher.Launcher { return launcher.New().Bin(*bin) }
	p := newPolicy(*bin, wd, defaults())

	m := launcher.NewManager()
	m.Logger = logger
	m.Defaults = func(http.ResponseWriter, *http.Request) *launcher.Launcher { return defaults() }
	m.BeforeLaunch = p.beforeLaunch(logger)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	logger.Println("[chrome-manager] listening on:", ln.Addr().String())
	srv := &http.Server{Handler: m}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
