package auth

import "testing"

func TestEmailDomainAllowed(t *testing.T) {
	helix := []string{"helix.ml"}
	multi := []string{"helix.ml", "example.com"}

	cases := []struct {
		name     string
		email    string
		verified bool
		allowed  []string
		want     bool
	}{
		{"empty list allows anything", "anyone@gmail.com", true, nil, true},
		{"empty list allows unverified", "anyone@gmail.com", false, nil, true},
		{"verified match", "luke@helix.ml", true, helix, true},
		{"verified match case-insensitive", "Luke@Helix.ML", true, helix, true},
		{"unverified match rejected", "luke@helix.ml", false, helix, false},
		{"wrong domain rejected", "someone@gmail.com", true, helix, false},
		{"spoofed subdomain rejected", "evil@helix.ml.evil.com", true, helix, false},
		{"subdomain of allowed rejected", "evil@sub.helix.ml", true, helix, false},
		{"no at-sign rejected", "notanemail", true, helix, false},
		{"trailing at rejected", "user@", true, helix, false},
		{"second domain in list matches", "person@example.com", true, multi, true},
		{"empty email with restriction rejected", "", true, helix, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := emailDomainAllowed(tc.email, tc.verified, tc.allowed); got != tc.want {
				t.Errorf("emailDomainAllowed(%q, %v, %v) = %v, want %v",
					tc.email, tc.verified, tc.allowed, got, tc.want)
			}
		})
	}
}

func TestParseEmailDomains(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", nil},
		{"   ", nil},
		{"helix.ml", []string{"helix.ml"}},
		{"helix.ml, example.com", []string{"helix.ml", "example.com"}},
		{" Helix.ML , , EXAMPLE.com ", []string{"helix.ml", "example.com"}},
	}

	for _, tc := range cases {
		got := ParseEmailDomains(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("ParseEmailDomains(%q) = %v, want %v", tc.in, got, tc.want)
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Fatalf("ParseEmailDomains(%q) = %v, want %v", tc.in, got, tc.want)
			}
		}
	}
}
