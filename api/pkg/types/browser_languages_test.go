package types

import "testing"

func TestSanitizeBrowserLanguages(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"en-GB,en", "en-GB,en"},
		{" en-US , en ", "en-US,en"},
		{"en-GB,en;q=0.9,fr;q=0.8", "en-GB,en,fr"},
		{"zh-Hans-CN,zh", "zh-Hans-CN,zh"},
		{"en,EN,en", "en"},
		{"en,\"],\"x\":1,de", "en,de"},
		{"en;rm -rf /,de$(id)", "en"},
		{"a,toolonglang,123,*", ""},
		{"a,b,c,d,e,f,g,h,i,j,k,l", ""},
		{"aa,ab,ac,ad,ae,af,ag,ah,ai,aj,ak", "aa,ab,ac,ad,ae,af,ag,ah,ai,aj"},
	}
	for _, tt := range tests {
		if got := SanitizeBrowserLanguages(tt.in); got != tt.want {
			t.Errorf("SanitizeBrowserLanguages(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
