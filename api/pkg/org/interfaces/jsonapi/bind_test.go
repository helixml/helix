package jsonapi_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/helixml/helix/api/pkg/org/interfaces/jsonapi"
)

type procAttrs struct {
	Name         string `json:"name"`
	InputTopicID string `json:"input_topic_id"`
	Kind         string `json:"kind"`
}

func TestBindDecodesAttributes(t *testing.T) {
	body := `{"data":{"type":"processors","attributes":{"name":"Fmt","input_topic_id":"s-in","kind":"template"}}}`
	r := httptest.NewRequest("POST", "/processors", strings.NewReader(body))
	var a procAttrs
	if err := jsonapi.Bind(r, &a); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if a.Name != "Fmt" || a.InputTopicID != "s-in" || a.Kind != "template" {
		t.Errorf("decoded %+v", a)
	}
}

func TestBindRejectsMissingAttributes(t *testing.T) {
	r := httptest.NewRequest("POST", "/processors", strings.NewReader(`{"data":{"type":"processors"}}`))
	var a procAttrs
	if err := jsonapi.Bind(r, &a); err == nil {
		t.Error("want error for missing attributes, got nil")
	}
}

func TestBindRejectsEmptyBody(t *testing.T) {
	r := httptest.NewRequest("POST", "/processors", strings.NewReader(""))
	var a procAttrs
	if err := jsonapi.Bind(r, &a); err == nil {
		t.Error("want error for empty body, got nil")
	}
}

func TestBindRejectsMalformedJSON(t *testing.T) {
	r := httptest.NewRequest("POST", "/processors", strings.NewReader(`{"data":{`))
	var a procAttrs
	if err := jsonapi.Bind(r, &a); err == nil {
		t.Error("want error for malformed JSON, got nil")
	}
}
