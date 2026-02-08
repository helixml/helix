package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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
	Name        string `json:"name"`
	CPUs        int    `json:"cpus"`
	MemoryMB    int    `json:"memory_mb"`
	DiskPath    string `json:"disk_path"`
	VsockCID    uint32 `json:"vsock_cid"`    // virtio-vsock Context ID for host<->guest communication
	SSHPort     int    `json:"ssh_port"`     // Host port forwarded to guest SSH
	APIPort     int    `json:"api_port"`     // Host port forwarded to Helix API
	VideoPort   int    `json:"video_port"`   // Host port forwarded to video stream WebSocket
	QMPPort     int    `json:"qmp_port"`     // QEMU Machine Protocol for control
}

// VMStatus represents current VM status
type VMStatus struct {
	State      VMState `json:"state"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryUsed int64   `json:"memory_used"`
	Uptime     int64   `json:"uptime"`
	Sessions   int     `json:"sessions"`
	ErrorMsg   string  `json:"error_msg,omitempty"`
	APIReady   bool    `json:"api_ready"`
	VideoReady bool    `json:"video_ready"`
}

// VMManager manages the Helix VM
type VMManager struct {
	ctx        context.Context
	appCtx     context.Context
	config     VMConfig
	status     VMStatus
	statusMu   sync.RWMutex
	cmd        *exec.Cmd
	cancelFunc context.CancelFunc
	startTime  time.Time
}

// NewVMManager creates a new VM manager
func NewVMManager() *VMManager {
	return &VMManager{
		config: VMConfig{
			Name:      "helix-vm",
			CPUs:      4,
			MemoryMB:  8192,  // 8GB - enough for Docker + GNOME + Zed + containers
			VsockCID:  3,     // Guest CID (2 is host, 3+ are guests)
			SSHPort:   2222,  // Host:2222 -> Guest:22
			APIPort:   8080,  // Host:8080 -> Guest:8080 (Helix API)
			VideoPort: 8765,  // Host:8765 -> Guest:8765 (video stream WebSocket)
			QMPPort:   4444,  // QMP for VM control
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

// getVMImagePath returns the path to the VM disk image
func (vm *VMManager) getVMImagePath() string {
	if vm.config.DiskPath != "" {
		return vm.config.DiskPath
	}

	// Default location
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, ".helix", "vm", "helix-ubuntu.qcow2")
}

// Start starts the VM
func (vm *VMManager) Start() error {
	vm.statusMu.Lock()
	if vm.status.State != VMStateStopped {
		vm.statusMu.Unlock()
		return fmt.Errorf("VM is not stopped (current state: %s)", vm.status.State)
	}
	vm.status.State = VMStateStarting
	vm.status.ErrorMsg = ""
	vm.statusMu.Unlock()

	vm.emitStatus()

	// Check if VM image exists
	imagePath := vm.getVMImagePath()
	if _, err := os.Stat(imagePath); os.IsNotExist(err) {
		vm.setError(fmt.Errorf("VM image not found at %s. Please run setup first.", imagePath))
		return fmt.Errorf("VM image not found")
	}

	ctx, cancel := context.WithCancel(context.Background())
	vm.ctx = ctx
	vm.cancelFunc = cancel

	go vm.runVM(ctx)

	return nil
}

// runVM runs the QEMU process with virtio-gpu and vsock
func (vm *VMManager) runVM(ctx context.Context) {
	homeDir, _ := os.UserHomeDir()
	vmDir := filepath.Join(homeDir, ".helix", "vm")
	imagePath := vm.getVMImagePath()
	efiCode := "/opt/homebrew/share/qemu/edk2-aarch64-code.fd"
	efiVars := filepath.Join(vmDir, "efi-vars.fd")
	cloudInitISO := filepath.Join(vmDir, "cloud-init.iso")

	// Create EFI vars file if it doesn't exist
	if _, err := os.Stat(efiVars); os.IsNotExist(err) {
		f, err := os.Create(efiVars)
		if err == nil {
			f.Truncate(64 * 1024 * 1024) // 64MB
			f.Close()
		}
	}

	// Build QEMU command
	// Architecture:
	//   Linux VM with Docker → sandbox container → dev containers (helix-ubuntu)
	//   Dev containers capture frames via PipeWire → stream via WebSocket
	//   Frames forwarded to host via port forward → VideoToolbox encode on host
	//   virtio-gpu provides rendering acceleration for the VM
	args := []string{
		// Machine configuration with HVF acceleration
		"-machine", "virt,accel=hvf,highmem=on",
		"-cpu", "host",
		"-smp", fmt.Sprintf("%d", vm.config.CPUs),
		"-m", fmt.Sprintf("%d", vm.config.MemoryMB),

		// EFI firmware
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", efiCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", efiVars),

		// Storage
		"-device", "virtio-blk-pci,drive=hd0",
		"-drive", fmt.Sprintf("id=hd0,file=%s,format=qcow2,if=none,cache=writeback", imagePath),
		"-device", "virtio-blk-pci,drive=cd0",
		"-drive", fmt.Sprintf("id=cd0,file=%s,format=raw,if=none,readonly=on", cloudInitISO),

		// Network with port forwarding for SSH, API, and video stream
		"-device", "virtio-net-pci,netdev=net0",
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22,hostfwd=tcp::%d-:8080,hostfwd=tcp::%d-:8765",
			vm.config.SSHPort, vm.config.APIPort, vm.config.VideoPort),

		// virtio-vsock for high-speed host<->guest communication
		// Useful for frame transfer bypassing network stack
		"-device", fmt.Sprintf("vhost-vsock-pci,guest-cid=%d", vm.config.VsockCID),

		// GPU: virtio-gpu with virgl3d for OpenGL acceleration
		// This accelerates rendering inside the VM (GNOME, Zed, etc.)
		// EDID enabled with 5K preferred resolution so 5120x2880 is available as a DRM mode.
		// Containers requesting lower resolutions still get their exact mode (1080p, 4K, etc.)
		"-device", "virtio-gpu-gl-pci,id=gpu0,edid=on,xres=5120,yres=2880",

		// Serial console for debugging (no VNC needed)
		"-serial", "mon:stdio",

		// QMP for VM control (pause, resume, etc.)
		"-qmp", fmt.Sprintf("tcp:localhost:%d,server,nowait", vm.config.QMPPort),

		// No graphical display - headless VM
		"-display", "none",
	}

	// Check if QEMU is available
	qemuPath, err := exec.LookPath("qemu-system-aarch64")
	if err != nil {
		vm.setError(fmt.Errorf("QEMU not found. Please install via 'brew install qemu'"))
		return
	}

	vm.cmd = exec.CommandContext(ctx, qemuPath, args...)
	vm.cmd.Env = vm.buildQEMUEnv()
	vm.cmd.Stdout = os.Stdout
	vm.cmd.Stderr = os.Stderr
	vm.cmd.Stdin = os.Stdin // Allow serial console interaction

	if err := vm.cmd.Start(); err != nil {
		vm.setError(fmt.Errorf("failed to start VM: %w", err))
		return
	}

	vm.startTime = time.Now()
	vm.statusMu.Lock()
	vm.status.State = VMStateRunning
	vm.statusMu.Unlock()
	vm.emitStatus()

	// Wait for VM services to be ready
	go vm.waitForReady(ctx)

	// Wait for process to exit
	err = vm.cmd.Wait()

	vm.statusMu.Lock()
	if ctx.Err() != nil {
		// Normal shutdown
		vm.status.State = VMStateStopped
	} else if err != nil {
		vm.status.State = VMStateError
		vm.status.ErrorMsg = err.Error()
	} else {
		vm.status.State = VMStateStopped
	}
	vm.statusMu.Unlock()
	vm.emitStatus()
}

// waitForReady waits for the VM's services to be ready
func (vm *VMManager) waitForReady(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	videoReady := false
	apiReady := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check if video stream port is responding
			if !videoReady {
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vm.config.VideoPort), time.Second)
				if err == nil {
					conn.Close()
					vm.statusMu.Lock()
					vm.status.VideoReady = true
					vm.statusMu.Unlock()
					vm.emitStatus()
					videoReady = true
				}
			}

			// Check if API is responding
			if !apiReady {
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vm.config.APIPort), time.Second)
				if err == nil {
					conn.Close()
					vm.statusMu.Lock()
					vm.status.APIReady = true
					vm.statusMu.Unlock()
					vm.emitStatus()
					apiReady = true
				}
			}

			if videoReady && apiReady {
				return
			}
		}
	}
}

// Stop stops the VM gracefully
func (vm *VMManager) Stop() error {
	vm.statusMu.Lock()
	if vm.status.State != VMStateRunning {
		vm.statusMu.Unlock()
		return fmt.Errorf("VM is not running")
	}
	vm.status.State = VMStateStopping
	vm.statusMu.Unlock()
	vm.emitStatus()

	// Try graceful shutdown via QMP first
	vm.sendQMPCommand("system_powerdown")

	// Give it time to shut down gracefully
	time.Sleep(5 * time.Second)

	// Cancel context to signal shutdown
	if vm.cancelFunc != nil {
		vm.cancelFunc()
	}

	// Force kill if still running
	if vm.cmd != nil && vm.cmd.Process != nil {
		vm.cmd.Process.Kill()
	}

	return nil
}

// sendQMPCommand sends a command to QEMU via QMP
func (vm *VMManager) sendQMPCommand(command string) error {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vm.config.QMPPort), 2*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	// QMP handshake
	buf := make([]byte, 1024)
	conn.Read(buf) // Read greeting

	// Send capabilities negotiation
	conn.Write([]byte(`{"execute": "qmp_capabilities"}`))
	conn.Read(buf) // Read response

	// Send actual command
	conn.Write([]byte(fmt.Sprintf(`{"execute": "%s"}`, command)))
	conn.Read(buf) // Read response

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

// emitStatus emits status update to frontend
func (vm *VMManager) emitStatus() {
	if vm.appCtx != nil {
		runtime.EventsEmit(vm.appCtx, "vm:status", vm.GetStatus())
	}
}

// GetVideoPort returns the video stream port
func (vm *VMManager) GetVideoPort() int {
	return vm.config.VideoPort
}

// GetVsockCID returns the vsock CID for the VM
func (vm *VMManager) GetVsockCID() uint32 {
	return vm.config.VsockCID
}

// GetSSHCommand returns the SSH command to connect to the VM
func (vm *VMManager) GetSSHCommand() string {
	return fmt.Sprintf("ssh -p %d helix@localhost", vm.config.SSHPort)
}

// buildQEMUEnv returns the environment variables for the QEMU process.
// Sets VK_DRIVER_FILES to use KosmicKrisp (Mesa Vulkan) instead of MoltenVK.
// KosmicKrisp produces dramatically better rendering quality under concurrent
// GNOME sessions with virglrenderer's Venus Vulkan path.
func (vm *VMManager) buildQEMUEnv() []string {
	env := os.Environ()

	// Use KosmicKrisp Vulkan driver from UTM's bundle.
	// The ICD JSON uses relative paths to resolve the framework library.
	icdPath := "/Applications/UTM.app/Contents/Resources/vulkan/icd.d/kosmickrisp_mesa_icd.json"
	if _, err := os.Stat(icdPath); err == nil {
		env = append(env, "VK_DRIVER_FILES="+icdPath)
	}

	return env
}
