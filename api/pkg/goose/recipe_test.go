package goose

import (
	"strings"
	"testing"
)

const minimalRecipe = `version: "1.0.0"
title: "Triage CI"
description: "Walk through a failing CI run"
instructions: "Be concise"
prompt: |
  Investigate {{ ci_url }} for branch {{ branch }} and report what failed.
parameters:
  - key: ci_url
    requirement: required
    description: "URL of the failing run"
  - key: branch
    requirement: required
    description: "Branch name"
  - key: depth
    requirement: optional
    default: "1"
`

func TestParseValid(t *testing.T) {
	r, err := Parse([]byte(minimalRecipe))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if r.Version != "1.0.0" {
		t.Fatalf("version: want 1.0.0, got %q", r.Version)
	}
	if len(r.Parameters) != 3 {
		t.Fatalf("want 3 parameters, got %d", len(r.Parameters))
	}
}

func TestParseRejectsEmpty(t *testing.T) {
	if _, err := Parse(nil); err == nil {
		t.Fatal("expected error for empty recipe")
	}
}

func TestParseRejectsMissingVersion(t *testing.T) {
	if _, err := Parse([]byte("title: x\n")); err == nil {
		t.Fatal("expected error for missing version")
	}
}

func TestParseRejectsDuplicateKeys(t *testing.T) {
	doc := `version: "1"
parameters:
  - key: foo
    requirement: required
  - key: foo
    requirement: optional
`
	if _, err := Parse([]byte(doc)); err == nil {
		t.Fatal("expected error for duplicate parameter keys")
	}
}

func TestSubstitute(t *testing.T) {
	out := Substitute("hello {{ name }}, your {{role}} is good", map[string]string{
		"name": "Alice",
		"role": "engineer",
	})
	want := "hello Alice, your engineer is good"
	if out != want {
		t.Fatalf("want %q, got %q", want, out)
	}
}

func TestSubstituteLeavesUnknownAlone(t *testing.T) {
	out := Substitute("hello {{ name }} from {{ city }}", map[string]string{"name": "Bob"})
	want := "hello Bob from {{ city }}"
	if out != want {
		t.Fatalf("want %q, got %q", want, out)
	}
}

func TestValidate(t *testing.T) {
	r, _ := Parse([]byte(minimalRecipe))
	missing := Validate(r, map[string]string{"branch": "main"})
	if len(missing) != 1 || missing[0] != "ci_url" {
		t.Fatalf("want missing ci_url only, got %v", missing)
	}
}

func TestValidateAllPresent(t *testing.T) {
	r, _ := Parse([]byte(minimalRecipe))
	missing := Validate(r, map[string]string{"ci_url": "x", "branch": "y"})
	if len(missing) != 0 {
		t.Fatalf("want no missing, got %v", missing)
	}
}

func TestBakeSubstitutes(t *testing.T) {
	out, err := Bake([]byte(minimalRecipe), map[string]string{
		"ci_url": "https://ci.example.com/run/42",
		"branch": "feature/foo",
	})
	if err != nil {
		t.Fatalf("Bake: %v", err)
	}
	if !strings.Contains(out, "https://ci.example.com/run/42") {
		t.Fatalf("ci_url not substituted; got:\n%s", out)
	}
	if !strings.Contains(out, "feature/foo") {
		t.Fatalf("branch not substituted; got:\n%s", out)
	}
	// Optional param should remain available via default, even though we
	// didn't pass it in — but it's only referenced as {{ depth }} if at all.
	if strings.Contains(out, "{{ branch }}") {
		t.Fatalf("branch placeholder not replaced; got:\n%s", out)
	}
}

func TestBakeRejectsMissingRequired(t *testing.T) {
	_, err := Bake([]byte(minimalRecipe), map[string]string{"branch": "x"})
	if err == nil {
		t.Fatal("expected error for missing required ci_url")
	}
	if !strings.Contains(err.Error(), "ci_url") {
		t.Fatalf("error should name missing param: %v", err)
	}
}
