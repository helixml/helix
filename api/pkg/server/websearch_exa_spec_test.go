package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// exaSpecPath is Exa's published OpenAPI spec; see testdata/exa-openapi/README.md.
const exaSpecPath = "testdata/exa-openapi/exa-openapi-spec.yaml"

// exaSpec checks requests and responses against Exa's OpenAPI spec.
type exaSpec struct {
	doc *openapi3.T
}

var (
	exaSpecOnce sync.Once
	exaSpecDoc  *openapi3.T
	exaSpecErr  error
)

func loadExaSpec(t *testing.T) *exaSpec {
	t.Helper()
	exaSpecOnce.Do(func() { exaSpecDoc, exaSpecErr = parseExaSpec(exaSpecPath) })
	require.NoError(t, exaSpecErr, "load Exa OpenAPI spec")
	return &exaSpec{doc: exaSpecDoc}
}

func parseExaSpec(path string) (*openapi3.T, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var tree any
	if err := yaml.Unmarshal(raw, &tree); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	if err := downgradeOpenAPI31(tree); err != nil {
		return nil, err
	}
	data, err := json.Marshal(tree)
	if err != nil {
		return nil, err
	}
	doc, err := openapi3.NewLoader().LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("load spec: %w", err)
	}
	// 3.1 allows keywords beside $ref and in schemas that 3.0 does not.
	err = doc.Validate(context.Background(),
		openapi3.AllowExtraSiblingFields("description", "deprecated", "default", "title", "x-exa-lifecycle", "x-exa-replacement"),
		openapi3.DisableExamplesValidation())
	if err != nil {
		return nil, fmt.Errorf("validate spec: %w", err)
	}
	if err := exaSpecHelixDifferences(doc); err != nil {
		return nil, err
	}
	return doc, nil
}

// downgradeOpenAPI31 rewrites the spec's OpenAPI 3.1 schema keywords into
// their 3.0 equivalents, which the pinned kin-openapi understands. Each
// rewrite keeps the meaning; see the README for the table.
func downgradeOpenAPI31(node any) error {
	switch v := node.(type) {
	case map[string]any:
		if c, ok := v["const"]; ok {
			switch c.(type) {
			case map[string]any, []any:
				return fmt.Errorf("non-scalar const %v", c)
			}
			delete(v, "const")
			v["enum"] = []any{c}
		}
		if v["type"] == "null" {
			delete(v, "type")
			v["nullable"] = true
			v["enum"] = []any{nil}
		}
		if m, ok := v["exclusiveMinimum"]; ok {
			if _, isBool := m.(bool); !isBool {
				v["minimum"] = m
				v["exclusiveMinimum"] = true
			}
		}
		if ex, ok := v["examples"].([]any); ok {
			delete(v, "examples")
			if len(ex) > 0 {
				v["example"] = ex[0]
			}
		}
		// propertyNames constrains object keys, which 3.0 cannot express.
		// Outside the websets API it is always {type: string}, which every
		// JSON key satisfies.
		delete(v, "propertyNames")
		for _, child := range v {
			if err := downgradeOpenAPI31(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range v {
			if err := downgradeOpenAPI31(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// exaSpecHelixDifferences relaxes the spec where Helix deliberately returns
// more than Exa documents. Each is one named path; nothing else is relaxed.
func exaSpecHelixDifferences(doc *openapi3.T) error {
	schemas := doc.Components.Schemas
	need := func(name string) (*openapi3.Schema, error) {
		ref, ok := schemas[name]
		if !ok || ref.Value == nil {
			return nil, fmt.Errorf("spec has no schema %q", name)
		}
		return ref.Value, nil
	}

	// 1. warnings on /contents and /findSimilar. Helix reports every ignored
	// or approximated field as a warnings[] entry on every endpoint (design
	// §7.1: never silently ignore a field); Exa documents warnings on /search
	// only. Same item schema as /search's warnings.
	warning, err := need("SearchWarningOutput")
	if err != nil {
		return err
	}
	for _, name := range []string{"ContentsResponse", "FindSimilarResponse"} {
		s, err := need(name)
		if err != nil {
			return err
		}
		if _, exists := s.Properties["warnings"]; exists {
			return fmt.Errorf("%s now documents warnings; drop this relaxation", name)
		}
		s.Properties["warnings"] = openapi3.NewArraySchema().WithItems(warning).NewRef()
	}

	// 2. Contents on subpages. Helix renders the requested contents (text,
	// highlights, summary, extras) on each subpage as on its parent result;
	// the spec's subpage item lists page metadata only.
	for _, name := range []string{"SearchResultOutput", "FindSimilarResult"} {
		s, err := need(name)
		if err != nil {
			return err
		}
		sub := s.Properties["subpages"]
		if sub == nil || sub.Value.Items == nil || sub.Value.Items.Value == nil {
			return fmt.Errorf("%s.subpages has no item schema", name)
		}
		item := sub.Value.Items.Value
		for _, field := range []string{"text", "highlights", "summary", "extras"} {
			if _, exists := item.Properties[field]; exists {
				return fmt.Errorf("%s.subpages[] now documents %s; drop this relaxation", name, field)
			}
			item.Properties[field] = s.Properties[field]
		}
	}
	return nil
}

func (s *exaSpec) route(path string) (*routers.Route, error) {
	item := s.doc.Paths.Value(path)
	if item == nil || item.Post == nil {
		return nil, fmt.Errorf("spec has no POST %s", path)
	}
	return &routers.Route{Spec: s.doc, Path: path, PathItem: item, Method: http.MethodPost, Operation: item.Post}, nil
}

func (s *exaSpec) requestInput(path string, body []byte) (*openapi3filter.RequestValidationInput, error) {
	route, err := s.route(path)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(http.MethodPost, "https://api.exa.ai"+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", "test")
	return &openapi3filter.RequestValidationInput{
		Request: req,
		Route:   route,
		Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc, MultiError: true},
	}, nil
}

// validateRequest checks a request body against the operation's request
// schema, so the test sends only what a real Exa client may send.
func (s *exaSpec) validateRequest(path string, body []byte) error {
	in, err := s.requestInput(path, body)
	if err != nil {
		return err
	}
	if err := openapi3filter.ValidateRequest(context.Background(), in); err != nil {
		return errors.New(describeSpecError(err))
	}
	return nil
}

// validateJSONResponse checks a JSON response against the schema the spec
// declares for its status code.
func (s *exaSpec) validateJSONResponse(path string, reqBody []byte, status int, header http.Header, body []byte) error {
	in, err := s.requestInput(path, reqBody)
	if err != nil {
		return err
	}
	out := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: in,
		Status:                 status,
		Header:                 header,
		Body:                   io.NopCloser(bytes.NewReader(body)),
		Options:                &openapi3filter.Options{IncludeResponseStatus: true, MultiError: true},
	}
	if err := openapi3filter.ValidateResponse(context.Background(), out); err != nil {
		return errors.New(describeSpecError(err))
	}
	return nil
}

// streamEventSchema is the schema of one SSE data payload of a 200 response.
func (s *exaSpec) streamEventSchema(path string) (*openapi3.Schema, error) {
	route, err := s.route(path)
	if err != nil {
		return nil, err
	}
	resp := route.Operation.Responses.Status(http.StatusOK)
	if resp == nil || resp.Value == nil {
		return nil, fmt.Errorf("spec has no 200 response for %s", path)
	}
	media := resp.Value.Content.Get("text/event-stream")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		return nil, fmt.Errorf("spec declares no text/event-stream response for %s", path)
	}
	return media.Schema.Value, nil
}

// validateStreamEvent checks one SSE data payload against the spec.
func (s *exaSpec) validateStreamEvent(path string, event any) error {
	schema, err := s.streamEventSchema(path)
	if err != nil {
		return err
	}
	if err := schema.VisitJSON(event, openapi3.VisitAsResponse(), openapi3.MultiErrors()); err != nil {
		return errors.New(describeSpecError(err))
	}
	return nil
}

// describeSpecError flattens kin-openapi's nested errors into one line per
// failing location, without the schema dumps its Error() includes.
func describeSpecError(err error) string {
	var lines []string
	var walk func(err error, depth int)
	walk = func(err error, depth int) {
		indent := strings.Repeat("  ", depth)
		switch e := err.(type) {
		case openapi3.MultiError:
			for _, inner := range e {
				walk(inner, depth)
			}
		case *openapi3.SchemaError:
			lines = append(lines, fmt.Sprintf("%sat /%s: %s", indent, strings.Join(e.JSONPointer(), "/"), e.Reason))
			if e.Origin != nil {
				walk(e.Origin, depth+1)
			}
		default:
			if inner := errors.Unwrap(err); inner != nil {
				walk(inner, depth)
				return
			}
			lines = append(lines, indent+err.Error())
		}
	}
	walk(err, 0)
	return strings.Join(lines, "\n")
}
