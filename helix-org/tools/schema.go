package tools

import (
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/google/jsonschema-go/jsonschema"
)

// schemaOpts are passed to every jsonschema.For call so all our tool
// schemas agree on how to render the special types we use.
//
// json.RawMessage is []byte under the hood; without an override the
// generator emits "array of integer 0..255", which is technically true
// of any byte slice but useless for an MCP client. We treat it as
// "any JSON value" instead.
var schemaOpts = &jsonschema.ForOptions{
	TypeSchemas: map[reflect.Type]*jsonschema.Schema{
		reflect.TypeFor[json.RawMessage](): {Type: "object"},
	},
}

// mustSchema builds a JSON Schema from the given args type T at package
// init time. A failure here is a build-time invariant violation (the
// args type is not representable as JSON Schema), not a runtime data
// problem — panicking is the right response and the test suite catches
// it.
func mustSchema[T any]() *jsonschema.Schema {
	s, err := jsonschema.For[T](schemaOpts)
	if err != nil {
		panic(fmt.Sprintf("jsonschema.For[%T]: %v", *new(T), err))
	}
	return s
}
