package main

import (
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
