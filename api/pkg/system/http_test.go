package system

import "testing"

func TestHas5xxStatusCode(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "500", message: "status code 500", want: true},
		{name: "524", message: "status code 524: timeout", want: true},
		{name: "missing marker", message: "524: timeout", want: false},
		{name: "401", message: "status code 401: unauthorized", want: false},
		{name: "5000", message: "status code 5000", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Has5xxStatusCode(tt.message); got != tt.want {
				t.Fatalf("Has5xxStatusCode(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}
