package composeparse

import (
	"reflect"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func TestParse_ServedModelNamePreferred(t *testing.T) {
	yaml := `
services:
  qwen:
    image: vllm/vllm-openai:latest
    container_name: vllm-qwen
    ports:
      - "127.0.0.1:8000:8000"
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
              capabilities: [gpu]
    command:
      - --model
      - Qwen/Qwen3.5-35B-A3B-FP8
      - --served-model-name
      - qwen3.5-35b
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	wantModels := []types.ProfileModel{
		{Name: "qwen3.5-35b", ContainerName: "vllm-qwen", InternalPort: 8000},
	}
	if !reflect.DeepEqual(r.Models, wantModels) {
		t.Errorf("models: got %+v, want %+v", r.Models, wantModels)
	}
	if r.GPUCount != 1 {
		t.Errorf("GPUCount: got %d, want 1", r.GPUCount)
	}
}

func TestParse_ModelFallbackBasename(t *testing.T) {
	yaml := `
services:
  embed:
    image: vllm/vllm-openai:latest
    ports:
      - "8001:8000"
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
    command:
      - --model
      - Qwen/Qwen3-Embedding-8B
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(r.Models))
	}
	// No --served-model-name → fallback to lowercase basename of --model.
	if got, want := r.Models[0].Name, "qwen3-embedding-8b"; got != want {
		t.Errorf("model name: got %q, want %q", got, want)
	}
	// No container_name → falls back to service key.
	if got, want := r.Models[0].ContainerName, "embed"; got != want {
		t.Errorf("container name: got %q, want %q", got, want)
	}
	// Port form "host:container" → take container.
	if got, want := r.Models[0].InternalPort, 8000; got != want {
		t.Errorf("port: got %d, want %d", got, want)
	}
}

func TestParse_MultiGPUDeviceIDs(t *testing.T) {
	yaml := `
services:
  big:
    image: vllm/vllm-openai:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["2", "3", "4", "5"]
    command: ["--model", "MiniMax/foo", "--tensor-parallel-size", "4"]
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if r.GPUCount != 4 {
		t.Errorf("GPUCount: got %d, want 4", r.GPUCount)
	}
}

func TestParse_TensorParallelAcrossServicesUnionsGPUs(t *testing.T) {
	// Two services: one on GPU 0, one on GPUs 1+2. Union = 3 GPUs.
	yaml := `
services:
  small:
    image: vllm/vllm-openai:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
    command: ["--model", "x/y"]
  big:
    image: vllm/vllm-openai:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["1", "2"]
    command: ["--model", "p/q", "--tensor-parallel-size", "2"]
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if r.GPUCount != 3 {
		t.Errorf("GPUCount: got %d, want 3", r.GPUCount)
	}
	if len(r.Models) != 2 {
		t.Errorf("expected 2 models, got %d", len(r.Models))
	}
}

func TestParse_NoGPUReservation(t *testing.T) {
	yaml := `
services:
  cpu-only:
    image: ghcr.io/foo/bar:latest
    command: ["--model", "x/y"]
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if r.GPUCount != 0 {
		t.Errorf("GPUCount: got %d, want 0", r.GPUCount)
	}
	if len(r.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(r.Models))
	}
}

func TestParse_AMDDeviceCount(t *testing.T) {
	yaml := `
services:
  qwen-rocm:
    image: rocm/vllm:latest
    devices:
      - /dev/kfd
      - /dev/dri/renderD128
      - /dev/dri/renderD129
    group_add:
      - video
      - render
    command: ["--model", "Qwen/foo", "--served-model-name", "qwen-amd"]
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	// /dev/kfd is shared and not counted; renderD128 + renderD129 = 2 GPUs.
	if r.GPUCount != 2 {
		t.Errorf("GPUCount: got %d, want 2", r.GPUCount)
	}
	if r.Models[0].Name != "qwen-amd" {
		t.Errorf("model name: got %q, want %q", r.Models[0].Name, "qwen-amd")
	}
}

func TestParse_RejectsMixedNVIDIAAndAMDInOneService(t *testing.T) {
	yaml := `
services:
  confused:
    image: vllm/vllm-openai:latest
    devices:
      - /dev/kfd
      - /dev/dri/renderD128
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
    command: ["--model", "x/y"]
`
	_, err := Parse([]byte(yaml))
	if err == nil {
		t.Fatal("expected error for mixed NVIDIA+AMD declarations, got nil")
	}
}

func TestParse_FlagEqualsSyntax(t *testing.T) {
	yaml := `
services:
  s1:
    image: vllm/vllm-openai:latest
    command: ["--model=Qwen/foo", "--served-model-name=qwen"]
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if r.Models[0].Name != "qwen" {
		t.Errorf("model name: got %q, want %q", r.Models[0].Name, "qwen")
	}
}

func TestParse_StringCommandSplit(t *testing.T) {
	// Compose accepts command as a string; we split on whitespace, which is
	// correct for this purpose even though compose actually shell-evals.
	yaml := `
services:
  s1:
    image: vllm/vllm-openai:latest
    command: "--model Qwen/foo --served-model-name qwen"
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if r.Models[0].Name != "qwen" {
		t.Errorf("model name: got %q, want %q", r.Models[0].Name, "qwen")
	}
}

func TestParse_ServiceWithoutModelIsSkipped(t *testing.T) {
	// A sidecar / init container with no --model is allowed in a profile;
	// it just doesn't contribute a routable model. The GPU count still
	// reflects its device reservations.
	yaml := `
services:
  sidecar:
    image: alpine:latest
    command: ["sleep", "infinity"]
  qwen:
    image: vllm/vllm-openai:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              device_ids: ["0"]
    command: ["--model", "Qwen/foo"]
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Models) != 1 {
		t.Fatalf("expected 1 model (sidecar skipped), got %d", len(r.Models))
	}
	// Model name = lowercase basename of "Qwen/foo" = "foo".
	if r.Models[0].Name != "foo" {
		t.Errorf("model name: got %q, want %q", r.Models[0].Name, "foo")
	}
}

func TestParse_PortForms(t *testing.T) {
	cases := []struct {
		name    string
		yamlPort string
		want    int
	}{
		{"ip-host-container", `["127.0.0.1:8000:8001"]`, 8001},
		{"host-container", `["8000:8001"]`, 8001},
		{"single", `["8000"]`, 8000},
		{"single-int", `[8000]`, 8000},
		{"target-mapping", `[{target: 8001, published: 8000}]`, 8001},
		{"with-protocol", `["8000:8001/tcp"]`, 8001},
		{"range", `["8000-8005:8000-8005"]`, 8000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			y := `
services:
  s:
    image: x
    ports: ` + tc.yamlPort + `
    command: ["--model", "x/y"]
`
			r, err := Parse([]byte(y))
			if err != nil {
				t.Fatal(err)
			}
			if r.Models[0].InternalPort != tc.want {
				t.Errorf("got port %d, want %d", r.Models[0].InternalPort, tc.want)
			}
		})
	}
}

func TestParse_EmptyServicesIsError(t *testing.T) {
	_, err := Parse([]byte(`services: {}`))
	if err == nil {
		t.Error("expected error for empty services map")
	}
}

func TestParse_InvalidYAMLIsError(t *testing.T) {
	_, err := Parse([]byte("this: is: not: valid: yaml: at all"))
	if err == nil {
		t.Error("expected parse error for invalid YAML")
	}
}

func TestParse_NVIDIACountFallback(t *testing.T) {
	// device_ids absent, count present.
	yaml := `
services:
  s:
    image: vllm/vllm-openai:latest
    deploy:
      resources:
        reservations:
          devices:
            - driver: nvidia
              count: 4
              capabilities: [gpu]
    command: ["--model", "x/y", "--tensor-parallel-size", "4"]
`
	r, err := Parse([]byte(yaml))
	if err != nil {
		t.Fatal(err)
	}
	if r.GPUCount != 4 {
		t.Errorf("GPUCount: got %d, want 4", r.GPUCount)
	}
}
