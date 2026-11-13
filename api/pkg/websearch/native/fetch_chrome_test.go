package native

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-rod/rod/lib/proto"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/controller/knowledge/browser"
	"github.com/helixml/helix/api/pkg/netguard"
	"github.com/helixml/helix/api/pkg/websearch"
)

// TestRenderConfinedToEgressProxy drives a real Chrome through the egress
// proxy under the strict policy. It needs a rod manager and an address on
// this host that Chrome can reach, e.g. with the dev stack:
//
//	WEBSEARCH_TEST_ROD_MANAGER=ws://172.19.0.2:7317 WEBSEARCH_TEST_HOST_IP=172.19.0.1
func TestRenderConfinedToEgressProxy(t *testing.T) {
	manager, hostIP := os.Getenv("WEBSEARCH_TEST_ROD_MANAGER"), os.Getenv("WEBSEARCH_TEST_HOST_IP")
	if manager == "" || hostIP == "" {
		t.Skip("set WEBSEARCH_TEST_ROD_MANAGER and WEBSEARCH_TEST_HOST_IP to run against a real Chrome")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// A private origin Chrome could reach directly; it must never be hit.
	var hits atomic.Int32
	originLn, err := net.Listen("tcp", net.JoinHostPort(hostIP, "0"))
	if err != nil {
		t.Fatal(err)
	}
	origin := &http.Server{Handler: http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) })}
	go func() { _ = origin.Serve(originLn) }()
	defer origin.Close()

	proxy, err := netguard.NewProxy(netguard.Policy{})
	if err != nil {
		t.Fatal(err)
	}
	proxyLn, err := net.Listen("tcp", net.JoinHostPort(hostIP, "0"))
	if err != nil {
		t.Fatal(err)
	}
	go func() { _ = proxy.Serve(ctx, proxyLn) }()
	user, pass := proxy.Credentials()

	cfg := &config.ServerConfig{}
	cfg.RAG.Crawler.LauncherEnabled = true
	cfg.RAG.Crawler.LauncherURL = manager
	cfg.RAG.Crawler.BrowserPoolSize = 1
	b, err := browser.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	f, err := NewFetcher(FetcherOptions{Browser: b, ProxyURL: "http://" + proxyLn.Addr().String(), ProxyUsername: user, ProxyPassword: pass})
	if err != nil {
		t.Fatal(err)
	}
	browserContexts := func() int {
		br, err := b.GetBrowser(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = b.PutBrowser(br) }()
		res, err := proto.TargetGetBrowserContexts{}.Call(br)
		if err != nil {
			t.Fatal(err)
		}
		return len(res.BrowserContextIDs)
	}
	before := browserContexts()

	for _, u := range []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://localhost:7317/",
		"http://127.0.0.1:7317/",
		"http://" + originLn.Addr().String() + "/",
	} {
		_, err := f.render(ctx, u)
		var fe *FetchError
		if !errors.As(err, &fe) || fe.Tag != websearch.TagBlockedDestination {
			t.Errorf("render(%s) err = %v, want %s", u, err, websearch.TagBlockedDestination)
		}
	}
	// A refused CONNECT surfaces as a navigation failure, but is still refused.
	if _, err := f.render(ctx, "https://169.254.169.254/"); err == nil {
		t.Error("render(https://169.254.169.254/) succeeded")
	}
	if n := hits.Load(); n != 0 {
		t.Errorf("private origin was hit %d times", n)
	}
	time.Sleep(500 * time.Millisecond) // disposal is asynchronous in Chrome
	if after := browserContexts(); after != before {
		t.Errorf("browser contexts: %d before, %d after; a render leaked its context", before, after)
	}
}
