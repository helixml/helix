package hydra

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/types"
)

// The API sends its golden build deadline; Hydra must use it rather than a
// shorter hardcoded one (previously 30 min vs the API's 6h, which killed every
// cold build and left the cache stale forever).
func TestGoldenBuildTimeout_UsesAPIDeadline(t *testing.T) {
	req := &CreateDevContainerRequest{GoldenBuild: true, GoldenBuildTimeoutSeconds: int(6 * time.Hour / time.Second)}
	if got := goldenBuildTimeout(req); got != 6*time.Hour {
		t.Fatalf("goldenBuildTimeout = %s, want 6h", got)
	}

	// Older APIs don't send a deadline: fall back to the shared constant, not
	// a Hydra-local value that could drift from the API's.
	if got := goldenBuildTimeout(&CreateDevContainerRequest{GoldenBuild: true}); got != types.GoldenBuildTimeout {
		t.Fatalf("goldenBuildTimeout default = %s, want types.GoldenBuildTimeout (%s)", got, types.GoldenBuildTimeout)
	}
}

// The deadline must survive the API -> Hydra wire format.
func TestGoldenBuildTimeout_JSONRoundTrip(t *testing.T) {
	data, err := json.Marshal(&CreateDevContainerRequest{GoldenBuild: true, GoldenBuildTimeoutSeconds: 21600})
	if err != nil {
		t.Fatal(err)
	}
	var req CreateDevContainerRequest
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	if got := goldenBuildTimeout(&req); got != 6*time.Hour {
		t.Fatalf("goldenBuildTimeout after round trip = %s, want 6h", got)
	}
}

func stillRunning() (bool, string) { return false, "" }

// waitForGoldenBuildResult keeps polling until the configured deadline: a
// result that arrives after a short deadline would have been lost, but is
// picked up when the deadline is longer.
func TestWaitForGoldenBuildResult_HonoursConfiguredDeadline(t *testing.T) {
	writeAfter := func(path string, d time.Duration) {
		go func() {
			time.Sleep(d)
			_ = os.WriteFile(path, []byte("0\n"), 0o644)
		}()
	}

	t.Run("result after short deadline times out", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".golden-build-result")
		writeAfter(path, 500*time.Millisecond)
		start := time.Now()
		_, err := waitForGoldenBuildResult(100*time.Millisecond, 10*time.Millisecond, stillRunning, path)
		if !errors.Is(err, errGoldenBuildTimeout) {
			t.Fatalf("err = %v, want errGoldenBuildTimeout", err)
		}
		if elapsed := time.Since(start); elapsed < 100*time.Millisecond {
			t.Fatalf("gave up after %s, before the configured 100ms deadline", elapsed)
		}
	})

	t.Run("result within long deadline is returned", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), ".golden-build-result")
		writeAfter(path, 300*time.Millisecond)
		data, err := waitForGoldenBuildResult(5*time.Second, 10*time.Millisecond, stillRunning, path)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(data) != "0\n" {
			t.Fatalf("data = %q, want %q", data, "0\n")
		}
	})

	t.Run("falls back to second path", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "zvol", ".golden-build-result")
		present := filepath.Join(dir, ".golden-build-result")
		if err := os.WriteFile(present, []byte("1"), 0o644); err != nil {
			t.Fatal(err)
		}
		data, err := waitForGoldenBuildResult(time.Second, 10*time.Millisecond, stillRunning, missing, present)
		if err != nil || string(data) != "1" {
			t.Fatalf("got (%q, %v), want (\"1\", nil)", data, err)
		}
	})
}

// A build container that dies without a result must fail the wait right
// away, not hold the build's data until the hours-long deadline.
func TestWaitForGoldenBuildResult_ContainerExitedWithoutResult(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".golden-build-result")
	exited := func() (bool, string) { return true, "build container exited with code 1" }

	start := time.Now()
	_, err := waitForGoldenBuildResult(time.Hour, 10*time.Millisecond, exited, path)
	if err == nil || errors.Is(err, errGoldenBuildTimeout) {
		t.Fatalf("err = %v, want container-exited error", err)
	}
	if !strings.Contains(err.Error(), "build container exited with code 1 without writing a golden build result") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("took %s to notice the exited container", elapsed)
	}
}

// The container may exit right after writing the result: that's a result,
// not a failure.
func TestWaitForGoldenBuildResult_ResultWrittenJustBeforeExit(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".golden-build-result")
	exited := func() (bool, string) {
		_ = os.WriteFile(path, []byte("0\n"), 0o644)
		return true, "build container exited with code 0"
	}

	data, err := waitForGoldenBuildResult(time.Hour, 10*time.Millisecond, exited, path)
	if err != nil || string(data) != "0\n" {
		t.Fatalf("got (%q, %v), want (\"0\\n\", nil)", data, err)
	}
}
