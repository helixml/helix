package inferencerouter

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

func runningProfile(modelNames ...string) *types.RunnerProfile {
	models := make([]types.ProfileModel, len(modelNames))
	for i, n := range modelNames {
		models[i] = types.ProfileModel{Name: n, ContainerName: "c-" + n, InternalPort: 8000}
	}
	return &types.RunnerProfile{Name: "p", Models: models}
}

func TestRouter_PickRunner_HappyPath(t *testing.T) {
	r := NewRouter()
	r.SetRunnerState(&RunnerState{
		ID: "runner-a", URL: "http://a:8081", Status: "running",
		ActiveProfile: runningProfile("qwen-7b"), LastSeen: time.Now(),
	})
	got, err := r.PickRunner("qwen-7b")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "runner-a" {
		t.Errorf("got %q, want runner-a", got.ID)
	}
}

func TestRouter_PickRunner_NoMatchReturnsNoRunnerError(t *testing.T) {
	r := NewRouter()
	r.SetRunnerState(&RunnerState{
		ID: "runner-a", Status: "running",
		ActiveProfile: runningProfile("qwen-7b"),
	})
	_, err := r.PickRunner("nonexistent")
	if !errors.Is(err, ErrNoRunner) {
		t.Errorf("got %v, want errors.Is(err, ErrNoRunner) == true", err)
	}
	var nre *NoRunnerError
	if !errors.As(err, &nre) {
		t.Fatalf("expected *NoRunnerError, got %T", err)
	}
	if nre.RequestedModel != "nonexistent" {
		t.Errorf("RequestedModel: got %q", nre.RequestedModel)
	}
	if len(nre.AvailableModels) != 1 || nre.AvailableModels[0] != "qwen-7b" {
		t.Errorf("AvailableModels: got %v", nre.AvailableModels)
	}
}

func TestRouter_PickRunner_SkipsNonRunning(t *testing.T) {
	r := NewRouter()
	r.SetRunnerState(&RunnerState{
		ID: "runner-a", Status: "starting",
		ActiveProfile: runningProfile("qwen-7b"),
	})
	r.SetRunnerState(&RunnerState{
		ID: "runner-b", Status: "running",
		ActiveProfile: runningProfile("qwen-7b"),
	})
	for i := 0; i < 5; i++ {
		got, err := r.PickRunner("qwen-7b")
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != "runner-b" {
			t.Errorf("iter %d: picked non-running runner %q", i, got.ID)
		}
	}
}

func TestRouter_PickRunner_RoundRobin(t *testing.T) {
	r := NewRouter()
	r.SetRunnerState(&RunnerState{ID: "a", Status: "running", ActiveProfile: runningProfile("m")})
	r.SetRunnerState(&RunnerState{ID: "b", Status: "running", ActiveProfile: runningProfile("m")})
	r.SetRunnerState(&RunnerState{ID: "c", Status: "running", ActiveProfile: runningProfile("m")})
	picked := make([]string, 6)
	for i := range picked {
		got, err := r.PickRunner("m")
		if err != nil {
			t.Fatal(err)
		}
		picked[i] = got.ID
	}
	// Sorted candidates are [a, b, c]; counter starts at 0, so first pick
	// is candidates[0] = a, then b, then c, then a again.
	want := []string{"a", "b", "c", "a", "b", "c"}
	for i := range want {
		if picked[i] != want[i] {
			t.Errorf("pick %d: got %q, want %q", i, picked[i], want[i])
		}
	}
}

func TestRouter_PickRunner_PerModelCounters(t *testing.T) {
	// Two models, two runners each. Picks for one model don't influence
	// rotation for the other.
	r := NewRouter()
	r.SetRunnerState(&RunnerState{ID: "a", Status: "running", ActiveProfile: runningProfile("x", "y")})
	r.SetRunnerState(&RunnerState{ID: "b", Status: "running", ActiveProfile: runningProfile("x", "y")})

	xPicks := []string{}
	for i := 0; i < 3; i++ {
		got, _ := r.PickRunner("x")
		xPicks = append(xPicks, got.ID)
	}
	yPicks := []string{}
	for i := 0; i < 3; i++ {
		got, _ := r.PickRunner("y")
		yPicks = append(yPicks, got.ID)
	}
	// Each model gets its own a, b, a rotation.
	if xPicks[0] != "a" || xPicks[1] != "b" || xPicks[2] != "a" {
		t.Errorf("x rotation broken: %v", xPicks)
	}
	if yPicks[0] != "a" || yPicks[1] != "b" || yPicks[2] != "a" {
		t.Errorf("y rotation broken: %v", yPicks)
	}
}

func TestRouter_AvailableModels_UnionDedup(t *testing.T) {
	r := NewRouter()
	r.SetRunnerState(&RunnerState{ID: "a", Status: "running", ActiveProfile: runningProfile("x", "y")})
	r.SetRunnerState(&RunnerState{ID: "b", Status: "running", ActiveProfile: runningProfile("y", "z")})
	r.SetRunnerState(&RunnerState{ID: "c", Status: "starting", ActiveProfile: runningProfile("hidden")})
	got := r.AvailableModels()
	want := []string{"x", "y", "z"} // sorted, deduped, "hidden" excluded (not running)
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got %v, want %v", got, want)
			break
		}
	}
}

func TestRouter_RemoveRunner(t *testing.T) {
	r := NewRouter()
	r.SetRunnerState(&RunnerState{ID: "a", Status: "running", ActiveProfile: runningProfile("m")})
	r.RemoveRunner("a")
	if _, err := r.PickRunner("m"); !errors.Is(err, ErrNoRunner) {
		t.Errorf("after remove: expected ErrNoRunner, got %v", err)
	}
	if r.GetRunner("a") != nil {
		t.Error("GetRunner should return nil after RemoveRunner")
	}
}

func TestRouter_RouteableModels_NilProfileNoneSkipped(t *testing.T) {
	r := NewRouter()
	r.SetRunnerState(&RunnerState{ID: "a", Status: "running", ActiveProfile: nil})
	if got := r.AvailableModels(); len(got) != 0 {
		t.Errorf("nil profile should expose 0 models, got %v", got)
	}
}

func TestRouter_NoRunnerError_AvailableModelsEmptyMessage(t *testing.T) {
	r := NewRouter()
	_, err := r.PickRunner("gpt-5.4")
	var nre *NoRunnerError
	if !errors.As(err, &nre) {
		t.Fatal("expected NoRunnerError")
	}
	got := nre.Error()
	// User-facing wording: must not leak Helix-internal "runner" terminology
	// (this string surfaces as an OpenAI 503 to end users) and must name
	// the requested model so they can see what was rejected.
	if strings.Contains(got, "runner") {
		t.Errorf("error message should not mention %q (user-facing): %q", "runner", got)
	}
	if !strings.Contains(got, `"gpt-5.4"`) {
		t.Errorf("error message should quote the requested model name: %q", got)
	}
	if !strings.Contains(got, "not available") {
		t.Errorf("error message should say model is not available: %q", got)
	}
}

func TestRouter_NoRunnerError_AvailableModelsListMessage(t *testing.T) {
	nre := &NoRunnerError{
		RequestedModel:  "gpt-5.4",
		AvailableModels: []string{"qwen3-coder", "llama-3.3"},
	}
	got := nre.Error()
	if strings.Contains(got, "runner") {
		t.Errorf("error message should not mention %q (user-facing): %q", "runner", got)
	}
	if !strings.Contains(got, "qwen3-coder") || !strings.Contains(got, "llama-3.3") {
		t.Errorf("error message should list available models: %q", got)
	}
}
