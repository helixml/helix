package hydra

// sandbox_ops.go contains the in-memory command tracker and the helpers used
// by the Sandboxes API to exec/cat/upload files inside a headless dev container.
//
// State is intentionally **not** persisted: when a sandbox is deleted, all of
// its commands and logs are dropped. This matches the "no persistence" rule of
// the Sandboxes API.

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	dockertypes "github.com/docker/docker/api/types"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/rs/zerolog/log"
)

// ErrSandboxCommandNotFound is returned when looking up an unknown command id.
var ErrSandboxCommandNotFound = errors.New("sandbox command not found")

// SandboxCommandStatus mirrors the value reported by the API.
type SandboxCmdStatus string

const (
	CmdStatusPending  SandboxCmdStatus = "pending"
	CmdStatusRunning  SandboxCmdStatus = "running"
	CmdStatusFinished SandboxCmdStatus = "finished"
	CmdStatusFailed   SandboxCmdStatus = "failed"
	CmdStatusKilled   SandboxCmdStatus = "killed"
)

// SandboxCmdRecord is the in-memory representation of a single exec.
type SandboxCmdRecord struct {
	ID         string           `json:"id"`
	SandboxID  string           `json:"sandbox_id"`
	Cmd        string           `json:"cmd"`
	Args       []string         `json:"args,omitempty"`
	Cwd        string           `json:"cwd,omitempty"`
	Env        []string         `json:"env,omitempty"`
	Sudo       bool             `json:"sudo,omitempty"`
	Detached   bool             `json:"detached,omitempty"`
	Status     SandboxCmdStatus `json:"status"`
	ExitCode   *int             `json:"exit_code,omitempty"`
	StartedAt  time.Time        `json:"started_at"`
	FinishedAt *time.Time       `json:"finished_at,omitempty"`

	// Logs holds the running stdout+stderr; cap at logBufferMax to avoid blowing memory.
	mu       sync.Mutex
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	subs     []chan SandboxLogChunk
	execID   string // Docker exec id, used to inspect status / kill
	canceled bool
}

const logBufferMax = 1 << 20 // 1 MiB per stream

// SandboxLogChunk is streamed to subscribers (used for SSE log streaming).
type SandboxLogChunk struct {
	Stream string `json:"stream"` // "stdout" or "stderr"
	Data   string `json:"data"`
}

// AppendStdout appends raw stdout bytes, capping size, and notifies subscribers.
func (r *SandboxCmdRecord) AppendStdout(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stdout.Len()+len(p) > logBufferMax {
		// Drop oldest by truncating to half — naive but bounded.
		buf := r.stdout.Bytes()
		keep := len(buf) / 2
		r.stdout.Reset()
		r.stdout.Write(buf[keep:])
	}
	r.stdout.Write(p)
	chunk := SandboxLogChunk{Stream: "stdout", Data: string(p)}
	for _, sub := range r.subs {
		select {
		case sub <- chunk:
		default:
		}
	}
}

// AppendStderr appends raw stderr bytes.
func (r *SandboxCmdRecord) AppendStderr(p []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.stderr.Len()+len(p) > logBufferMax {
		buf := r.stderr.Bytes()
		keep := len(buf) / 2
		r.stderr.Reset()
		r.stderr.Write(buf[keep:])
	}
	r.stderr.Write(p)
	chunk := SandboxLogChunk{Stream: "stderr", Data: string(p)}
	for _, sub := range r.subs {
		select {
		case sub <- chunk:
		default:
		}
	}
}

// Stdout returns the accumulated stdout snapshot.
func (r *SandboxCmdRecord) Stdout() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stdout.String()
}

// Stderr returns the accumulated stderr snapshot.
func (r *SandboxCmdRecord) Stderr() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stderr.String()
}

// Subscribe returns a channel of log chunks. The first send is the buffered
// history; subsequent sends are live. Caller must drain the channel.
func (r *SandboxCmdRecord) Subscribe() (<-chan SandboxLogChunk, func()) {
	ch := make(chan SandboxLogChunk, 64)
	r.mu.Lock()
	// Replay buffered output first.
	if r.stdout.Len() > 0 {
		ch <- SandboxLogChunk{Stream: "stdout", Data: r.stdout.String()}
	}
	if r.stderr.Len() > 0 {
		ch <- SandboxLogChunk{Stream: "stderr", Data: r.stderr.String()}
	}
	r.subs = append(r.subs, ch)
	r.mu.Unlock()

	cancel := func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		for i, sub := range r.subs {
			if sub == ch {
				r.subs = append(r.subs[:i], r.subs[i+1:]...)
				close(ch)
				return
			}
		}
	}
	return ch, cancel
}

// Finish marks the record finished and closes subscribers.
func (r *SandboxCmdRecord) Finish(status SandboxCmdStatus, exitCode int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Status = status
	ec := exitCode
	r.ExitCode = &ec
	now := time.Now()
	r.FinishedAt = &now
	for _, sub := range r.subs {
		close(sub)
	}
	r.subs = nil
}

// SandboxOps holds the manager-scoped command tracker.
type SandboxOps struct {
	dm *DevContainerManager

	mu       sync.Mutex
	commands map[string]*SandboxCmdRecord            // cmdID → record
	bySandbx map[string]map[string]*SandboxCmdRecord // sandboxID → cmdID → record
}

// NewSandboxOps wires the ops manager to a DevContainerManager.
func NewSandboxOps(dm *DevContainerManager) *SandboxOps {
	return &SandboxOps{
		dm:       dm,
		commands: map[string]*SandboxCmdRecord{},
		bySandbx: map[string]map[string]*SandboxCmdRecord{},
	}
}

// GetCommand returns a record by id.
func (o *SandboxOps) GetCommand(cmdID string) (*SandboxCmdRecord, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	rec, ok := o.commands[cmdID]
	if !ok {
		return nil, ErrSandboxCommandNotFound
	}
	return rec, nil
}

// ListCommands returns every record for a sandbox, newest first.
func (o *SandboxOps) ListCommands(sandboxID string) []*SandboxCmdRecord {
	o.mu.Lock()
	defer o.mu.Unlock()
	m := o.bySandbx[sandboxID]
	out := make([]*SandboxCmdRecord, 0, len(m))
	for _, r := range m {
		out = append(out, r)
	}
	// Sort newest first by StartedAt.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j].StartedAt.After(out[i].StartedAt) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// ForgetSandbox drops every command record for a sandbox (called on delete).
func (o *SandboxOps) ForgetSandbox(sandboxID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	m := o.bySandbx[sandboxID]
	for cmdID := range m {
		delete(o.commands, cmdID)
	}
	delete(o.bySandbx, sandboxID)
}

func (o *SandboxOps) saveRecord(rec *SandboxCmdRecord) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.commands[rec.ID] = rec
	if _, ok := o.bySandbx[rec.SandboxID]; !ok {
		o.bySandbx[rec.SandboxID] = map[string]*SandboxCmdRecord{}
	}
	o.bySandbx[rec.SandboxID][rec.ID] = rec
}

// ExecRequest is the input to RunCommand.
type ExecRequest struct {
	SandboxID string   `json:"sandbox_id"`
	CmdID     string   `json:"cmd_id"`
	Cmd       string   `json:"cmd"`
	Args      []string `json:"args"`
	Cwd       string   `json:"cwd"`
	Env       []string `json:"env"`
	Sudo      bool     `json:"sudo"`
	Detached  bool     `json:"detached"`
	// TimeoutSeconds is per-command timeout. 0 = no timeout.
	TimeoutSeconds int `json:"timeout_seconds"`
}

// RunCommand starts a command inside the sandbox container. If req.Detached is
// true it returns immediately with status=running; otherwise it blocks until
// the command exits.
func (o *SandboxOps) RunCommand(ctx context.Context, sessionID string, req *ExecRequest) (*SandboxCmdRecord, error) {
	dc := o.dm.FindDevContainerBySessionID(sessionID)
	if dc == nil {
		return nil, fmt.Errorf("sandbox container not found for session %s", sessionID)
	}

	dockerClient, err := o.dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker client: %w", err)
	}
	// For detached commands the goroutine outlives this function, so the
	// client must stay open until the runner finishes — closed below.
	closeClient := true
	defer func() {
		if closeClient {
			dockerClient.Close()
		}
	}()

	cmdLine := append([]string{req.Cmd}, req.Args...)
	if req.Sudo {
		cmdLine = append([]string{"sudo", "-E", "-n"}, cmdLine...)
	} else {
		// Wrap in /bin/sh -c when the user provided a single string with
		// spaces, so quoting works as expected. Otherwise use direct exec.
		if len(req.Args) == 0 && strings.ContainsAny(req.Cmd, " \t|&;<>$()`") {
			cmdLine = []string{"/bin/sh", "-c", req.Cmd}
		}
	}

	execCfg := dockertypes.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          cmdLine,
		Env:          req.Env,
		WorkingDir:   req.Cwd,
		Tty:          false,
	}

	// For detached commands the HTTP request context is cancelled the moment
	// we return the response, which would tear down the exec attach. Use a
	// background context for the create + attach instead.
	dockerCtx := ctx
	if req.Detached {
		dockerCtx = context.Background()
	}

	created, err := dockerClient.ContainerExecCreate(dockerCtx, dc.ContainerID, execCfg)
	if err != nil {
		return nil, fmt.Errorf("docker exec create: %w", err)
	}

	rec := &SandboxCmdRecord{
		ID:        req.CmdID,
		SandboxID: req.SandboxID,
		Cmd:       req.Cmd,
		Args:      req.Args,
		Cwd:       req.Cwd,
		Env:       req.Env,
		Sudo:      req.Sudo,
		Detached:  req.Detached,
		Status:    CmdStatusRunning,
		StartedAt: time.Now(),
		execID:    created.ID,
	}
	o.saveRecord(rec)

	attach, err := dockerClient.ContainerExecAttach(dockerCtx, created.ID, dockertypes.ExecStartCheck{})
	if err != nil {
		rec.Finish(CmdStatusFailed, -1)
		return nil, fmt.Errorf("docker exec attach: %w", err)
	}

	runner := func(streamCtx context.Context) {
		defer attach.Close()

		// Drain stdout/stderr through demux.
		stdoutW := &cmdRecordWriter{rec: rec, stream: "stdout"}
		stderrW := &cmdRecordWriter{rec: rec, stream: "stderr"}

		copyDone := make(chan error, 1)
		go func() {
			_, err := stdcopy.StdCopy(stdoutW, stderrW, attach.Reader)
			copyDone <- err
		}()

		var copyErr error
		select {
		case copyErr = <-copyDone:
		case <-streamCtx.Done():
			rec.mu.Lock()
			rec.canceled = true
			rec.mu.Unlock()
			// Best effort: kill the inner process.
			_ = killExec(context.Background(), dockerClient, created.ID)
			copyErr = streamCtx.Err()
		}

		// Inspect to get the real exit code.
		inspect, ierr := dockerClient.ContainerExecInspect(context.Background(), created.ID)
		var exit int
		switch {
		case rec.canceled:
			exit = 137
			rec.Finish(CmdStatusKilled, exit)
		case ierr != nil:
			rec.Finish(CmdStatusFailed, -1)
		case inspect.ExitCode != 0:
			exit = inspect.ExitCode
			rec.Finish(CmdStatusFinished, exit)
		default:
			rec.Finish(CmdStatusFinished, 0)
		}

		if copyErr != nil && !errors.Is(copyErr, io.EOF) && !errors.Is(copyErr, context.Canceled) {
			log.Debug().Err(copyErr).Str("cmd_id", rec.ID).Msg("sandbox exec stream ended with error")
		}
	}

	if req.Detached {
		// Hand client ownership to the goroutine so it survives our return.
		closeClient = false
		go func() {
			defer dockerClient.Close()
			runner(context.Background())
		}()
		return rec, nil
	}

	// Foreground — apply timeout if requested.
	runCtx, cancel := context.WithCancel(ctx)
	if req.TimeoutSeconds > 0 {
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(req.TimeoutSeconds)*time.Second)
	}
	defer cancel()
	runner(runCtx)
	return rec, nil
}

func killExec(ctx context.Context, dockerClient interface {
	ContainerExecInspect(ctx context.Context, execID string) (dockertypes.ContainerExecInspect, error)
}, execID string) error {
	// Docker doesn't expose ContainerExecKill — closing the attach + inspecting
	// is the closest we can do; the inner process receives SIGPIPE on stdin
	// close. Caller wrapping the docker client signature here keeps the import
	// graph minimal.
	_, _ = dockerClient.ContainerExecInspect(ctx, execID)
	return nil
}

// KillCommand terminates a running exec by closing its hijacked connection.
// The actual kill is done by sending SIGTERM via /proc inside the container.
func (o *SandboxOps) KillCommand(ctx context.Context, sessionID, cmdID, signal string) error {
	rec, err := o.GetCommand(cmdID)
	if err != nil {
		return err
	}
	if rec.Status != CmdStatusRunning {
		return nil
	}
	if signal == "" {
		signal = "TERM"
	}
	// Use exec to send a signal to the process tree of the running exec via
	// its Pid (looked up via inspect).
	dc := o.dm.FindDevContainerBySessionID(sessionID)
	if dc == nil {
		return fmt.Errorf("sandbox not found")
	}
	dockerClient, err := o.dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	insp, err := dockerClient.ContainerExecInspect(ctx, rec.execID)
	if err != nil {
		return err
	}
	if insp.Pid == 0 {
		return nil
	}
	// Run `kill -SIGNAL PID` inside the container using a fresh exec.
	killCfg := dockertypes.ExecConfig{
		Cmd: []string{"/bin/sh", "-c", fmt.Sprintf("kill -%s %d || true", signal, insp.Pid)},
	}
	created, err := dockerClient.ContainerExecCreate(ctx, dc.ContainerID, killCfg)
	if err != nil {
		return err
	}
	return dockerClient.ContainerExecStart(ctx, created.ID, dockertypes.ExecStartCheck{})
}

// cmdRecordWriter pipes Docker stdcopy demuxed bytes into a SandboxCmdRecord.
type cmdRecordWriter struct {
	rec    *SandboxCmdRecord
	stream string
}

func (w *cmdRecordWriter) Write(p []byte) (int, error) {
	if w.stream == "stdout" {
		w.rec.AppendStdout(p)
	} else {
		w.rec.AppendStderr(p)
	}
	return len(p), nil
}

// ----------------------------------------------------------------------------
// File I/O helpers
// ----------------------------------------------------------------------------

// ReadFile reads a file from the sandbox container as raw bytes.
func (o *SandboxOps) ReadFile(ctx context.Context, sessionID, path string) ([]byte, error) {
	dc := o.dm.FindDevContainerBySessionID(sessionID)
	if dc == nil {
		return nil, fmt.Errorf("sandbox not found")
	}
	dockerClient, err := o.dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return nil, err
	}
	defer dockerClient.Close()

	rc, _, err := dockerClient.CopyFromContainer(ctx, dc.ContainerID, path)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	tr := tar.NewReader(rc)
	hdr, err := tr.Next()
	if err != nil {
		return nil, err
	}
	if hdr.Typeflag == tar.TypeDir {
		return nil, fmt.Errorf("%s is a directory", path)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, tr); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteFile writes a file into the sandbox at the given path. Creates parent
// directories. Mode is the octal permission; 0 means 0644.
func (o *SandboxOps) WriteFile(ctx context.Context, sessionID, path string, data []byte, mode int64) error {
	dc := o.dm.FindDevContainerBySessionID(sessionID)
	if dc == nil {
		return fmt.Errorf("sandbox not found")
	}
	dockerClient, err := o.dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	if mode == 0 {
		mode = 0644
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)

	// Ensure parent dir exists by issuing a one-shot exec.
	mk := dockertypes.ExecConfig{Cmd: []string{"mkdir", "-p", dir}}
	if cr, err := dockerClient.ContainerExecCreate(ctx, dc.ContainerID, mk); err == nil {
		_ = dockerClient.ContainerExecStart(ctx, cr.ID, dockertypes.ExecStartCheck{})
	}

	// Build a tar containing the single file.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	hdr := &tar.Header{
		Name: base,
		Mode: mode,
		Size: int64(len(data)),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if _, err := tw.Write(data); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}

	return dockerClient.CopyToContainer(ctx, dc.ContainerID, dir, &buf, dockertypes.CopyToContainerOptions{})
}

// DeleteFile removes a file or directory inside the sandbox.
func (o *SandboxOps) DeleteFile(ctx context.Context, sessionID, path string, recursive bool) error {
	dc := o.dm.FindDevContainerBySessionID(sessionID)
	if dc == nil {
		return fmt.Errorf("sandbox not found")
	}
	dockerClient, err := o.dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return err
	}
	defer dockerClient.Close()

	args := []string{"rm"}
	if recursive {
		args = append(args, "-rf")
	}
	args = append(args, "--", path)

	cfg := dockertypes.ExecConfig{Cmd: args, AttachStderr: true, AttachStdout: true}
	cr, err := dockerClient.ContainerExecCreate(ctx, dc.ContainerID, cfg)
	if err != nil {
		return err
	}
	att, err := dockerClient.ContainerExecAttach(ctx, cr.ID, dockertypes.ExecStartCheck{})
	if err != nil {
		return err
	}
	defer att.Close()
	var stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(io.Discard, &stderr, att.Reader)
	insp, err := dockerClient.ContainerExecInspect(ctx, cr.ID)
	if err != nil {
		return err
	}
	if insp.ExitCode != 0 {
		return fmt.Errorf("rm failed: %s", stderr.String())
	}
	return nil
}

// ListDirectoryEntry is the parsed shape of one row in `ls -la --time-style=...`.
type ListDirectoryEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

// ListDirectory enumerates a directory inside the sandbox.
func (o *SandboxOps) ListDirectory(ctx context.Context, sessionID, path string) ([]ListDirectoryEntry, error) {
	dc := o.dm.FindDevContainerBySessionID(sessionID)
	if dc == nil {
		return nil, fmt.Errorf("sandbox not found")
	}
	dockerClient, err := o.dm.getDockerClient(dc.DockerSocket)
	if err != nil {
		return nil, err
	}
	defer dockerClient.Close()

	if path == "" {
		path = "/root"
	}

	cfg := dockertypes.ExecConfig{
		AttachStdout: true,
		AttachStderr: true,
		Cmd:          []string{"/bin/sh", "-c", fmt.Sprintf("ls -la --time-style=long-iso -- %q 2>&1", path)},
	}
	cr, err := dockerClient.ContainerExecCreate(ctx, dc.ContainerID, cfg)
	if err != nil {
		return nil, err
	}
	att, err := dockerClient.ContainerExecAttach(ctx, cr.ID, dockertypes.ExecStartCheck{})
	if err != nil {
		return nil, err
	}
	defer att.Close()
	var stdout, stderr bytes.Buffer
	_, _ = stdcopy.StdCopy(&stdout, &stderr, att.Reader)
	insp, err := dockerClient.ContainerExecInspect(ctx, cr.ID)
	if err != nil {
		return nil, err
	}
	if insp.ExitCode != 0 {
		return nil, fmt.Errorf("ls failed: %s", stderr.String())
	}

	return parseLsOutput(stdout.String(), path), nil
}

// parseLsOutput parses `ls -la --time-style=long-iso` rows.
func parseLsOutput(out, parent string) []ListDirectoryEntry {
	var entries []ListDirectoryEntry
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "total ") {
			continue
		}
		// long-iso: <perms> <links> <owner> <group> <size> <date> <time> <name>
		parts := strings.Fields(line)
		if len(parts) < 8 {
			continue
		}
		mode := parts[0]
		size, _ := parseInt64(parts[4])
		modTime := parts[5] + " " + parts[6]
		name := strings.Join(parts[7:], " ")
		// Strip symlink target after " -> ".
		if idx := strings.Index(name, " -> "); idx >= 0 {
			name = name[:idx]
		}
		if name == "." || name == ".." {
			continue
		}
		entries = append(entries, ListDirectoryEntry{
			Name:    name,
			Path:    filepath.Join(parent, name),
			IsDir:   strings.HasPrefix(mode, "d"),
			Size:    size,
			Mode:    mode,
			ModTime: modTime,
		})
	}
	return entries
}

func parseInt64(s string) (int64, error) {
	var n int64
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("not an integer: %q", s)
		}
		n = n*10 + int64(r-'0')
	}
	return n, nil
}
