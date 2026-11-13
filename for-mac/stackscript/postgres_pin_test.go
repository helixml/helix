package stackscript

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDocker stands in for docker inside the VM: one compose project
// ("helix"), a postgres volume whose PG_VERSION is $FAKE_PG_VERSION (none
// when empty), and `compose ps` listing api when $FAKE_RUNNING is set.
const fakeDocker = `#!/bin/bash
echo "docker $*" >> "$FAKE_LOG"
case "$1 $2" in
  "volume ls")
    [ -n "$FAKE_PG_VERSION" ] && [[ "$*" == *project=helix\ * || "$*" == *project=helix ]] && echo helix_helix-postgres-db
    exit 0 ;;
  "volume inspect")
    [ -n "$FAKE_PG_VERSION" ] || exit 1
    [ "$3" = "-f" ] && echo "$FAKE_VOLUME_DIR"
    exit 0 ;;
  "image ls") exit 0 ;;
  "run --rm") exit 1 ;;
esac
if [ "$1" = compose ]; then
  case "$*" in
    *" ps "*) [ -n "$FAKE_RUNNING" ] && echo api; exit 0 ;;
    *" up -d"*) echo "up for $(grep '^HELIX_POSTGRES_IMAGE=' .env || echo default)"; exit 0 ;;
  esac
fi
exit 0
`

type stackRun struct {
	out    string
	log    string
	envVM  string
	upCall bool
}

func runStackStart(t *testing.T, composeFile, pgVersion string, running, prePinned bool) stackRun {
	t.Helper()
	home := t.TempDir()
	helix := filepath.Join(home, "helix")
	bin := filepath.Join(home, "bin")
	volume := filepath.Join(home, "volumes", "helix_helix-postgres-db", "_data")
	for _, dir := range []string{helix, bin, volume} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path, content string, mode os.FileMode) {
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(bin, "docker"), fakeDocker, 0o755)
	write(filepath.Join(bin, "sudo"), "#!/bin/sh\nexit 1\n", 0o755)
	write(filepath.Join(helix, composeFile), "services: {}\n", 0o644)
	env := "RUNNER_TOKEN=x\n"
	if prePinned {
		env += "HELIX_POSTGRES_IMAGE=postgres:12.22-alpine\n"
	}
	// Only .env.vm: the script links .env to it, as on a first boot.
	write(filepath.Join(helix, ".env.vm"), env, 0o644)
	if pgVersion != "" {
		write(filepath.Join(volume, "PG_VERSION"), pgVersion+"\n", 0o600)
	}

	logPath := filepath.Join(home, "docker.log")
	cmd := exec.Command("bash", "-c", Start())
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"PATH="+bin+":"+os.Getenv("PATH"),
		"FAKE_LOG="+logPath,
		"FAKE_PG_VERSION="+pgVersion,
		"FAKE_VOLUME_DIR="+volume,
		"FAKE_RUNNING="+map[bool]string{true: "1"}[running],
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("script failed: %v\n%s", err, out)
	}
	logData, _ := os.ReadFile(logPath)
	envVM, _ := os.ReadFile(filepath.Join(helix, ".env.vm"))
	return stackRun{
		out:    string(out),
		log:    string(logData),
		envVM:  string(envVM),
		upCall: strings.Contains(string(logData), "up -d"),
	}
}

func TestStartScriptPinsPostgres12Volume(t *testing.T) {
	for _, compose := range []string{"docker-compose.yaml", "docker-compose.dev.yaml"} {
		t.Run(compose, func(t *testing.T) {
			r := runStackStart(t, compose, "12", false, false)
			if !strings.Contains(r.envVM, "\nHELIX_POSTGRES_IMAGE=postgres:12.22-alpine\n") {
				t.Fatalf("PG12 volume not pinned:\n%s\noutput:\n%s", r.envVM, r.out)
			}
			if !strings.Contains(r.out, "up for HELIX_POSTGRES_IMAGE=postgres:12.22-alpine") {
				t.Fatalf("stack not started with the pin:\n%s", r.out)
			}
			if !strings.Contains(r.out, "COMPOSE_FILE="+compose) {
				t.Fatalf("wrong compose file:\n%s", r.out)
			}
		})
	}
}

func TestStartScriptRecreatesRunningStackAfterPin(t *testing.T) {
	// Docker restarted the containers on boot, before the pin existed.
	for _, compose := range []string{"docker-compose.yaml", "docker-compose.dev.yaml"} {
		t.Run(compose, func(t *testing.T) {
			r := runStackStart(t, compose, "12", true, false)
			if strings.Contains(r.out, "ALREADY_RUNNING") || !r.upCall {
				t.Fatalf("running stack not recreated after pinning:\n%s", r.out)
			}
		})
	}
}

func TestStartScriptLeavesPinnedRunningStack(t *testing.T) {
	r := runStackStart(t, "docker-compose.yaml", "12", true, true)
	if !strings.Contains(r.out, "ALREADY_RUNNING") || r.upCall {
		t.Fatalf("pinned running stack should be left alone:\n%s", r.out)
	}
	if strings.Count(r.envVM, "HELIX_POSTGRES_IMAGE=") != 1 {
		t.Fatalf("pin duplicated:\n%s", r.envVM)
	}
}

func TestStartScriptFreshVolumeUsesDefault(t *testing.T) {
	for _, pg := range []string{"", "17"} {
		r := runStackStart(t, "docker-compose.yaml", pg, false, false)
		if strings.Contains(r.envVM, "HELIX_POSTGRES_IMAGE") {
			t.Fatalf("PG_VERSION %q should not be pinned:\n%s", pg, r.envVM)
		}
		if !strings.Contains(r.out, "up for default") {
			t.Fatalf("stack not started:\n%s", r.out)
		}
		if strings.Contains(r.log, "docker run") {
			t.Fatalf("PG_VERSION %q read through a container:\n%s", pg, r.log)
		}
	}
}

func TestPostgresPinShellMatchesInstallSh(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "install.sh"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	start := strings.Index(s, "# BEGIN postgres-pin")
	end := strings.Index(s, "# END postgres-pin\n")
	if start < 0 || end < 0 {
		t.Fatal("postgres-pin markers missing from install.sh")
	}
	if block := s[start : end+len("# END postgres-pin\n")]; block != postgresPinShell {
		t.Fatal("for-mac/stackscript/postgres_pin.go differs from install.sh's postgres-pin block; copy it again")
	}
}
