package sandbox

import (
	"errors"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/config"
	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/types"
)

// RuntimeSpec is everything the controller needs to launch a sandbox of a
// given runtime. It's resolved once at controller startup from
// config.Sandboxes.Runtimes plus any built-in entries (today: ubuntu-desktop).
//
// `Image` is the Docker image. `Entrypoint` and `Cmd` are passed to
// hydra.CreateDevContainerRequest verbatim — for keep-alive containers we
// stuff a shell expression into Cmd. `ContainerType` decides which hydra
// pipeline runs (desktop vs plain). Versioned desktop images are still
// resolved via the heartbeat blob.
type RuntimeSpec struct {
	Name          string
	Image         string
	Entrypoint    []string
	Cmd           []string
	ContainerType hydra.DevContainerType
	// Privileged is true for runtimes that need /dev access (today only the
	// desktop runtime). Headless containers run unprivileged.
	Privileged bool
	// VersionKey is set for runtimes whose image tag is published via the
	// hydra heartbeat (e.g. ubuntu-desktop -> "ubuntu"). Empty for
	// fixed-image runtimes.
	VersionKey string
}

// RuntimeRegistry is the lookup table of all runtimes available on this
// server. Built once at controller-startup; lookups are read-only after that.
type RuntimeRegistry struct {
	specs            map[string]*RuntimeSpec
	defaultRuntime   string
	allowCustomImage bool
}

// builtinDesktop is the streaming-display runtime. We keep it in code (not
// config) because its image is heartbeat-versioned and it has special
// container-type/privileged handling.
var builtinDesktop = &RuntimeSpec{
	Name:          string(types.SandboxRuntimeUbuntuDesktop),
	ContainerType: hydra.DevContainerTypeUbuntu,
	Privileged:    true,
	VersionKey:    "ubuntu",
}

// NewRuntimeRegistry parses cfg.Runtimes and builds the lookup. The format is
// a comma-separated list of `name=image[|cmd]` entries; see config.Sandboxes
// docstring for examples.
func NewRuntimeRegistry(cfg config.Sandboxes) (*RuntimeRegistry, error) {
	r := &RuntimeRegistry{
		specs:            map[string]*RuntimeSpec{},
		defaultRuntime:   strings.TrimSpace(cfg.DefaultRuntime),
		allowCustomImage: cfg.AllowCustomImage,
	}
	r.specs[builtinDesktop.Name] = builtinDesktop

	if strings.TrimSpace(cfg.Runtimes) != "" {
		for _, entry := range strings.Split(cfg.Runtimes, ",") {
			entry = strings.TrimSpace(entry)
			if entry == "" {
				continue
			}
			eq := strings.Index(entry, "=")
			if eq <= 0 {
				return nil, fmt.Errorf("invalid sandbox runtime entry %q: expected name=image[|cmd]", entry)
			}
			name := strings.TrimSpace(entry[:eq])
			rhs := entry[eq+1:]

			image, keepAlive := rhs, "tail -f /dev/null"
			if pipe := strings.Index(rhs, "|"); pipe >= 0 {
				image = strings.TrimSpace(rhs[:pipe])
				k := strings.TrimSpace(rhs[pipe+1:])
				if k != "" {
					keepAlive = k
				}
			}
			image = strings.TrimSpace(image)
			if name == "" || image == "" {
				return nil, fmt.Errorf("invalid sandbox runtime entry %q: empty name or image", entry)
			}

			r.specs[name] = &RuntimeSpec{
				Name:          name,
				Image:         image,
				Entrypoint:    []string{"/bin/sh", "-c"},
				Cmd:           []string{keepAlive},
				ContainerType: hydra.DevContainerTypeHeadless,
				Privileged:    false,
			}
		}
	}

	if r.defaultRuntime == "" {
		// Fall back to the first headless runtime if the operator forgot to
		// set the default.
		for name, s := range r.specs {
			if s.ContainerType == hydra.DevContainerTypeHeadless {
				r.defaultRuntime = name
				break
			}
		}
	}
	if r.defaultRuntime == "" {
		return nil, errors.New("no default sandbox runtime configured and no headless runtimes registered")
	}
	if _, ok := r.specs[r.defaultRuntime]; !ok {
		return nil, fmt.Errorf("HELIX_SANDBOX_DEFAULT_RUNTIME=%q does not match any configured runtime", r.defaultRuntime)
	}
	return r, nil
}

// Resolve picks a RuntimeSpec for a create request. Rules:
//   - explicit Image is honoured only when allow_custom_image is on; it
//     produces an ad-hoc spec with a generic keep-alive command.
//   - explicit Runtime must be a registered name.
//   - both empty → DefaultRuntime.
func (r *RuntimeRegistry) Resolve(req *types.CreateSandboxRequest) (*RuntimeSpec, error) {
	if req == nil {
		req = &types.CreateSandboxRequest{}
	}
	if req.Image != "" {
		if !r.allowCustomImage {
			return nil, errors.New("custom image override is disabled on this server (set HELIX_SANDBOX_ALLOW_CUSTOM_IMAGE=true to enable)")
		}
		if req.Runtime != "" {
			return nil, errors.New("specify either runtime or image, not both")
		}
		return &RuntimeSpec{
			Name:          "custom",
			Image:         req.Image,
			Entrypoint:    []string{"/bin/sh", "-c"},
			Cmd:           []string{"tail -f /dev/null"},
			ContainerType: hydra.DevContainerTypeHeadless,
		}, nil
	}
	name := string(req.Runtime)
	if name == "" {
		name = r.defaultRuntime
	}
	spec, ok := r.specs[name]
	if !ok {
		return nil, fmt.Errorf("unknown runtime %q", name)
	}
	return spec, nil
}

// DefaultRuntimeName returns the operator-configured default. Used by Create
// to stamp Sandbox.Runtime when the request omits both runtime and image.
func (r *RuntimeRegistry) DefaultRuntimeName() string {
	return r.defaultRuntime
}

// Names returns the configured runtime names (sorted is not guaranteed).
// Used by status / list-runtimes endpoints.
func (r *RuntimeRegistry) Names() []string {
	out := make([]string, 0, len(r.specs))
	for name := range r.specs {
		out = append(out, name)
	}
	return out
}
