package main

import (
	"net"
	"testing"
	"time"

	"github.com/miekg/dns"
)

func TestExchangeRetriesEmptyAPIAddress(t *testing.T) {
	calls := 0
	exchange := func(_ *dns.Msg, _ string) (*dns.Msg, error) {
		calls++
		if calls < 3 {
			return response(dns.RcodeSuccess), nil
		}
		return responseWithA(), nil
	}

	resp, err := exchangeWithAPIAddressRetry(query("api.", dns.TypeA), "upstream", exchange, 4, time.Nanosecond)
	if err != nil {
		t.Fatalf("exchangeWithAPIAddressRetry: %v", err)
	}
	if calls != 3 {
		t.Fatalf("exchange calls = %d, want 3", calls)
	}
	if len(resp.Answer) != 1 {
		t.Fatalf("answers = %d, want 1", len(resp.Answer))
	}
}

func TestShouldRetryAPIAddress(t *testing.T) {
	tests := []struct {
		name  string
		query *dns.Msg
		resp  *dns.Msg
		want  bool
	}{
		{name: "empty api A response", query: query("api.", dns.TypeA), resp: response(dns.RcodeSuccess), want: true},
		{name: "api A address", query: query("api.", dns.TypeA), resp: responseWithA(), want: false},
		{name: "api AAAA response", query: query("api.", dns.TypeAAAA), resp: response(dns.RcodeSuccess), want: false},
		{name: "external A response", query: query("example.com.", dns.TypeA), resp: response(dns.RcodeSuccess), want: false},
		{name: "api NXDOMAIN", query: query("api.", dns.TypeA), resp: response(dns.RcodeNameError), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRetryAPIAddress(tt.query, tt.resp); got != tt.want {
				t.Fatalf("shouldRetryAPIAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func query(name string, qtype uint16) *dns.Msg {
	m := new(dns.Msg)
	m.SetQuestion(name, qtype)
	return m
}

func response(rcode int) *dns.Msg {
	return &dns.Msg{MsgHdr: dns.MsgHdr{Rcode: rcode}}
}

func responseWithA() *dns.Msg {
	m := response(dns.RcodeSuccess)
	m.Answer = []dns.RR{&dns.A{Hdr: dns.RR_Header{Name: "api.", Rrtype: dns.TypeA, Class: dns.ClassINET}, A: []byte{10, 0, 0, 1}}}
	return m
}

// paddedResponse mimics a resolver that stuffs every reply with root NS records
// and glue, pushing it past 512 bytes even though the answers alone fit.
func paddedResponse(q *dns.Msg, answers int) *dns.Msg {
	m := new(dns.Msg)
	m.SetReply(q)
	for i := 0; i < answers; i++ {
		m.Answer = append(m.Answer, &dns.A{Hdr: dns.RR_Header{Name: q.Question[0].Name, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60}, A: []byte{104, 16, byte(i), 34}})
	}
	for _, l := range "abcdefghijklm" {
		ns := string(l) + ".root-servers.net."
		m.Ns = append(m.Ns, &dns.NS{Hdr: dns.RR_Header{Name: ".", Rrtype: dns.TypeNS, Class: dns.ClassINET, Ttl: 3600}, Ns: ns})
		m.Extra = append(m.Extra, &dns.AAAA{Hdr: dns.RR_Header{Name: ns, Rrtype: dns.TypeAAAA, Class: dns.ClassINET, Ttl: 3600}, AAAA: make([]byte, 16)})
	}
	return m
}

func TestFitResponse(t *testing.T) {
	edns := func(size uint16) *dns.Msg {
		q := query("registry.npmjs.org.", dns.TypeA)
		q.SetEdns0(size, false)
		return q
	}
	tests := []struct {
		name        string
		req         *dns.Msg
		answers     int
		tcp         bool
		maxLen      int
		wantAnswers int
		wantTC      bool
	}{
		{name: "no EDNS drops padding, keeps answers", req: query("registry.npmjs.org.", dns.TypeA), answers: 12, maxLen: 512, wantAnswers: 12},
		{name: "no EDNS answers too big sets TC", req: query("registry.npmjs.org.", dns.TypeA), answers: 40, maxLen: 512, wantTC: true},
		{name: "EDNS 4096 keeps everything", req: edns(4096), answers: 40, maxLen: 4096, wantAnswers: 40},
		{name: "TCP keeps everything", req: query("registry.npmjs.org.", dns.TypeA), answers: 40, tcp: true, maxLen: dns.MaxMsgSize, wantAnswers: 40},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := paddedResponse(tt.req, tt.answers)
			fitResponse(tt.req, resp, tt.tcp)
			if resp.Len() > tt.maxLen {
				t.Fatalf("len = %d, want <= %d", resp.Len(), tt.maxLen)
			}
			if resp.Truncated != tt.wantTC {
				t.Fatalf("TC = %v, want %v", resp.Truncated, tt.wantTC)
			}
			if !tt.wantTC && len(resp.Answer) != tt.wantAnswers {
				t.Fatalf("answers = %d, want %d", len(resp.Answer), tt.wantAnswers)
			}
		})
	}
}

// TestServeDNSPaddedUpstream runs the proxy end to end: a strict client with
// a plain 512-byte buffer (as Docker's resolver uses) must get an answer, not
// "buffer size too small", when upstream returns a padded 700+ byte reply.
func TestServeDNSPaddedUpstream(t *testing.T) {
	upstream := startServer(t, "udp", dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		w.WriteMsg(paddedResponse(r, 12))
	}))
	proxy := startServer(t, "udp", &forwarder{upstream: upstream})

	resp, _, err := (&dns.Client{}).Exchange(query("registry.npmjs.org.", dns.TypeA), proxy)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if resp.Rcode != dns.RcodeSuccess || len(resp.Answer) != 12 {
		t.Fatalf("rcode = %d answers = %d, want success with 12", resp.Rcode, len(resp.Answer))
	}
}

// TestServeDNSTruncatedUpstreamRetriesTCP checks that a TC reply from upstream
// is re-fetched over TCP rather than relayed with its answers missing.
func TestServeDNSTruncatedUpstreamRetriesTCP(t *testing.T) {
	addr := "127.0.0.1:" + freePort(t)
	startServerOn(t, addr, "udp", dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		m := new(dns.Msg)
		m.SetReply(r)
		m.Truncated = true
		w.WriteMsg(m)
	}))
	startServerOn(t, addr, "tcp", dns.HandlerFunc(func(w dns.ResponseWriter, r *dns.Msg) {
		w.WriteMsg(paddedResponse(r, 3))
	}))
	proxy := startServer(t, "udp", &forwarder{upstream: addr})

	resp, _, err := (&dns.Client{}).Exchange(query("example.com.", dns.TypeA), proxy)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	if len(resp.Answer) != 3 || resp.Truncated {
		t.Fatalf("answers = %d TC = %v, want 3 answers untruncated", len(resp.Answer), resp.Truncated)
	}
}

func freePort(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	_, port, _ := net.SplitHostPort(l.Addr().String())
	return port
}

func startServer(t *testing.T, network string, h dns.Handler) string {
	t.Helper()
	addr := "127.0.0.1:" + freePort(t)
	startServerOn(t, addr, network, h)
	return addr
}

func startServerOn(t *testing.T, addr, network string, h dns.Handler) {
	t.Helper()
	started := make(chan struct{})
	s := &dns.Server{Addr: addr, Net: network, Handler: h, NotifyStartedFunc: func() { close(started) }}
	go s.ListenAndServe()
	<-started
	t.Cleanup(func() { s.Shutdown() })
}
