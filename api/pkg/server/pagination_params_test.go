package server

import (
	"net/url"
	"testing"
)

func TestQueryIntAtLeast(t *testing.T) {
	cases := []struct {
		name     string
		raw      string
		min, def int
		want     int
	}{
		{"missing", "", 1, 50, 50},
		{"valid", "page=3", 1, 1, 3},
		{"below min", "page=0", 1, 1, 1},
		{"zero allowed", "page=0", 0, 0, 0},
		{"negative", "page=-2", 0, 0, 0},
		{"malformed", "page=abc", 1, 7, 7},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			values, err := url.ParseQuery(tc.raw)
			if err != nil {
				t.Fatal(err)
			}
			if got := queryIntAtLeast(values, "page", tc.min, tc.def); got != tc.want {
				t.Fatalf("queryIntAtLeast(%q) = %d, want %d", tc.raw, got, tc.want)
			}
		})
	}
}
