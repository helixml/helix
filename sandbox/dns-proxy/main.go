// dns-proxy is a simple DNS forwarder for the Helix sandbox.
// It listens on the nested Docker bridge gateway (10.213.0.1:53) and forwards
// queries to the outer Docker's embedded DNS (127.0.0.11:53).
//
// This enables enterprise DNS resolution from dev containers:
//
//	Dev container → dns-proxy (10.213.0.1) → Docker DNS (127.0.0.11) → host DNS → enterprise DNS
//
// It also answers the "outer-api" alias: a desktop on a nested Helix-in-Helix
// dockerd sees an inner compose "api" service that shadows the real outer API.
// "outer-api" lets the desktop reach the outer API unambiguously. Because the
// proxy forwards upstream to the *outer* Docker DNS, rewriting an "outer-api"
// query to "api" and forwarding it resolves to the current outer API IP — and,
// critically, it re-resolves on every query, so a recreated API container is
// picked up automatically (no stale /etc/hosts pin). See #2641.
package main

import (
	"flag"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	listenAddr := flag.String("listen", "172.17.0.1:53", "Address to listen on")
	upstream := flag.String("upstream", "127.0.0.11:53", "Upstream DNS server")
	alias := flag.String("alias", "outer-api=api", "name alias resolved upstream as <from>=<to> (re-resolved every query)")
	flag.Parse()

	// Configure logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	handler := &forwarder{upstream: *upstream}
	if from, to, ok := strings.Cut(*alias, "="); ok && from != "" && to != "" {
		handler.aliasFrom = dns.CanonicalName(from)
		handler.aliasTo = dns.CanonicalName(to)
	}

	server := &dns.Server{
		Addr:    *listenAddr,
		Net:     "udp",
		Handler: handler,
	}

	go func() {
		log.Info().
			Str("listen", *listenAddr).
			Str("upstream", *upstream).
			Msg("Starting DNS proxy")

		if err := server.ListenAndServe(); err != nil {
			log.Fatal().Err(err).Msg("DNS server failed")
		}
	}()

	// Wait for shutdown signal
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig

	log.Info().Msg("Shutting down DNS proxy")
	server.Shutdown()
}

type forwarder struct {
	upstream string
	// aliasFrom/aliasTo implement the "outer-api" -> "api" rewrite (canonical,
	// trailing-dot names). Empty when no alias is configured.
	aliasFrom string
	aliasTo   string
	exchange  func(*dns.Msg, string) (*dns.Msg, error)
	retryWait time.Duration
	retries   int
}

func (f *forwarder) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	c := new(dns.Client)
	c.Timeout = 5 * time.Second

	// Rewrite an aliased question (e.g. outer-api -> api) before forwarding, and
	// remember the original name so we can present the answer under the alias.
	query := r
	aliased := false
	if f.aliasFrom != "" && len(r.Question) > 0 && dns.CanonicalName(r.Question[0].Name) == f.aliasFrom {
		query = r.Copy()
		query.Question[0].Name = f.aliasTo
		aliased = true
	}

	exchange := f.exchange
	if exchange == nil {
		exchange = func(query *dns.Msg, upstream string) (*dns.Msg, error) {
			resp, _, err := c.Exchange(query, upstream)
			return resp, err
		}
	}
	retries := f.retries
	if retries == 0 {
		retries = 20
	}
	retryWait := f.retryWait
	if retryWait == 0 {
		retryWait = 250 * time.Millisecond
	}

	resp, err := exchangeWithAPIAddressRetry(query, f.upstream, exchange, retries, retryWait)
	if err != nil {
		log.Warn().Err(err).Str("upstream", f.upstream).Msg("DNS forward failed")
		// Return SERVFAIL
		msg := new(dns.Msg)
		msg.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(msg)
		return
	}

	// Present the response under the original (aliased) name so the client's
	// resolver accepts it as an answer to what it asked.
	if aliased {
		resp.Question = r.Question
		for _, rr := range resp.Answer {
			if dns.CanonicalName(rr.Header().Name) == f.aliasTo {
				rr.Header().Name = f.aliasFrom
			}
		}
	}

	if len(r.Question) > 0 {
		log.Debug().
			Str("name", r.Question[0].Name).
			Bool("aliased", aliased).
			Int("answers", len(resp.Answer)).
			Msg("DNS query forwarded")
	}

	w.WriteMsg(resp)
}

func exchangeWithAPIAddressRetry(query *dns.Msg, upstream string, exchange func(*dns.Msg, string) (*dns.Msg, error), retries int, retryWait time.Duration) (*dns.Msg, error) {
	resp, err := exchange(query, upstream)
	for attempt := 0; err == nil && shouldRetryAPIAddress(query, resp) && attempt < retries; attempt++ {
		time.Sleep(retryWait)
		resp, err = exchange(query, upstream)
	}
	return resp, err
}

func shouldRetryAPIAddress(query, resp *dns.Msg) bool {
	if len(query.Question) != 1 || resp == nil || resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 0 {
		return false
	}
	q := query.Question[0]
	return dns.CanonicalName(q.Name) == "api." && q.Qtype == dns.TypeA
}
