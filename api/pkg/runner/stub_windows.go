//go:build windows
// +build windows

package runner

import (
	"context"
	"fmt"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// This file contains stub implementations for Windows builds.
// It ensures that the package can be imported on Windows
// without including any of the Unix-specific code.

type AxolotlRuntime struct{}

var _ Runtime = &AxolotlRuntime{}

type AxolotlRuntimeParams struct {
	RunnerOptions *Options
}

func NewAxolotlRuntime(_ context.Context, _ AxolotlRuntimeParams) (*AxolotlRuntime, error) {
	return nil, fmt.Errorf("axolotl runtime is not supported on windows")
}

func (a *AxolotlRuntime) PullModel(_ context.Context, _ string, _ func(PullProgress) error) error {
	panic("unimplemented")
}

func (a *AxolotlRuntime) Runtime() types.Runtime {
	panic("unimplemented")
}

func (a *AxolotlRuntime) Start(_ context.Context) error {
	panic("unimplemented")
}

func (a *AxolotlRuntime) Stop() error {
	panic("unimplemented")
}

func (a *AxolotlRuntime) URL() string {
	panic("unimplemented")
}

func (a *AxolotlRuntime) Version() string {
	panic("unimplemented")
}

func (a *AxolotlRuntime) Warm(_ context.Context, _ string) error {
	panic("unimplemented")
}

func (a *AxolotlRuntime) Status(_ context.Context) string {
	panic("unimplemented")
}

func (a *AxolotlRuntime) ListModels(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("axolotl runtime is not supported on windows")
}

var _ Runtime = &DiffusersRuntime{}

type DiffusersRuntime struct {
}

type DiffusersRuntimeParams struct {
	CacheDir *string
}

func NewDiffusersRuntime(_ context.Context, _ DiffusersRuntimeParams) (*DiffusersRuntime, error) {
	return nil, fmt.Errorf("diffusers runtime is not supported on windows")
}

func (d *DiffusersRuntime) PullModel(_ context.Context, _ string, _ func(PullProgress) error) error {
	panic("unimplemented")
}

func (d *DiffusersRuntime) Runtime() types.Runtime {
	panic("unimplemented")
}

func (d *DiffusersRuntime) Start(_ context.Context) error {
	panic("unimplemented")
}

func (d *DiffusersRuntime) Stop() error {
	panic("unimplemented")
}

func (d *DiffusersRuntime) URL() string {
	panic("unimplemented")
}

func (d *DiffusersRuntime) Version() string {
	panic("unimplemented")
}

func (d *DiffusersRuntime) Warm(_ context.Context, _ string) error {
	panic("unimplemented")
}

func (d *DiffusersRuntime) Status(_ context.Context) string {
	panic("unimplemented")
}

func (d *DiffusersRuntime) ListModels(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("diffusers runtime is not supported on windows")
}

type OllamaRuntime struct {
	version       string
	cacheDir      string
	port          int
	startTimeout  time.Duration
	contextLength int64
	model         string
	args          []string
	ollamaClient  interface{} // Using interface{} instead of *api.Client to avoid import issues
	cmd           interface{} // Using interface{} instead of *exec.Cmd to avoid import issues
	cancel        context.CancelFunc
}

type OllamaRuntimeParams struct {
	CacheDir      *string
	Port          *int           // If nil, will be assigned a random port
	StartTimeout  *time.Duration // How long to wait for ollama to start, if nil, will use default
	ContextLength *int64         // Optional: Context length to use for the model
	Model         *string        // Optional: Model to use
	Args          []string       // Optional: Additional arguments to pass to Ollama
}

var _ Runtime = &OllamaRuntime{}

func NewOllamaRuntime(_ context.Context, _ OllamaRuntimeParams) (*OllamaRuntime, error) {
	return nil, fmt.Errorf("ollama runtime is not supported on windows")
}

func (o *OllamaRuntime) PullModel(_ context.Context, _ string, _ func(PullProgress) error) error {
	panic("unimplemented")
}

func (o *OllamaRuntime) Runtime() types.Runtime {
	panic("unimplemented")
}

func (o *OllamaRuntime) Start(_ context.Context) error {
	panic("unimplemented")
}

func (o *OllamaRuntime) Stop() error {
	panic("unimplemented")
}

func (o *OllamaRuntime) URL() string {
	panic("unimplemented")
}

func (o *OllamaRuntime) Version() string {
	panic("unimplemented")
}

func (o *OllamaRuntime) Warm(_ context.Context, _ string) error {
	panic("unimplemented")
}

func (a *OllamaRuntime) Status(_ context.Context) string {
	panic("unimplemented")
}

func (a *OllamaRuntime) ListModels(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("ollama runtime is not supported on windows")
}

type VLLMRuntime struct {
	version       string
	cacheDir      string
	port          int
	startTimeout  time.Duration
	contextLength int64
	model         string
	args          []string
	cmd           interface{} // Using interface{} instead of *exec.Cmd to avoid import issues
	cancel        context.CancelFunc
}

type VLLMRuntimeParams struct {
	CacheDir      *string
	Port          *int           // If nil, will be assigned a random port
	StartTimeout  *time.Duration // How long to wait for vLLM to start, if nil, will use default
	ContextLength *int64         // Optional: Context length to use for the model
	Model         *string        // Optional: Model to use
	Args          []string       // Optional: Additional arguments to pass to vLLM
}

var _ Runtime = &VLLMRuntime{}

func NewVLLMRuntime(_ context.Context, _ VLLMRuntimeParams) (*VLLMRuntime, error) {
	return nil, fmt.Errorf("vLLM runtime is not supported on windows")
}

func (v *VLLMRuntime) PullModel(_ context.Context, _ string, _ func(PullProgress) error) error {
	panic("unimplemented")
}

func (v *VLLMRuntime) Runtime() types.Runtime {
	panic("unimplemented")
}

func (v *VLLMRuntime) Start(_ context.Context) error {
	panic("unimplemented")
}

func (v *VLLMRuntime) Stop() error {
	panic("unimplemented")
}

func (v *VLLMRuntime) URL() string {
	panic("unimplemented")
}

func (v *VLLMRuntime) Version() string {
	panic("unimplemented")
}

func (v *VLLMRuntime) Warm(_ context.Context, _ string) error {
	panic("unimplemented")
}

func (v *VLLMRuntime) Status(_ context.Context) string {
	panic("unimplemented")
}

func (v *VLLMRuntime) ListModels(_ context.Context) ([]string, error) {
	return nil, fmt.Errorf("vLLM runtime is not supported on windows")
}
