// Package goose parses Block's Goose recipe YAML format and substitutes
// parameter values. It does NOT vendor Goose's full recipe schema; we only
// need enough to:
//
//  1. List declared parameters (so the UI can render an input form).
//  2. Substitute parameter values into the rendered file.
//  3. Reject obviously bogus recipes (no version, malformed YAML).
//
// Substitution is Jinja-style: {{ var }} or {{var}}, with optional inner
// whitespace. Goose itself uses a full Jinja2 evaluator (rust crate
// `minijinja`); we keep our substitution intentionally simple because
// recipes are baked at task-creation time (when complex expressions are
// not expected) and the user's authored recipe still runs through goose at
// agent time for any extra Jinja logic. Anything we cannot substitute
// (unknown variable, complex expression) is left intact and goose handles
// it at runtime.
package goose

import (
	"fmt"
	"path"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// DefaultName derives a slash-command name from a recipe file path:
// basename minus the .yaml / .yml extension. Empty input → empty output;
// callers decide whether that's an error. The result is intentionally
// not validated against SlashCommandPattern — callers that need a
// shell-safe name should still run it through the pattern check.
func DefaultName(filePath string) string {
	base := path.Base(strings.TrimSpace(filePath))
	if base == "" || base == "." || base == "/" {
		return ""
	}
	for _, ext := range []string{".yaml", ".yml"} {
		if strings.HasSuffix(strings.ToLower(base), ext) {
			return base[:len(base)-len(ext)]
		}
	}
	return base
}

// SlashCommandPattern matches valid Goose slash-command names: lowercase
// alphanumerics, dashes, and underscores; must start with an alphanumeric.
// Mirrors the frontend's pattern in GooseRecipesEditor.tsx — keep in sync.
var SlashCommandPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// RecipeParameter is a single declared parameter on a Goose recipe.
// We only care about the fields needed to drive the spec-task UI.
type RecipeParameter struct {
	Key         string   `yaml:"key" json:"key"`
	InputType   string   `yaml:"input_type,omitempty" json:"input_type,omitempty"`
	Requirement string   `yaml:"requirement,omitempty" json:"requirement,omitempty"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	Default     string   `yaml:"default,omitempty" json:"default,omitempty"`
	Options     []string `yaml:"options,omitempty" json:"options,omitempty"`
}

// Recipe is the slice of the Goose recipe schema we parse.
type Recipe struct {
	Version     string            `yaml:"version" json:"version"`
	Title       string            `yaml:"title,omitempty" json:"title,omitempty"`
	Description string            `yaml:"description,omitempty" json:"description,omitempty"`
	Parameters  []RecipeParameter `yaml:"parameters,omitempty" json:"parameters,omitempty"`
}

// Parse decodes a recipe YAML payload. Returns an error for malformed YAML
// or when the recipe is missing a version field. We do not enforce the
// full Goose schema — goose itself will reject the recipe at runtime if
// extensions / activities / prompts are wrong.
func Parse(content []byte) (*Recipe, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("recipe is empty")
	}
	var r Recipe
	if err := yaml.Unmarshal(content, &r); err != nil {
		return nil, fmt.Errorf("decode recipe yaml: %w", err)
	}
	if strings.TrimSpace(r.Version) == "" {
		return nil, fmt.Errorf("recipe missing required 'version' field")
	}
	// Sanity check parameter keys: they must be non-empty and unique.
	seen := map[string]bool{}
	for i, p := range r.Parameters {
		key := strings.TrimSpace(p.Key)
		if key == "" {
			return nil, fmt.Errorf("recipe parameter %d has empty key", i)
		}
		if seen[key] {
			return nil, fmt.Errorf("recipe parameter %q declared more than once", key)
		}
		seen[key] = true
	}
	return &r, nil
}

// jinjaPattern matches {{ var }} / {{var}} / {{  var_2 }} — a single
// identifier surrounded by optional whitespace inside double braces. We
// deliberately do not handle filters or expressions: anything that
// doesn't match this pattern is passed through unchanged so the full
// Jinja2 evaluator inside goose can still render it at runtime.
var jinjaPattern = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// Substitute replaces {{ var }} placeholders in content with values from
// params. Variables present in params are substituted; variables absent
// from params are left as-is, including any Jinja expressions we don't
// understand. Missing required parameters are reported via Validate, not
// here, so callers can decide whether to fail-fast or let goose handle it.
func Substitute(content string, params map[string]string) string {
	if len(params) == 0 {
		return content
	}
	return jinjaPattern.ReplaceAllStringFunc(content, func(match string) string {
		sub := jinjaPattern.FindStringSubmatch(match)
		if len(sub) != 2 {
			return match
		}
		val, ok := params[sub[1]]
		if !ok {
			return match
		}
		return val
	})
}

// Validate checks that every parameter declared with requirement=="required"
// has a value in params (either supplied or via the recipe's default).
// Returns the list of missing parameter keys; nil if all required values
// are present.
func Validate(r *Recipe, params map[string]string) []string {
	var missing []string
	for _, p := range r.Parameters {
		if p.Requirement != "required" {
			continue
		}
		val, ok := params[p.Key]
		if (!ok || strings.TrimSpace(val) == "") && strings.TrimSpace(p.Default) == "" {
			missing = append(missing, p.Key)
		}
	}
	return missing
}

// Bake parses a recipe, validates required params, and returns the
// substituted YAML content ready to write to disk for goose to consume.
// Returns an error if the recipe is malformed or required params are
// missing. The returned string is the *original* YAML with placeholders
// replaced — we don't re-serialise via go-yaml, so comments and ordering
// from the source recipe are preserved.
func Bake(content []byte, params map[string]string) (string, error) {
	r, err := Parse(content)
	if err != nil {
		return "", err
	}
	if missing := Validate(r, params); len(missing) > 0 {
		return "", fmt.Errorf("recipe missing required parameters: %s", strings.Join(missing, ", "))
	}
	// Merge params over recipe defaults so {{var}} placeholders resolve
	// even when the user didn't supply an explicit value.
	merged := map[string]string{}
	for _, p := range r.Parameters {
		if p.Default != "" {
			merged[p.Key] = p.Default
		}
	}
	for k, v := range params {
		merged[k] = v
	}
	return Substitute(string(content), merged), nil
}
