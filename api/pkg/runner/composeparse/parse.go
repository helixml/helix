// Package composeparse extracts the model list and GPU count from a runner
// profile's Docker Compose YAML. It is intentionally minimal — it doesn't
// validate the full compose schema; it pulls out only what the profile store
// needs (which models the profile exposes and how many GPUs it touches).
//
// Vendor / architecture / VRAM constraints are NOT extracted from YAML —
// they are operator inputs entered alongside the profile (see
// types.ProfileGPURequirement).
package composeparse

import (
	"errors"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/helixml/helix/api/pkg/types"
	"gopkg.in/yaml.v3"
)

// ParseResult is the structured output of Parse.
type ParseResult struct {
	// Models, one per service that exposes a model server. Order matches the
	// service ordering in the YAML (yaml.v3 preserves map order).
	Models []types.ProfileModel

	// GPUCount is the union of GPUs touched across all services. For NVIDIA
	// declarations this is the size of the union of device_ids across all
	// services. For AMD declarations it is the number of distinct
	// /dev/dri/renderD* entries across all services.
	GPUCount int
}

// Parse reads a compose YAML byte slice and returns the derived metadata.
// Any structural error or per-service parse error is returned as a single
// error; callers surface this verbatim to the operator.
func Parse(data []byte) (*ParseResult, error) {
	var raw composeFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("compose YAML is not valid: %w", err)
	}
	if len(raw.Services) == 0 {
		return nil, errors.New("compose has no services")
	}

	out := &ParseResult{}
	nvidiaIDs := map[string]struct{}{}
	amdRenderNodes := map[string]struct{}{}

	// yaml.v3 unmarshals maps in source order via MapSlice if we ask, but
	// the standard map type re-orders. To preserve service order we walk the
	// raw node tree.
	for _, name := range raw.serviceNames() {
		svc := raw.Services[name]

		// Reject services that mix NVIDIA and AMD GPU declarations in one
		// service — ambiguous and likely an operator mistake.
		hasNVIDIA := svc.hasNVIDIAGPUDecl()
		hasAMD := svc.hasAMDGPUDecl()
		if hasNVIDIA && hasAMD {
			return nil, fmt.Errorf("service %q declares both NVIDIA and AMD GPU passthrough; pick one", name)
		}

		// Collect GPU references.
		if hasNVIDIA {
			for _, id := range svc.nvidiaDeviceIDs() {
				nvidiaIDs[id] = struct{}{}
			}
		}
		if hasAMD {
			for _, dev := range svc.amdRenderNodes() {
				amdRenderNodes[dev] = struct{}{}
			}
		}

		// A service is a model server only if its command extracts to a
		// model name. Services with no model name (e.g. a sidecar / init
		// container) are skipped silently — they're allowed in a profile.
		modelName := extractModelName(svc.commandTokens())
		if modelName == "" {
			continue
		}
		port := extractInternalPort(svc.Ports, svc.Expose)
		container := svc.ContainerName
		if container == "" {
			container = name
		}
		out.Models = append(out.Models, types.ProfileModel{
			Name:          modelName,
			ContainerName: container,
			InternalPort:  port,
		})
	}

	// Sum NVIDIA + AMD counts. In practice operators won't mix vendors in
	// one profile (the compatibility check would never accept such a
	// profile against any single runner), but we don't forbid it at the
	// parse level — the profile-store just records the count.
	out.GPUCount = len(nvidiaIDs) + len(amdRenderNodes)
	return out, nil
}

// composeFile is the minimal subset of the compose schema we care about.
type composeFile struct {
	Services map[string]composeService `yaml:"services"`
	// rawNode preserves source order for stable iteration.
	rawNode *yaml.Node
}

func (c *composeFile) UnmarshalYAML(node *yaml.Node) error {
	c.rawNode = node
	type alias composeFile
	tmp := alias{}
	if err := node.Decode(&tmp); err != nil {
		return err
	}
	c.Services = tmp.Services
	return nil
}

// serviceNames returns service keys in source order.
func (c *composeFile) serviceNames() []string {
	if c.rawNode == nil {
		// Fallback to map iteration with deterministic sort.
		out := make([]string, 0, len(c.Services))
		for k := range c.Services {
			out = append(out, k)
		}
		sort.Strings(out)
		return out
	}
	// rawNode is the top-level mapping. Find the "services" key, then walk
	// its mapping content in source order.
	for _, top := range mappingChildren(c.rawNode) {
		if top.key == "services" && top.value != nil && top.value.Kind == yaml.MappingNode {
			out := make([]string, 0, len(c.Services))
			for _, kv := range mappingChildren(top.value) {
				out = append(out, kv.key)
			}
			return out
		}
	}
	return nil
}

type composeService struct {
	Image         string         `yaml:"image"`
	ContainerName string         `yaml:"container_name"`
	Command       yaml.Node      `yaml:"command"`
	Ports         []yaml.Node    `yaml:"ports"`
	Expose        []yaml.Node    `yaml:"expose"`
	Devices       []string       `yaml:"devices"`  // AMD: /dev/kfd, /dev/dri/renderD*
	GroupAdd      []string       `yaml:"group_add"`
	Deploy        composeDeploy  `yaml:"deploy"` // NVIDIA passthrough lives here
}

type composeDeploy struct {
	Resources composeResources `yaml:"resources"`
}

type composeResources struct {
	Reservations composeReservations `yaml:"reservations"`
}

type composeReservations struct {
	Devices []composeReservationDevice `yaml:"devices"`
}

type composeReservationDevice struct {
	Driver       string   `yaml:"driver"`
	DeviceIDs    []string `yaml:"device_ids"`
	Count        any      `yaml:"count"` // can be int or "all"
	Capabilities []string `yaml:"capabilities"`
}

func (s *composeService) hasNVIDIAGPUDecl() bool {
	for _, d := range s.Deploy.Resources.Reservations.Devices {
		if strings.EqualFold(d.Driver, "nvidia") {
			return true
		}
	}
	return false
}

// nvidiaDeviceIDs returns the device_ids from NVIDIA reservations, as
// strings. If `count` is used instead of `device_ids`, we fall back to
// generating synthetic IDs ("count:0", "count:1", ...) so the count is
// still correct even though specific GPU indices are unknown.
func (s *composeService) nvidiaDeviceIDs() []string {
	var out []string
	for _, d := range s.Deploy.Resources.Reservations.Devices {
		if !strings.EqualFold(d.Driver, "nvidia") {
			continue
		}
		if len(d.DeviceIDs) > 0 {
			out = append(out, d.DeviceIDs...)
			continue
		}
		if d.Count != nil {
			n := coerceCount(d.Count)
			for i := 0; i < n; i++ {
				out = append(out, fmt.Sprintf("count:%d", i))
			}
		}
	}
	return out
}

func (s *composeService) hasAMDGPUDecl() bool {
	hasKFD := false
	hasRender := false
	for _, d := range s.Devices {
		if d == "/dev/kfd" {
			hasKFD = true
		}
		if strings.HasPrefix(d, "/dev/dri/render") {
			hasRender = true
		}
	}
	return hasKFD || hasRender
}

func (s *composeService) amdRenderNodes() []string {
	var out []string
	for _, d := range s.Devices {
		if strings.HasPrefix(d, "/dev/dri/render") {
			out = append(out, d)
		}
	}
	return out
}

func (s *composeService) commandTokens() []string {
	switch s.Command.Kind {
	case yaml.ScalarNode:
		// String form — split on whitespace. Compose semantics actually
		// pass it through /bin/sh -c, but for the purpose of extracting a
		// --served-model-name flag, whitespace splitting is good enough.
		return strings.Fields(s.Command.Value)
	case yaml.SequenceNode:
		out := make([]string, 0, len(s.Command.Content))
		for _, n := range s.Command.Content {
			out = append(out, n.Value)
		}
		return out
	}
	return nil
}

// extractModelName looks for --served-model-name first (preferred — it's
// what API callers will use), then falls back to --model with basename
// extraction.
func extractModelName(tokens []string) string {
	if name := flagValue(tokens, "--served-model-name"); name != "" {
		return name
	}
	if m := flagValue(tokens, "--model"); m != "" {
		// Basename: "Qwen/Qwen3-Embedding-8B" -> "Qwen3-Embedding-8B".
		// Normalised to lowercase for case-insensitive routing match.
		return strings.ToLower(path.Base(m))
	}
	return ""
}

// flagValue returns the value following the first occurrence of flagName,
// or empty string if not present. Handles both "--flag value" and
// "--flag=value".
func flagValue(tokens []string, flagName string) string {
	for i, tok := range tokens {
		if tok == flagName && i+1 < len(tokens) {
			return tokens[i+1]
		}
		if strings.HasPrefix(tok, flagName+"=") {
			return strings.TrimPrefix(tok, flagName+"=")
		}
	}
	return ""
}

// extractInternalPort returns the host-mapped port from a service's
// `ports:` mapping. The inference-proxy runs in the outer sandbox network
// namespace; it can't resolve the inner dockerd's container DNS, so it
// reaches services via 127.0.0.1:<host_port> exposed by docker compose
// `ports:` mappings. Falls back to `expose:` (container port) if no
// host port mapping is declared, but in practice the inference-proxy
// requires host mappings to work.
//
// Handles the port-spec forms:
//   - "127.0.0.1:8000:8001"  -> 8000  (host port — middle chunk)
//   - "8000:8001"            -> 8000  (host port — first chunk)
//   - "8000"                 -> 8000  (only one port, treat as host)
//   - 8000                   -> 8000
//   - {published: 8000, target: 8001} -> 8000
//
// The container port (the LAST chunk) is no longer extracted because
// nothing in the API server / inference-proxy path uses it.
func extractInternalPort(ports, expose []yaml.Node) int {
	for _, n := range ports {
		if p := hostPortFromNode(n); p > 0 {
			return p
		}
	}
	for _, n := range expose {
		// `expose:` declares container ports only. As a degraded
		// fallback we return that — the inference-proxy will fail with
		// connection refused, which is a cleaner error than 0.
		if p := containerPortFromNode(n); p > 0 {
			return p
		}
	}
	return 0
}

// hostPortFromNode returns the host-side port from a compose `ports:`
// entry. The host port is whichever is on the left of the colon: with
// "ip:host:container" it's the middle, with "host:container" it's the
// first, with bare "8000" or 8000 it's the value.
func hostPortFromNode(n yaml.Node) int {
	switch n.Kind {
	case yaml.ScalarNode:
		s := strings.TrimSpace(n.Value)
		// Strip trailing "/tcp" or "/udp" if present.
		if i := strings.Index(s, "/"); i >= 0 {
			s = s[:i]
		}
		// Split on colon. Forms:
		//   "host"                 -> [host]
		//   "host:container"       -> [host, container]
		//   "ip:host:container"    -> [ip, host, container]
		parts := strings.Split(s, ":")
		var hostStr string
		switch len(parts) {
		case 1:
			hostStr = parts[0]
		case 2:
			hostStr = parts[0]
		case 3:
			hostStr = parts[1]
		default:
			return 0
		}
		// May be a range "8000-8005" — take the first.
		if i := strings.Index(hostStr, "-"); i >= 0 {
			hostStr = hostStr[:i]
		}
		v, err := strconv.Atoi(strings.TrimSpace(hostStr))
		if err != nil {
			return 0
		}
		return v
	case yaml.MappingNode:
		// Look for `published: NNNN`. (Falls back to `target` if
		// `published` is absent — in that case host = container.)
		var target int
		for i := 0; i < len(n.Content)-1; i += 2 {
			k, v := n.Content[i], n.Content[i+1]
			if v.Kind != yaml.ScalarNode {
				continue
			}
			if k.Value == "published" {
				if p, err := strconv.Atoi(v.Value); err == nil {
					return p
				}
			}
			if k.Value == "target" {
				if p, err := strconv.Atoi(v.Value); err == nil {
					target = p
				}
			}
		}
		return target
	}
	return 0
}

func containerPortFromNode(n yaml.Node) int {
	switch n.Kind {
	case yaml.ScalarNode:
		s := strings.TrimSpace(n.Value)
		if i := strings.LastIndex(s, ":"); i >= 0 {
			s = s[i+1:]
		}
		if i := strings.Index(s, "/"); i >= 0 {
			s = s[:i]
		}
		if i := strings.Index(s, "-"); i >= 0 {
			s = s[:i]
		}
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil {
			return 0
		}
		return v
	}
	return 0
}

// coerceCount handles the deploy.resources.reservations.devices.count field
// which compose allows to be either an int or the literal string "all".
// "all" is unhelpful for derived count purposes (we don't know how many
// GPUs the host has at YAML-parse time), so we treat it as 1.
func coerceCount(v any) int {
	switch x := v.(type) {
	case int:
		return x
	case int64:
		return int(x)
	case float64:
		return int(x)
	case string:
		if x == "all" {
			return 1
		}
		if n, err := strconv.Atoi(x); err == nil {
			return n
		}
	}
	return 0
}

// mappingChildren returns the key/value pairs of a YAML mapping node in
// source order. Used to walk the top-level of a compose document while
// preserving service order.
type mappingKV struct {
	key   string
	value *yaml.Node
}

func mappingChildren(n *yaml.Node) []mappingKV {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	out := make([]mappingKV, 0, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		out = append(out, mappingKV{key: n.Content[i].Value, value: n.Content[i+1]})
	}
	return out
}
