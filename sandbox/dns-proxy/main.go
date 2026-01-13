// dns-proxy is a simple DNS forwarder for the Helix sandbox.
// It listens on the nested Docker bridge gateway (172.17.0.1:53) and forwards
// queries to the outer Docker's embedded DNS (127.0.0.11:53).
//
// This enables enterprise DNS resolution from dev containers:
//   Dev container → dns-proxy (172.17.0.1) → Docker DNS (127.0.0.11) → host DNS → enterprise DNS
package main

import (
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/miekg/dns"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	listenAddr := flag.String("listen", "172.17.0.1:53", "Address to listen on")
	upstream := flag.String("upstream", "127.0.0.11:53", "Upstream DNS server")
	flag.Parse()

	// Configure logging
	log.Logger = log.Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339})

	handler := &forwarder{upstream: *upstream}

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
}

func (f *forwarder) ServeDNS(w dns.ResponseWriter, r *dns.Msg) {
	c := new(dns.Client)
	c.Timeout = 5 * time.Second

	resp, _, err := c.Exchange(r, f.upstream)
	if err != nil {
		log.Warn().Err(err).Str("upstream", f.upstream).Msg("DNS forward failed")
		// Return SERVFAIL
		msg := new(dns.Msg)
		msg.SetRcode(r, dns.RcodeServerFailure)
		w.WriteMsg(msg)
		return
	}

	if len(r.Question) > 0 {
		log.Debug().
			Str("name", r.Question[0].Name).
			Int("answers", len(resp.Answer)).
			Msg("DNS query forwarded")
	}

	w.WriteMsg(resp)
}
