package tui

import (
	"testing"
)

func TestPredictiveEcho_Basic(t *testing.T) {
	p := NewPredictiveEcho()

	// Type "hello"
	p.AddPredicted("h")
	p.AddPredicted("e")
	p.AddPredicted("l")
	p.AddPredicted("l")
	p.AddPredicted("o")

	if !p.HasPredicted() {
		t.Error("should have predicted text")
	}
	if p.FullText() != "hello" {
		t.Errorf("expected 'hello', got %q", p.FullText())
	}

	// Server confirms
	p.Confirm("hello")
	if p.HasPredicted() {
		t.Error("should not have predicted after confirm")
	}
	if p.FullText() != "hello" {
		t.Errorf("expected 'hello' after confirm, got %q", p.FullText())
	}
}

func TestPredictiveEcho_PartialConfirm(t *testing.T) {
	p := NewPredictiveEcho()

	// Type "hello world"
	for _, c := range "hello world" {
		p.AddPredicted(string(c))
	}

	// Server confirms "hello" (not the full text yet)
	p.Confirm("hello")
	if p.FullText() != "hello world" {
		t.Errorf("expected 'hello world', got %q", p.FullText())
	}
	if !p.HasPredicted() {
		t.Error("should still have predicted text for ' world'")
	}
}

func TestPredictiveEcho_Rollback(t *testing.T) {
	p := NewPredictiveEcho()

	p.AddPredicted("h")
	p.AddPredicted("i")
	p.Rollback()

	if p.HasPredicted() {
		t.Error("should not have predicted after rollback")
	}
	if p.FullText() != "" {
		t.Errorf("expected empty after rollback, got %q", p.FullText())
	}
}

func TestPredictiveEcho_Backspace(t *testing.T) {
	p := NewPredictiveEcho()

	p.AddPredicted("h")
	p.AddPredicted("e")
	p.AddPredicted("x")
	p.RemovePredicted()

	if p.FullText() != "he" {
		t.Errorf("expected 'he', got %q", p.FullText())
	}
}

func TestPredictiveEcho_Disabled(t *testing.T) {
	p := NewPredictiveEcho()
	p.SetEnabled(false)

	p.AddPredicted("h")
	if p.HasPredicted() {
		t.Error("should not add predicted when disabled")
	}
}

func TestPredictiveEcho_Reset(t *testing.T) {
	p := NewPredictiveEcho()
	p.AddPredicted("hello")
	p.Confirm("he")
	p.Reset()

	if p.FullText() != "" {
		t.Errorf("expected empty after reset, got %q", p.FullText())
	}
}

func TestPredictiveEcho_ServerDiverges(t *testing.T) {
	p := NewPredictiveEcho()

	// User types "helllo" (typo)
	for _, c := range "helllo" {
		p.AddPredicted(string(c))
	}

	// Server autocorrects to "hello"
	p.Confirm("hello")

	// Predicted should be rolled back since server diverged
	if p.HasPredicted() {
		t.Error("predicted should be cleared when server diverges")
	}
	if p.FullText() != "hello" {
		t.Errorf("expected 'hello', got %q", p.FullText())
	}
}
