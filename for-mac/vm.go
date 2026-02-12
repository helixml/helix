package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// splitLines splits a string into non-empty lines
func splitLines(s string) []string {
	return strings.Split(strings.TrimSpace(s), "\n")
}

// ErrVMImagesNotDownloaded is returned when VM images need to be downloaded from the CDN
var ErrVMImagesNotDownloaded = fmt.Errorf("VM images not downloaded")

// VMState represents the current state of the VM
type VMState string

const (
	VMStateStopped  VMState = "stopped"
	VMStateStarting VMState = "starting"
	VMStateRunning  VMState = "running"
	VMStateStopping VMState = "stopping"
	VMStateError    VMState = "error"
)

// VMConfig holds VM configuration
type VMConfig struct {
	Name            string `json:"name"`
	CPUs            int    `json:"cpus"`
	MemoryMB        int    `json:"memory_mb"`
	DiskPath        string `json:"disk_path"`
	VsockCID        uint32 `json:"vsock_cid"`          // virtio-vsock Context ID for host<->guest communication
	SSHPort         int    `json:"ssh_port"`            // Host port forwarded to guest SSH
	APIPort         int    `json:"api_port"`            // Host port forwarded to Helix API
	QMPPort         int    `json:"qmp_port"`            // QEMU Machine Protocol for control
	FrameExportPort int    `json:"frame_export_port"`   // TCP port for Helix frame export (0 = disabled)
	ExposeOnNetwork bool   `json:"expose_on_network"`   // Bind to 0.0.0.0 instead of localhost
}

// VMStatus represents current VM status
type VMStatus struct {
	State      VMState `json:"state"`
	BootStage  string  `json:"boot_stage,omitempty"` // Current boot stage (shown in UI during startup)
	CPUPercent float64 `json:"cpu_percent"`
	MemoryUsed int64   `json:"memory_used"`
	Uptime     int64   `json:"uptime"`
	Sessions   int     `json:"sessions"`
	ErrorMsg   string  `json:"error_msg,omitempty"`
	APIReady   bool    `json:"api_ready"`
}

// VMManager manages the Helix VM (QEMU on macOS, WSL2 on Windows)
type VMManager struct {
	ctx        context.Context
	appCtx     context.Context
	config     VMConfig
	status     VMStatus
	statusMu   sync.RWMutex
	cmd        *exec.Cmd
	cancelFunc context.CancelFunc
	startTime  time.Time
	// Serial console ring buffer
	consoleBuf   []byte
	consoleMu    sync.Mutex
	consoleStdin io.WriteCloser
	// Callback for state changes (used by system tray)
	onStateChange func(state string)
	// Desktop auto-login secret (set from AppSettings before VM start)
	desktopSecret string
	// VM console login password (set from AppSettings before VM start)
	consolePassword string
}

// NewVMManager creates a new VM manager
func NewVMManager() *VMManager {
	return &VMManager{
		config: VMConfig{
			Name:            "helix-vm",
			CPUs:            4,
			MemoryMB:        8192,
			VsockCID:        3,
			SSHPort:         41222,
			APIPort:         41080,
			QMPPort:         41444,
			FrameExportPort: 41937,
		},
		status: VMStatus{
			State: VMStateStopped,
		},
	}
}

// SetAppContext sets the Wails app context
func (vm *VMManager) SetAppContext(ctx context.Context) {
	vm.appCtx = ctx
}

// GetStatus returns current VM status
func (vm *VMManager) GetStatus() VMStatus {
	vm.statusMu.RLock()
	defer vm.statusMu.RUnlock()

	if vm.status.State == VMStateRunning && !vm.startTime.IsZero() {
		vm.status.Uptime = int64(time.Since(vm.startTime).Seconds())
	}

	return vm.status
}

// GetConfig returns current VM config
func (vm *VMManager) GetConfig() VMConfig {
	return vm.config
}

// SetConfig updates VM config
func (vm *VMManager) SetConfig(config VMConfig) error {
	vm.statusMu.Lock()
	defer vm.statusMu.Unlock()

	if vm.status.State != VMStateStopped {
		return fmt.Errorf("cannot change config while VM is running")
	}

	vm.config = config
	return nil
}

// setError sets error state
func (vm *VMManager) setError(err error) {
	vm.statusMu.Lock()
	vm.status.State = VMStateError
	vm.status.ErrorMsg = err.Error()
	vm.statusMu.Unlock()
	vm.emitStatus()
}

// emitStatus emits status update to frontend and notifies tray
func (vm *VMManager) emitStatus() {
	status := vm.GetStatus()
	if vm.appCtx != nil {
		runtime.EventsEmit(vm.appCtx, "vm:status", status)
	}
	if vm.onStateChange != nil {
		vm.onStateChange(string(status.State))
	}
}

const maxConsoleSize = 256 * 1024 // 256KB ring buffer

// appendConsole appends data to the serial console ring buffer and emits to frontend
func (vm *VMManager) appendConsole(data []byte) {
	vm.consoleMu.Lock()
	vm.consoleBuf = append(vm.consoleBuf, data...)
	if len(vm.consoleBuf) > maxConsoleSize {
		vm.consoleBuf = vm.consoleBuf[len(vm.consoleBuf)-maxConsoleSize:]
	}
	vm.consoleMu.Unlock()
	if vm.appCtx != nil {
		runtime.EventsEmit(vm.appCtx, "console:output", string(data))
	}
}

// GetConsoleOutput returns the full console buffer
func (vm *VMManager) GetConsoleOutput() string {
	vm.consoleMu.Lock()
	defer vm.consoleMu.Unlock()
	return string(vm.consoleBuf)
}

// checkAPIHealth verifies the Helix API is actually responding to HTTP requests.
func (vm *VMManager) checkAPIHealth() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/v1/status", vm.config.APIPort))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// diagnoseAPIFailure checks docker compose inside the VM to determine why the API isn't starting.
// Uses runInVM which is implemented per-platform (SSH on macOS, wsl.exe on Windows).
func (vm *VMManager) diagnoseAPIFailure() string {
	cmd := vm.runInVM(`cd ~/helix 2>/dev/null && docker compose ps --format '{{.Service}}: {{.Status}}' 2>/dev/null | head -20`)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("could not check container status: %v", err)
	}
	status := strings.TrimSpace(string(out))
	if status == "" {
		return "no containers running — docker compose may have failed to start"
	}
	log.Printf("Container status:\n%s", status)

	logCmd := vm.runInVM(`cd ~/helix 2>/dev/null && docker compose logs api --tail 10 2>/dev/null`)
	logOut, _ := logCmd.CombinedOutput()
	if len(logOut) > 0 {
		log.Printf("API container logs:\n%s", string(logOut))
	}

	return fmt.Sprintf("containers: %s", status)
}

// ensureDockerRunning starts Docker if it isn't already running inside the VM/WSL.
func (vm *VMManager) ensureDockerRunning() error {
	cmd := vm.runInVM("if ! systemctl is-active docker >/dev/null 2>&1; then sudo systemctl start docker && echo STARTED; else echo RUNNING; fi")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Docker: %w (output: %s)", err, string(out))
	}
	log.Printf("Docker: %s", strings.TrimSpace(string(out)))
	return nil
}

// startHelixStack starts the Helix docker-compose services inside the VM/WSL.
func (vm *VMManager) startHelixStack() error {
	script := `
cd ~/helix 2>/dev/null || exit 0
if [ ! -f .env.vm ]; then
    echo 'NO_ENV_FILE'
    exit 0
fi
# Docker Compose always reads .env by default; symlink so it picks up our config
if [ ! -e .env ]; then
    ln -s .env.vm .env
fi
# Check if stack is already running
if docker compose ps --format '{{.Service}}' 2>/dev/null | grep -q api; then
    echo 'ALREADY_RUNNING'
else
    echo 'Starting Helix stack...'
    docker compose -f docker-compose.dev.yaml up -d 2>&1
    echo 'STARTED'
fi
`
	cmd := vm.runInVM(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Helix stack: %w (output: %s)", err, string(out))
	}
	log.Printf("Helix stack: %s", strings.TrimSpace(string(out)))
	return nil
}

// injectDesktopSecret ensures the desktop auto-login secret, admin config,
// and console password are applied to the VM/WSL environment.
func (vm *VMManager) injectDesktopSecret() error {
	script := fmt.Sprintf(`
ENV_FILE=/home/ubuntu/helix/.env.vm
if [ ! -f "$ENV_FILE" ]; then
    exit 0
fi

CHANGED=0

# Desktop auto-login secret
if grep -q '^DESKTOP_AUTO_LOGIN_SECRET=' "$ENV_FILE" 2>/dev/null; then
    CURRENT=$(grep '^DESKTOP_AUTO_LOGIN_SECRET=' "$ENV_FILE" | cut -d= -f2-)
    if [ "$CURRENT" != "%s" ]; then
        sed -i "s|^DESKTOP_AUTO_LOGIN_SECRET=.*|DESKTOP_AUTO_LOGIN_SECRET=%s|" "$ENV_FILE"
        CHANGED=1
    fi
else
    echo 'DESKTOP_AUTO_LOGIN_SECRET=%s' >> "$ENV_FILE"
    CHANGED=1
fi

# Ensure all desktop users are admin
if ! grep -q '^ADMIN_USER_IDS=' "$ENV_FILE" 2>/dev/null; then
    echo 'ADMIN_USER_IDS=all' >> "$ENV_FILE"
    CHANGED=1
fi

# Persist env to ZFS (if available) or local config
if [ $CHANGED -eq 1 ]; then
    if [ -d /helix/config ]; then
        sudo cp "$ENV_FILE" /helix/config/env.vm
    fi
    echo 'ENV_UPDATED'
fi

# Set ubuntu user password and persist
PASS_FILE=/helix/config/console_password
if [ -d /helix/config ]; then
    CURRENT_PASS=""
    if [ -f "$PASS_FILE" ]; then
        CURRENT_PASS=$(sudo cat "$PASS_FILE" 2>/dev/null)
    fi
    if [ "$CURRENT_PASS" != "%s" ]; then
        echo 'ubuntu:%s' | sudo chpasswd
        echo '%s' | sudo tee "$PASS_FILE" > /dev/null
        sudo chmod 600 "$PASS_FILE"
        echo 'PASS_UPDATED'
    fi
else
    # No persistent config dir — just set the password
    echo 'ubuntu:%s' | sudo chpasswd 2>/dev/null || true
fi
`, vm.desktopSecret, vm.desktopSecret, vm.desktopSecret,
		vm.consolePassword, vm.consolePassword, vm.consolePassword,
		vm.consolePassword)

	cmd := vm.runInVM(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inject desktop secret failed: %w (output: %s)", err, string(out))
	}
	outStr := string(out)
	if strings.Contains(outStr, "ENV_UPDATED") {
		log.Printf("Desktop secret injected into .env.vm — restarting API container")
		restart := vm.runInVM("cd ~/helix && docker compose --env-file .env.vm restart api 2>&1 || true")
		restartOut, _ := restart.CombinedOutput()
		log.Printf("API restart: %s", string(restartOut))
	}
	if strings.Contains(outStr, "PASS_UPDATED") {
		log.Printf("Console password updated for ubuntu user")
	}
	return nil
}
