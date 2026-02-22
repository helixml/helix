// docker-wrapper intercepts docker build/compose build commands and routes them
// through the shared BuildKit builder with smart --load (skips image export when
// unchanged). Installed at /usr/local/bin/docker ahead of /usr/bin/docker in PATH.
//
// Modes:
//  1. docker compose ... build → decompose into individual buildx build calls
//  2. docker build ...        → rewrite to docker buildx build (shared BuildKit)
//  3. docker buildx build ... → smart --load with digest comparison
//  4. anything else           → exec /usr/bin/docker (transparent passthrough)
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

const realDocker = "/usr/bin/docker"

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		passthrough(args)
		return
	}

	switch {
	case args[0] == "compose":
		handleCompose(args)
	case args[0] == "build":
		// docker build ... → docker buildx build ...
		handleBuild(args[1:])
	case args[0] == "buildx" && len(args) > 1 && args[1] == "build":
		// docker buildx build ... → smart --load
		handleBuild(args[2:])
	default:
		passthrough(args)
	}
}

// passthrough replaces the process with the real docker binary.
func passthrough(args []string) {
	argv := append([]string{realDocker}, args...)
	err := syscall.Exec(realDocker, argv, os.Environ())
	// Exec only returns on error
	fmt.Fprintf(os.Stderr, "[docker-wrapper] exec failed: %v\n", err)
	os.Exit(1)
}

// run executes a command and returns stdout, exit code, and any error.
func run(name string, args ...string) (string, int, error) {
	cmd := exec.Command(name, args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return string(out), exitErr.ExitCode(), nil
		}
		return "", 1, err
	}
	return strings.TrimSpace(string(out)), 0, nil
}

// runPassthrough runs a command with stdio inherited.
func runPassthrough(name string, args ...string) int {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return exitErr.ExitCode()
		}
		return 1
	}
	return 0
}

// getBuilderDriver returns the buildx builder driver type ("remote", "docker", etc).
// Cached in _DOCKER_WRAPPER_DRIVER env var across recursive invocations.
func getBuilderDriver() string {
	if d := os.Getenv("_DOCKER_WRAPPER_DRIVER"); d != "" {
		return d
	}

	var inspectArgs []string
	if b := os.Getenv("BUILDX_BUILDER"); b != "" {
		inspectArgs = []string{"buildx", "inspect", b}
	} else {
		inspectArgs = []string{"buildx", "inspect"}
	}

	out, _, err := run(realDocker, inspectArgs...)
	driver := "docker"
	if err == nil {
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(line, "Driver:") {
				d := strings.TrimSpace(strings.TrimPrefix(line, "Driver:"))
				if d != "" {
					driver = d
				}
				break
			}
		}
	}

	os.Setenv("_DOCKER_WRAPPER_DRIVER", driver)
	return driver
}

// --- Compose handling ---

type composeConfig struct {
	Name     string                    `json:"name"`
	Services map[string]composeService `json:"services"`
}

type composeService struct {
	Image string        `json:"image"`
	Build *composeBuild `json:"build"`
}

type composeBuild struct {
	Context    string                 `json:"context"`
	Dockerfile string                 `json:"dockerfile"`
	Target     string                 `json:"target"`
	Args       map[string]interface{} `json:"args"`
}

func handleCompose(args []string) {
	// Scan for "build" subcommand
	hasBuild := false
	for _, a := range args {
		if a == "build" {
			hasBuild = true
			break
		}
	}
	if !hasBuild {
		passthrough(args)
		return
	}

	driver := getBuilderDriver()
	if driver != "remote" {
		passthrough(args)
		return
	}

	// Parse: compose flags (before "build"), build flags & services (after "build")
	var composeFlags, buildFlags, services []string
	phase := "compose"
	skipNext := false

	for _, a := range args[1:] { // skip "compose"
		if skipNext {
			if phase == "compose" {
				composeFlags = append(composeFlags, a)
			} else {
				buildFlags = append(buildFlags, a)
			}
			skipNext = false
			continue
		}

		if phase == "compose" {
			if a == "build" {
				phase = "build"
				continue
			}
			composeFlags = append(composeFlags, a)
			switch a {
			case "-f", "--file", "-p", "--project-name", "--project-directory", "--env-file":
				skipNext = true
			}
		} else {
			switch {
			case a == "--no-cache" || a == "--pull" || a == "--quiet" || a == "-q":
				buildFlags = append(buildFlags, a)
			case strings.HasPrefix(a, "--progress="):
				buildFlags = append(buildFlags, a)
			case a == "--progress":
				buildFlags = append(buildFlags, a)
				skipNext = true
			case strings.HasPrefix(a, "-"):
				// ignore other compose build flags
			default:
				services = append(services, a)
			}
		}
	}

	// Get compose config as JSON
	configArgs := append([]string{"compose"}, composeFlags...)
	configArgs = append(configArgs, "config", "--format", "json")
	cfgJSON, rc, err := run(realDocker, configArgs...)
	if err != nil || rc != 0 || cfgJSON == "" {
		fmt.Fprintln(os.Stderr, "[docker-wrapper] Failed to parse compose config, falling back")
		fallbackArgs := append([]string{"compose"}, composeFlags...)
		fallbackArgs = append(fallbackArgs, "build")
		fallbackArgs = append(fallbackArgs, buildFlags...)
		fallbackArgs = append(fallbackArgs, services...)
		passthrough(fallbackArgs)
		return
	}

	var cfg composeConfig
	if err := json.Unmarshal([]byte(cfgJSON), &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "[docker-wrapper] Failed to parse compose JSON: %v, falling back\n", err)
		fallbackArgs := append([]string{"compose"}, composeFlags...)
		fallbackArgs = append(fallbackArgs, "build")
		fallbackArgs = append(fallbackArgs, buildFlags...)
		fallbackArgs = append(fallbackArgs, services...)
		passthrough(fallbackArgs)
		return
	}

	// Filter services with build sections
	serviceFilter := make(map[string]bool)
	for _, s := range services {
		serviceFilter[s] = true
	}

	var svcList []string
	for name, svc := range cfg.Services {
		if svc.Build == nil {
			continue
		}
		if len(serviceFilter) > 0 && !serviceFilter[name] {
			continue
		}
		svcList = append(svcList, name)
	}

	if len(svcList) == 0 {
		fmt.Fprintln(os.Stderr, "[docker-wrapper] No services with build sections")
		os.Exit(0)
	}

	fmt.Fprintf(os.Stderr, "[docker-wrapper] compose build: %d service(s) via smart --load\n", len(svcList))

	for _, svcName := range svcList {
		svc := cfg.Services[svcName]
		b := svc.Build

		ctx := b.Context
		if ctx == "" {
			ctx = "."
		}
		dockerfile := b.Dockerfile
		if dockerfile == "" {
			dockerfile = "Dockerfile"
		}

		img := svc.Image
		if img == "" {
			img = cfg.Name + "-" + svcName
		}

		var buildArgs []string
		buildArgs = append(buildArgs, "-t", img, "-f", ctx+"/"+dockerfile)
		if b.Target != "" {
			buildArgs = append(buildArgs, "--target", b.Target)
		}
		for k, v := range b.Args {
			// Args can be string or null (env-sourced)
			switch val := v.(type) {
			case string:
				buildArgs = append(buildArgs, "--build-arg", k+"="+val)
			default:
				// null or other → pass key only (docker resolves from env)
				buildArgs = append(buildArgs, "--build-arg", k)
			}
		}
		buildArgs = append(buildArgs, buildFlags...)
		buildArgs = append(buildArgs, ctx)

		fmt.Fprintf(os.Stderr, "[docker-wrapper]   %s → %s\n", svcName, img)

		// Recursive: call ourselves with "buildx build ..."
		selfArgs := append([]string{"buildx", "build"}, buildArgs...)
		rc := runPassthrough(os.Args[0], selfArgs...)
		if rc != 0 {
			fmt.Fprintf(os.Stderr, "[docker-wrapper] Failed: %s (exit %d)\n", svcName, rc)
			os.Exit(1)
		}
	}

	os.Exit(0)
}

// --- Build handling ---

func handleBuild(args []string) {
	driver := getBuilderDriver()

	if driver != "remote" {
		// Non-remote builder: just use buildx build directly
		passthrough(append([]string{"buildx", "build"}, args...))
		return
	}

	// Remote builder: extract tags and check for explicit output flags
	var tags []string
	hasTag := false
	hasOutput := false
	nextIsTag := false

	for _, arg := range args {
		if nextIsTag {
			tags = append(tags, arg)
			nextIsTag = false
			continue
		}
		switch {
		case arg == "-t" || arg == "--tag":
			hasTag = true
			nextIsTag = true
		case arg == "--output" || strings.HasPrefix(arg, "--output=") || arg == "--load" || arg == "--push":
			hasOutput = true
		}
	}

	// If user specified explicit output or no tag, pass through
	if hasOutput || !hasTag {
		passthrough(append([]string{"buildx", "build"}, args...))
		return
	}

	// Check if all tagged images exist in local daemon
	allLocal := true
	for _, tag := range tags {
		out, _, _ := run(realDocker, "images", "-q", tag)
		if out == "" {
			allLocal = false
			break
		}
	}

	if !allLocal {
		// Image not in local daemon — must load it
		if reg := os.Getenv("HELIX_REGISTRY"); reg != "" {
			loadViaRegistry(tags, args)
			return
		}
		passthrough(append([]string{"buildx", "build"}, append(args, "--load")...))
		return
	}

	// Image exists locally — quick build with --output type=image to check digest
	iidFile, err := os.CreateTemp("", "buildx-iid-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[docker-wrapper] Failed to create temp file: %v\n", err)
		passthrough(append([]string{"buildx", "build"}, append(args, "--load")...))
		return
	}
	iidPath := iidFile.Name()
	iidFile.Close()
	defer os.Remove(iidPath)

	probeArgs := append([]string{"buildx", "build"}, args...)
	probeArgs = append(probeArgs, "--output", "type=image", "--provenance=false", "--iidfile", iidPath)
	rc := runPassthrough(realDocker, probeArgs...)
	if rc != 0 {
		os.Exit(rc)
	}

	newID := ""
	if data, err := os.ReadFile(iidPath); err == nil {
		newID = strings.TrimSpace(string(data))
	}

	if newID == "" {
		// Couldn't determine digest — fall back to loading
		if reg := os.Getenv("HELIX_REGISTRY"); reg != "" {
			loadViaRegistry(tags, args)
			return
		}
		passthrough(append([]string{"buildx", "build"}, append(args, "--load")...))
		return
	}

	// Compare buildx digest with local daemon's image ID
	needLoad := false
	for _, tag := range tags {
		out, _, _ := run(realDocker, "images", "--no-trunc", "-q", tag)
		localID := strings.Split(out, "\n")[0]
		if newID != localID {
			needLoad = true
			break
		}
	}

	if needLoad {
		short := newID
		if len(short) > 19 {
			short = short[:19]
		}
		fmt.Fprintf(os.Stderr, "[docker-wrapper] Image changed (new: %s), loading into daemon...\n", short)
		if reg := os.Getenv("HELIX_REGISTRY"); reg != "" {
			loadViaRegistry(tags, args)
			return
		}
		passthrough(append([]string{"buildx", "build"}, append(args, "--load")...))
		return
	}

	short := newID
	if len(short) > 19 {
		short = short[:19]
	}
	fmt.Fprintf(os.Stderr, "[docker-wrapper] Image unchanged (%s), skipping load\n", short)
	os.Exit(0)
}

// loadViaRegistry pushes image to shared registry and pulls into local daemon.
// Only transfers changed layers (~0.6s for a 1-layer change in 7.73GB image).
func loadViaRegistry(tags []string, args []string) {
	reg := os.Getenv("HELIX_REGISTRY")
	regTag := reg + "/buildcache/" + tags[0]

	// Strip -t/--tag flags from args (they conflict with --output name=)
	var buildArgs []string
	skipNext := false
	for _, arg := range args {
		if skipNext {
			skipNext = false
			continue
		}
		if arg == "-t" || arg == "--tag" {
			skipNext = true
			continue
		}
		buildArgs = append(buildArgs, arg)
	}

	// Build and push to registry
	pushArgs := append([]string{"buildx", "build"}, buildArgs...)
	pushArgs = append(pushArgs, "--output", fmt.Sprintf("type=image,name=%s,push=true", regTag), "--provenance=false")
	rc := runPassthrough(realDocker, pushArgs...)
	if rc != 0 {
		fmt.Fprintln(os.Stderr, "[docker-wrapper] Registry push failed, falling back to tarball --load")
		passthrough(append([]string{"buildx", "build"}, append(args, "--load")...))
		return
	}

	// Pull from registry (layer-level dedup)
	rc = runPassthrough(realDocker, "pull", regTag)
	if rc != 0 {
		fmt.Fprintln(os.Stderr, "[docker-wrapper] Registry pull failed, falling back to tarball --load")
		passthrough(append([]string{"buildx", "build"}, append(args, "--load")...))
		return
	}

	// Re-tag to original tag names
	for _, tag := range tags {
		runPassthrough(realDocker, "tag", regTag, tag)
	}

	fmt.Fprintln(os.Stderr, "[docker-wrapper] Loaded via registry (layer-level dedup)")
}
