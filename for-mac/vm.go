package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

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

// getHelixDataDir returns the macOS-conventional data directory for Helix.
// Uses ~/Library/Application Support/Helix/ which works with and without App Sandbox.
func getHelixDataDir() string {
	homeDir, _ := os.UserHomeDir()
	return filepath.Join(homeDir, "Library", "Application Support", "Helix")
}

// getVMDir returns the writable VM directory
func (vm *VMManager) getVMDir() string {
	return filepath.Join(getHelixDataDir(), "vm", "helix-desktop")
}

// getVMImagePath returns the path to the root disk image
func (vm *VMManager) getVMImagePath() string {
	if vm.config.DiskPath != "" {
		return vm.config.DiskPath
	}
	return filepath.Join(vm.getVMDir(), "disk.qcow2")
}

// getZFSDiskPath returns the path to the ZFS data disk
func (vm *VMManager) getZFSDiskPath() string {
	return filepath.Join(vm.getVMDir(), "zfs-data.qcow2")
}

// ensureVMExtracted checks if VM disk images exist in the writable location.
// VM images are downloaded from the CDN on first launch rather than bundled in the app.
// Returns ErrVMImagesNotDownloaded if images need to be downloaded, or nil if ready.
func (vm *VMManager) ensureVMExtracted() error {
	vmDir := vm.getVMDir()
	rootDisk := vm.getVMImagePath()

	// Root disk is required (downloaded from CDN)
	if _, err := os.Stat(rootDisk); err != nil {
		log.Printf("VM root disk not found at %s — download required", vmDir)

		// Copy EFI vars from bundle if available (they're small, ~64MB, and still bundled)
		bundlePath := vm.getAppBundlePath()
		if bundlePath != "" {
			bundledEFI := filepath.Join(bundlePath, "Contents", "Resources", "vm", "efi_vars.fd")
			efiVars := filepath.Join(vmDir, "efi_vars.fd")
			if _, err := os.Stat(efiVars); os.IsNotExist(err) {
				if _, err := os.Stat(bundledEFI); err == nil {
					os.MkdirAll(vmDir, 0755)
					log.Printf("Copying EFI vars from bundle...")
					if err := copyFile(bundledEFI, efiVars); err != nil {
						log.Printf("Warning: failed to copy EFI vars: %v", err)
					}
				}
			}
		}

		return ErrVMImagesNotDownloaded
	}

	// ZFS data disk is created locally on first boot (no need to download)
	zfsDisk := vm.getZFSDiskPath()
	if _, err := os.Stat(zfsDisk); os.IsNotExist(err) {
		log.Printf("Creating ZFS data disk at %s (128 GB thin-provisioned)...", zfsDisk)
		os.MkdirAll(vmDir, 0755)
		if err := vm.createEmptyQcow2(zfsDisk, "128G"); err != nil {
			return fmt.Errorf("failed to create ZFS data disk: %w", err)
		}
		log.Printf("ZFS data disk created")
	}

	return nil
}

// createEmptyQcow2 creates an empty thin-provisioned qcow2 image using qemu-img.
func (vm *VMManager) createEmptyQcow2(path, size string) error {
	qemuImg := vm.findQEMUImg()
	if qemuImg == "" {
		return fmt.Errorf("qemu-img not found — cannot create disk image")
	}
	cmd := exec.Command(qemuImg, "create", "-f", "qcow2", path, size)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findQEMUImg locates the qemu-img binary. Search order:
//  1. Same directory as the QEMU binary (app bundle or PATH)
//  2. Homebrew: /opt/homebrew/bin/qemu-img
//  3. System PATH
func (vm *VMManager) findQEMUImg() string {
	// Check next to the QEMU binary
	qemuPath := vm.findQEMUBinary()
	if qemuPath != "" {
		qemuImg := filepath.Join(filepath.Dir(qemuPath), "qemu-img")
		if _, err := os.Stat(qemuImg); err == nil {
			return qemuImg
		}
	}

	// Homebrew
	if _, err := os.Stat("/opt/homebrew/bin/qemu-img"); err == nil {
		return "/opt/homebrew/bin/qemu-img"
	}

	// System PATH
	path, err := exec.LookPath("qemu-img")
	if err == nil {
		return path
	}

	return ""
}

// copyFile copies a file from src to dst using streaming (no full file in memory)
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		os.Remove(dst) // clean up partial file
		return err
	}
	return dstFile.Close()
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

	// Ensure VM images are extracted from bundle (first launch copies from app bundle)
	if err := vm.ensureVMExtracted(); err != nil {
		vm.setError(err)
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	vm.ctx = ctx
	vm.cancelFunc = cancel

	go vm.runVM(ctx)

	return nil
}

// runVM runs the QEMU process with virtio-gpu and vsock
func (vm *VMManager) runVM(ctx context.Context) {
	vmDir := vm.getVMDir()
	imagePath := vm.getVMImagePath()
	zfsDiskPath := vm.getZFSDiskPath()

	// Find EFI firmware (bundled in app or Homebrew)
	efiCode := vm.findFirmware("edk2-aarch64-code.fd")
	if efiCode == "" {
		vm.setError(fmt.Errorf("EFI firmware not found. Install QEMU via 'brew install qemu' or use the bundled app"))
		return
	}

	// Use VM-specific EFI vars (extracted from bundle or from provisioning)
	efiVars := filepath.Join(vmDir, "efi_vars.fd")
	if _, err := os.Stat(efiVars); os.IsNotExist(err) {
		// Fall back to template if no VM-specific vars exist
		efiVarsTemplate := vm.findFirmware("edk2-arm-vars.fd")
		if efiVarsTemplate != "" {
			if data, readErr := os.ReadFile(efiVarsTemplate); readErr == nil {
				os.MkdirAll(vmDir, 0755)
				os.WriteFile(efiVars, data, 0644)
			}
		}
		// If template copy didn't work, create an empty 64MB file
		if _, checkErr := os.Stat(efiVars); os.IsNotExist(checkErr) {
			if f, createErr := os.Create(efiVars); createErr == nil {
				f.Truncate(64 * 1024 * 1024) // 64MB
				f.Close()
			}
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

		// Storage: root disk (vda) and ZFS data disk (vdb)
		"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio,cache=writeback", imagePath),
	}

	// Add ZFS data disk if it exists
	if _, err := os.Stat(zfsDiskPath); err == nil {
		args = append(args,
			"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio", zfsDiskPath),
		)
	}

	args = append(args,
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
	)

	// Find QEMU binary: bundled in app > system PATH
	qemuPath := vm.findQEMUBinary()
	if qemuPath == "" {
		vm.setError(fmt.Errorf("QEMU not found. Install via 'brew install qemu' or use the bundled app"))
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
	err := vm.cmd.Wait()

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

	sshReady := false
	zfsInitialized := false
	videoReady := false
	apiReady := false

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Wait for SSH first (needed for ZFS init)
			if !sshReady {
				conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vm.config.SSHPort), time.Second)
				if err == nil {
					conn.Close()
					sshReady = true
					log.Printf("VM SSH is ready")
				}
			}

			// Initialize ZFS pool via SSH if SSH is up but ZFS not yet done
			if sshReady && !zfsInitialized {
				if err := vm.initZFSPool(); err != nil {
					log.Printf("ZFS init not ready yet: %v", err)
				} else {
					zfsInitialized = true
				}
			}

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

// initZFSPool initializes the ZFS pool on the data disk via SSH.
// This is idempotent — if the pool already exists, it's a no-op.
func (vm *VMManager) initZFSPool() error {
	script := `
		if sudo zpool list helix 2>/dev/null; then
			echo 'ZFS pool helix already exists'
		else
			# Find the data disk (second virtio disk, typically /dev/vdb or /dev/vdc)
			DATA_DISK=""
			for disk in /dev/vdb /dev/vdc /dev/vdd; do
				if [ -b "$disk" ] && ! mount | grep -q "$disk"; then
					DATA_DISK="$disk"
					break
				fi
			done
			if [ -z "$DATA_DISK" ]; then
				echo 'ERROR: No unmounted data disk found'
				exit 1
			fi
			echo "Creating ZFS pool on $DATA_DISK..."
			sudo zpool create -f helix "$DATA_DISK"
		fi
		# Ensure datasets exist
		sudo zfs list helix/workspaces 2>/dev/null || sudo zfs create -o dedup=on -o compression=lz4 -o atime=off helix/workspaces
		sudo zfs list helix/docker 2>/dev/null || sudo zfs create -o compression=lz4 -o atime=off helix/docker
		echo 'ZFS ready'
	`
	cmd := exec.Command("ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=3",
		"-p", fmt.Sprintf("%d", vm.config.SSHPort),
		"helix@localhost",
		script,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SSH command failed: %w (output: %s)", err, string(out))
	}
	log.Printf("ZFS init: %s", string(out))
	return nil
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

// getAppBundlePath returns the path to the running .app bundle, if any.
// Returns empty string if not running from an app bundle.
func (vm *VMManager) getAppBundlePath() string {
	return getAppBundlePath()
}

// findQEMUBinary locates the QEMU binary. Search order:
//  1. Bundled in app: Contents/MacOS/qemu-system-aarch64 (wrapper executable)
//  2. System PATH: qemu-system-aarch64
//
// QEMU is built as a dylib + thin wrapper. The wrapper (75KB) has main() and
// loads libqemu-aarch64-softmmu.dylib via @executable_path. You cannot execute
// a .dylib directly — the wrapper executable is required.
func (vm *VMManager) findQEMUBinary() string {
	// Check app bundle first
	appPath := vm.getAppBundlePath()
	if appPath != "" {
		bundled := filepath.Join(appPath, "Contents", "MacOS", "qemu-system-aarch64")
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	// Fall back to system PATH
	path, err := exec.LookPath("qemu-system-aarch64")
	if err == nil {
		return path
	}

	return ""
}

// findFirmware locates an EFI firmware file. Search order:
//  1. Bundled in app: Contents/Resources/firmware/<name>
//  2. Homebrew: /opt/homebrew/share/qemu/<name>
func (vm *VMManager) findFirmware(name string) string {
	// Check app bundle first
	appPath := vm.getAppBundlePath()
	if appPath != "" {
		bundled := filepath.Join(appPath, "Contents", "Resources", "firmware", name)
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	// Homebrew path
	brewPath := filepath.Join("/opt/homebrew/share/qemu", name)
	if _, err := os.Stat(brewPath); err == nil {
		return brewPath
	}

	return ""
}

// findVulkanICD locates the KosmicKrisp Vulkan ICD JSON. Search order:
//  1. Bundled in app: Contents/Resources/vulkan/icd.d/kosmickrisp_mesa_icd.json
//  2. UTM.app: /Applications/UTM.app/Contents/Resources/vulkan/icd.d/kosmickrisp_mesa_icd.json
func (vm *VMManager) findVulkanICD() string {
	// Check app bundle first
	appPath := vm.getAppBundlePath()
	if appPath != "" {
		bundled := filepath.Join(appPath, "Contents", "Resources", "vulkan", "icd.d", "kosmickrisp_mesa_icd.json")
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	// Fall back to UTM.app
	utmPath := "/Applications/UTM.app/Contents/Resources/vulkan/icd.d/kosmickrisp_mesa_icd.json"
	if _, err := os.Stat(utmPath); err == nil {
		return utmPath
	}

	return ""
}

// buildQEMUEnv returns the environment variables for the QEMU process.
// Sets VK_DRIVER_FILES to use KosmicKrisp (Mesa Vulkan) instead of MoltenVK.
// KosmicKrisp produces dramatically better rendering quality under concurrent
// GNOME sessions with virglrenderer's Venus Vulkan path.
func (vm *VMManager) buildQEMUEnv() []string {
	env := os.Environ()

	// Use KosmicKrisp Vulkan driver — check bundled location first, then UTM.app
	icdPath := vm.findVulkanICD()
	if icdPath != "" {
		env = append(env, "VK_DRIVER_FILES="+icdPath)
	}

	return env
}
