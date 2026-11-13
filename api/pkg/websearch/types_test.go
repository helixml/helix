package websearch

import (
	"encoding/json"
	"testing"
)

func TestWarningPathIsJSONPointerOnTheWire(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"", `""`},
		{"query", `"/query"`},
		{"contents.highlights.highlightsPerUrl", `"/contents/highlights/highlightsPerUrl"`},
		{"additionalQueries[2]", `"/additionalQueries/2"`},
		{"/already/a/pointer", `"/already/a/pointer"`},
	} {
		b, err := json.Marshal(Warning{Code: WarningIgnoredField, Path: tc.path, Message: "m"})
		if err != nil {
			t.Fatal(err)
		}
		want := `{"code":"IGNORED_FIELD","path":` + tc.want + `,"message":"m"}`
		if string(b) != want {
			t.Errorf("path %q: got %s, want %s", tc.path, b, want)
		}
	}
}

func TestSummaryAcceptsBoolOrObject(t *testing.T) {
	var o ContentsOptions
	if err := json.Unmarshal([]byte(`{"summary":true}`), &o); err != nil || !o.Summary.Requested() {
		t.Fatalf("summary true: %v %+v", err, o.Summary)
	}
	o = ContentsOptions{}
	if err := json.Unmarshal([]byte(`{"summary":{"query":"q"}}`), &o); err != nil || !o.Summary.Requested() || o.Summary.Query != "q" {
		t.Fatalf("summary object: %v %+v", err, o.Summary)
	}
	o = ContentsOptions{}
	if err := json.Unmarshal([]byte(`{"summary":false}`), &o); err != nil || o.Summary.Requested() {
		t.Fatalf("summary false: %v %+v", err, o.Summary)
	}
}
