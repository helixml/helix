package provision

import (
	"fmt"
	"strings"
)

// cloudInit returns a bash script that the provider runs at first boot.
// Both Hot Aisle and Verda ship images with Docker + the right GPU
// runtime preinstalled (NVIDIA Container Toolkit for Verda's NVIDIA
// images, ROCm + amdgpu group for Hot Aisle), so the script's job is
// just: pull the helix-sandbox image and run it with the GPU mounts and
// env vars the harness needs.
//
// `vendor` must be "nvidia" or "amd". The two diverge only in the
// `docker run` flags — runtime selection for NVIDIA, device mounts for
// AMD.
func cloudInit(spec PodSpec, helixAPIURL, runnerToken, vendor string) string {
	var dockerRun []string
	dockerRun = append(dockerRun,
		"docker run -d --restart unless-stopped",
		"--name helix-sandbox",
		"--network host", // sandbox listens on :8081, easier than NAT
		"-e HELIX_API_URL="+shellQuote(helixAPIURL),
		"-e RUNNER_TOKEN="+shellQuote(runnerToken),
		"-e SANDBOX_INSTANCE_ID="+shellQuote("it-"+spec.EntryID),
		"-e GPU_VENDOR="+vendor,
	)
	switch vendor {
	case "nvidia":
		dockerRun = append(dockerRun,
			"--runtime=nvidia",
			"--gpus all",
		)
	case "amd":
		// AMD: passthrough kfd + dri devices; add the video and render
		// groups (host UID for `video` is conventionally 44, `render`
		// 109; the sandbox image's uid maps cover both).
		dockerRun = append(dockerRun,
			"--device=/dev/kfd",
			"--device=/dev/dri",
			"--group-add video",
			"--group-add render",
			"--security-opt seccomp=unconfined", // ROCm needs perf_event_open
		)
	}
	dockerRun = append(dockerRun, shellQuote(spec.ImageRef))

	return fmt.Sprintf(`#!/bin/bash
set -euxo pipefail

# Sanity check: image must exist on the host before we boot the sandbox.
docker pull %s

# Auto-terminate guard: cron-style shutdown at +35min so a stuck sandbox
# can't leak GPU spend even if the harness's teardown fails.
nohup bash -c 'sleep 2100 && shutdown -h now' >/dev/null 2>&1 &

%s
`,
		shellQuote(spec.ImageRef),
		strings.Join(dockerRun, " \\\n  "),
	)
}

// shellQuote single-quotes a string for safe inclusion in a bash command.
// Single quotes don't expand anything, so we just need to escape any
// embedded single quotes.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
