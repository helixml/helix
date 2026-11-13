// chrome-manager serves go-rod's launcher manager protocol (the same one
// rod-manager serves) for the crawler browser image, Dockerfile.chrome.
//
// Unlike rod-manager it launches only the configured browser binary, with the
// flags the manager itself hands out as defaults plus a short allowlist of
// flags the Helix crawler adds. Anything else (another binary, extra
// environment, positional arguments, a user data dir outside the temp
// prefix, ...) is refused with 400 before a browser starts. When
// CHROME_LAUNCHER_TOKEN is set, a launch must also carry it in the
// Helix-Launcher-Token header (the API sends RAG_CRAWLER_LAUNCHER_TOKEN).
package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
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

// TokenHeader carries the shared secret on launch requests.
const TokenHeader = "Helix-Launcher-Token"

// clientFlags are flags Helix clients add on top of the manager defaults
// (api/pkg/controller/knowledge/browser and launcher.MustNewManaged).
var clientFlags = []flags.Flag{
	"force-webrtc-ip-handling-policy",
	"disable-http2",
}

type policy struct {
	bin        string
	workingDir string
	token      string
	allowed    map[flags.Flag]bool
}

func newPolicy(bin, workingDir, token string, defaults *launcher.Launcher) *policy {
	p := &policy{bin: bin, workingDir: workingDir, token: token, allowed: map[flags.Flag]bool{}}
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

// authorized reports whether a launch request carries the shared secret.
// With no secret configured every request is.
func (p *policy) authorized(r *http.Request) bool {
	if p.token == "" {
		return true
	}
	got := r.Header.Get(TokenHeader)
	return subtle.ConstantTimeCompare([]byte(got), []byte(p.token)) == 1
}

// value returns the single non-empty value of f, whether f is set, and an
// error when it is set without one (go-rod would panic on it).
func value(l *launcher.Launcher, f flags.Flag) (string, bool, error) {
	list, has := l.Flags[f]
	if !has {
		return "", false, nil
	}
	if len(list) == 0 || list[0] == "" {
		return "", true, fmt.Errorf("%s needs a value", f)
	}
	return list[0], true, nil
}

func newUserDataDir() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return filepath.Join(launcher.DefaultUserDataDirPrefix, hex.EncodeToString(b))
}

// check validates a launch request and pins the browser binary, a fresh
// temporary profile when none is given, and a random debugging port.
func (p *policy) check(l *launcher.Launcher) error {
	if l.Flags == nil {
		return errors.New("no launch flags")
	}
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

	bin, has, err := value(l, flags.Bin)
	if err != nil {
		return err
	}
	if has && bin != p.bin {
		return fmt.Errorf("browser binary not allowed: %s", bin)
	}
	l.Flags[flags.Bin] = []string{p.bin}

	dir, has, err := value(l, flags.WorkingDir)
	if err != nil {
		return err
	}
	if has && dir != p.workingDir {
		return fmt.Errorf("working dir not allowed: %s", dir)
	}

	dir, has, err = value(l, flags.UserDataDir)
	if err != nil {
		return err
	}
	if has {
		prefix := launcher.DefaultUserDataDirPrefix + string(filepath.Separator)
		if filepath.Clean(dir) != dir || !strings.HasPrefix(dir, prefix) {
			return fmt.Errorf("user data dir not allowed: %s", dir)
		}
	} else {
		l.Flags[flags.UserDataDir] = []string{newUserDataDir()}
	}

	port, has, err := value(l, flags.RemoteDebuggingPort)
	if err != nil {
		return err
	}
	if has && port != "0" {
		return fmt.Errorf("remote debugging port not allowed: %s", port)
	}
	l.Flags[flags.RemoteDebuggingPort] = []string{"0"}
	return nil
}

// refuse answers 400 and flushes it, so the client sees the reason rather
// than a dropped connection.
func refuse(w http.ResponseWriter, msg string) {
	body := []byte("[chrome-manager] " + msg)
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write(body)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// handler checks launch requests before go-rod's Manager sees them: the
// shared secret, then the launch options, which it rewrites to the
// normalised form (pinned binary, fresh profile). Other requests (the
// defaults the client fetches first) go straight to the Manager.
func (p *policy) handler(m *launcher.Manager, logger *log.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Upgrade") != "websocket" {
			m.ServeHTTP(w, r)
			return
		}
		if !p.authorized(r) {
			logger.Printf("refused launch from %s: missing or wrong %s", r.RemoteAddr, TokenHeader)
			refuse(w, "missing or wrong "+TokenHeader)
			return
		}
		l := m.Defaults(w, r)
		if options := r.Header.Get(string(launcher.HeaderName)); options != "" {
			l.Flags = nil
			if err := json.Unmarshal([]byte(options), l); err != nil {
				refuse(w, "invalid launch options: "+err.Error())
				return
			}
		}
		if err := p.check(l); err != nil {
			logger.Printf("refused launch from %s: %v", r.RemoteAddr, err)
			refuse(w, err.Error())
			return
		}
		r.Header.Set(string(launcher.HeaderName), string(l.JSON()))
		m.ServeHTTP(w, r)
	})
}

// beforeLaunch re-checks what the Manager is about to launch.
func (p *policy) beforeLaunch(logger *log.Logger) func(*launcher.Launcher, http.ResponseWriter, *http.Request) {
	return func(l *launcher.Launcher, w http.ResponseWriter, r *http.Request) {
		err := p.check(l)
		if err == nil {
			return
		}
		logger.Printf("refused launch from %s: %v", r.RemoteAddr, err)
		refuse(w, err.Error())
		// Stops the Manager's handler before it launches a browser, the same
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
	token := os.Getenv("CHROME_LAUNCHER_TOKEN")
	if token == "" {
		logger.Println("[chrome-manager] WARNING: CHROME_LAUNCHER_TOKEN is not set; any client that can reach this port can launch a browser")
	}

	defaults := func() *launcher.Launcher { return launcher.New().Bin(*bin) }
	p := newPolicy(*bin, wd, token, defaults())

	m := launcher.NewManager()
	m.Logger = logger
	m.Defaults = func(http.ResponseWriter, *http.Request) *launcher.Launcher { return defaults() }
	m.BeforeLaunch = p.beforeLaunch(logger)

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatal(err)
	}
	logger.Println("[chrome-manager] listening on:", ln.Addr().String())
	srv := &http.Server{Handler: p.handler(m, logger)}
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
