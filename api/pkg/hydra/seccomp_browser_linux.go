//go:build linux

package hydra

import (
	"encoding/json"
	"fmt"

	"github.com/docker/docker/profiles/seccomp"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

// browserSandboxSeccomp is Docker's default seccomp profile, plus the
// namespace creation Chrome's renderer sandbox needs. Docker allows clone
// with namespace flags and unshare only to CAP_SYS_ADMIN, which an
// unprivileged container must not hold; without them Chrome's zygote dies
// with "Failed to move to new namespace". Everything else the default
// profile blocks stays blocked.
func browserSandboxSeccomp() (string, error) {
	profile := seccomp.DefaultProfile()
	for _, rule := range profile.Syscalls {
		if len(rule.Names) == 1 && rule.Names[0] == "clone" && len(rule.Args) > 0 {
			rule.Args = nil
		}
	}
	profile.Syscalls = append(profile.Syscalls, &seccomp.Syscall{
		LinuxSyscall: specs.LinuxSyscall{Names: []string{"unshare"}, Action: specs.ActAllow},
		Comment:      "Chrome's renderer sandbox creates user namespaces",
	})
	encoded, err := json.Marshal(profile)
	if err != nil {
		return "", fmt.Errorf("encode browser sandbox seccomp profile: %w", err)
	}
	return string(encoded), nil
}
