package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// cprPattern matches Cursor Position Report responses (e.g. \e[18;138R).
// xterm.js generates these in response to DSR queries (\e[6n) from the guest.
// They must be filtered out to prevent them echoing back as garbage text.
var cprPattern = regexp.MustCompile(`\x1b\[\d+;\d+R`)

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
	Name        string `json:"name"`
	CPUs        int    `json:"cpus"`
	MemoryMB    int    `json:"memory_mb"`
	DiskPath    string `json:"disk_path"`
	VsockCID    uint32 `json:"vsock_cid"`    // virtio-vsock Context ID for host<->guest communication
	SSHPort     int    `json:"ssh_port"`     // Host port forwarded to guest SSH
	APIPort     int    `json:"api_port"`     // Host port forwarded to Helix API
	QMPPort         int  `json:"qmp_port"`          // QEMU Machine Protocol for control
	FrameExportPort int  `json:"frame_export_port"` // TCP port for Helix frame export (0 = disabled)
	ExposeOnNetwork bool `json:"expose_on_network"` // Bind to 0.0.0.0 instead of localhost
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
	// Serial console ring buffer
	consoleBuf   []byte
	consoleMu    sync.Mutex
	consoleStdin io.WriteCloser
	// Callback for state changes (used by system tray)
	onStateChange func(state string)
	// Callback when API becomes ready (used by auth proxy)
	onAPIReady func()
	// Desktop auto-login secret (set from AppSettings before VM start)
	desktopSecret string
	// VM console login password (set from AppSettings before VM start)
	consolePassword string
	// License key to inject into the VM (set from AppSettings before VM start)
	licenseKey string
	// Auth settings injected into the VM's .env.vm
	newUsersAreAdmin bool
	allowRegistration bool
	// stackStarted tracks whether the full docker compose stack has been started.
	// Used to gate API restarts in injectDesktopSecret — during boot, we don't
	// want to restart just the API before the full stack is up.
	stackStarted bool
	// envUpdated is set by injectDesktopSecret when .env.vm was modified during
	// boot (before stack start). startHelixStack checks this: if containers are
	// already running (Docker auto-restart from a non-clean shutdown), it restarts
	// them to pick up the new env values.
	envUpdated bool
	// composeFile is the detected docker compose file name inside the VM.
	// Set by startHelixStack(): "docker-compose.dev.yaml" (dev/build-from-source)
	// or "docker-compose.yaml" (prod/install.sh with pre-built images).
	composeFile string
}

// getSpiceSocketPath returns the path for the SPICE Unix socket
func (vm *VMManager) getSpiceSocketPath() string {
	return filepath.Join(os.TempDir(), "helix-spice.sock")
}

// bindAddr returns the address to bind forwarded ports to.
// Returns "0.0.0.0" if network exposure is enabled, empty string (localhost) otherwise.
func (vm *VMManager) bindAddr() string {
	if vm.config.ExposeOnNetwork {
		return "0.0.0.0"
	}
	return ""
}

// adminUserIDs returns the ADMIN_USER_IDS env var value.
func (vm *VMManager) adminUserIDs() string {
	if vm.newUsersAreAdmin {
		return "all"
	}
	return ""
}

// registrationEnabled returns the AUTH_REGISTRATION_ENABLED env var value.
func (vm *VMManager) registrationEnabled() string {
	if vm.allowRegistration {
		return "true"
	}
	return "false"
}

// NewVMManager creates a new VM manager
func NewVMManager() *VMManager {
	return &VMManager{
		config: VMConfig{
			Name:      "helix-vm",
			CPUs:      4,
			MemoryMB:  8192,  // 8GB - enough for Docker + GNOME + Zed + containers
			VsockCID:  3,     // Guest CID (2 is host, 3+ are guests)
			SSHPort:   41222,  // Host:41222 -> Guest:22
			APIPort: 41080, // Host:41080 -> Guest:8080 (Helix API)
			QMPPort:         41444, // QMP for VM control
			FrameExportPort: 41937, // TCP port for Helix frame export
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

	// Dev mode: create overlay backed by golden image
	if goldenPath := os.Getenv("HELIX_DEV_IMAGE"); goldenPath != "" {
		if err := vm.ensureDevImage(goldenPath, vmDir); err != nil {
			return err
		}
		// Overlay created at rootDisk path — fall through to ZFS disk creation
	}

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
		log.Printf("Creating ZFS data disk at %s (256 GB thin-provisioned)...", zfsDisk)
		os.MkdirAll(vmDir, 0755)
		if err := vm.createEmptyQcow2(zfsDisk, "256G"); err != nil {
			return fmt.Errorf("failed to create ZFS data disk: %w", err)
		}
		log.Printf("ZFS data disk created")
	}

	return nil
}

// ensureDevImage copies the golden image into the working directory.
// Uses a simple full copy instead of qcow2 overlays — more disk usage but
// avoids overlay invalidation headaches when the golden image gets updated.
// Factory reset = delete the copy, next start re-copies from golden.
func (vm *VMManager) ensureDevImage(goldenPath, vmDir string) error {
	// Validate golden image exists
	if _, err := os.Stat(goldenPath); err != nil {
		return fmt.Errorf("HELIX_DEV_IMAGE not found: %s", goldenPath)
	}

	absGolden, err := filepath.Abs(goldenPath)
	if err != nil {
		return fmt.Errorf("failed to resolve HELIX_DEV_IMAGE path: %w", err)
	}

	os.MkdirAll(vmDir, 0755)
	diskCopy := filepath.Join(vmDir, "disk.qcow2")

	// Reuse existing copy (user runs factory reset to get a fresh one)
	if _, err := os.Stat(diskCopy); err == nil {
		log.Printf("Dev mode: reusing existing disk copy at %s", diskCopy)
		return nil
	}

	// Copy golden image to working directory
	log.Printf("Dev mode: copying golden image %s → %s", absGolden, diskCopy)
	if err := copyFile(absGolden, diskCopy); err != nil {
		os.Remove(diskCopy) // clean up partial copy
		return fmt.Errorf("failed to copy golden image: %w", err)
	}
	log.Printf("Dev mode: golden image copied successfully")

	// Copy EFI vars from golden image's directory, fall back to app bundle
	goldenDir := filepath.Dir(absGolden)
	efiSrc := filepath.Join(goldenDir, "efi_vars.fd")
	efiDst := filepath.Join(vmDir, "efi_vars.fd")
	if _, err := os.Stat(efiDst); os.IsNotExist(err) {
		if _, err := os.Stat(efiSrc); err == nil {
			log.Printf("Dev mode: copying EFI vars from %s", efiSrc)
			if err := copyFile(efiSrc, efiDst); err != nil {
				log.Printf("Warning: failed to copy EFI vars: %v", err)
			}
		} else {
			// Fall back to app bundle EFI vars
			bundlePath := vm.getAppBundlePath()
			if bundlePath != "" {
				bundledEFI := filepath.Join(bundlePath, "Contents", "Resources", "vm", "efi_vars.fd")
				if _, err := os.Stat(bundledEFI); err == nil {
					log.Printf("Dev mode: copying EFI vars from app bundle")
					if err := copyFile(bundledEFI, efiDst); err != nil {
						log.Printf("Warning: failed to copy EFI vars from bundle: %v", err)
					}
				}
			}
		}
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

// resizeQcow2 resizes an existing qcow2 image. VM must be stopped.
func (vm *VMManager) resizeQcow2(path, size string) error {
	qemuImg := vm.findQEMUImg()
	if qemuImg == "" {
		return fmt.Errorf("qemu-img not found — cannot resize disk image")
	}
	cmd := exec.Command(qemuImg, "resize", path, size)
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
	if vm.status.State != VMStateStopped && vm.status.State != VMStateError {
		vm.statusMu.Unlock()
		return fmt.Errorf("VM is not stopped (current state: %s)", vm.status.State)
	}
	vm.status.State = VMStateStarting
	vm.status.ErrorMsg = ""
	vm.status.APIReady = false
	vm.status.BootStage = ""
	vm.statusMu.Unlock()

	vm.emitStatus()

	// Kill any orphaned QEMU process from a previous crash
	vm.killStaleQEMU()

	// Clean up stale SPICE socket
	spiceSock := vm.getSpiceSocketPath()
	if _, err := os.Stat(spiceSock); err == nil {
		os.Remove(spiceSock)
	}

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

// killStaleQEMU checks for an orphaned QEMU process holding the QMP port
// and kills it. This handles the case where the app was force-quit but QEMU
// kept running.
func (vm *VMManager) killStaleQEMU() {
	// Check if QMP port is in use
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vm.config.QMPPort), 500*time.Millisecond)
	if err != nil {
		// Port is free, no stale process
		return
	}
	conn.Close()
	log.Printf("QMP port %d is in use — looking for stale QEMU process", vm.config.QMPPort)

	// Use lsof to find the PID holding the port
	cmd := exec.Command("lsof", "-ti", fmt.Sprintf("tcp:%d", vm.config.QMPPort))
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		log.Printf("Could not find PID holding port %d", vm.config.QMPPort)
		return
	}

	// Parse PID(s) and kill them
	for _, line := range splitLines(string(out)) {
		if line == "" {
			continue
		}
		pid := 0
		if _, err := fmt.Sscanf(line, "%d", &pid); err != nil || pid <= 0 {
			continue
		}
		log.Printf("Killing stale QEMU process (PID %d) holding QMP port %d", pid, vm.config.QMPPort)
		proc, err := os.FindProcess(pid)
		if err != nil {
			continue
		}
		proc.Kill()
	}

	// Wait briefly for the port to be released
	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vm.config.QMPPort), 200*time.Millisecond)
		if err != nil {
			// Port is free
			log.Printf("QMP port %d is now free", vm.config.QMPPort)
			return
		}
		conn.Close()
	}
	log.Printf("Warning: QMP port %d still in use after killing stale process", vm.config.QMPPort)
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
		// Machine configuration — matches UTM's approach:
		// Separate -machine and -accel flags (rather than accel= in machine).
		// gic-version=3 for GICv3 interrupt controller (required for >8 CPUs).
		// ipa-granule-size=0x1000 sets stage-2 page table granule to 4KB,
		// matching the Linux guest's page size. Without this, macOS HVF uses
		// 16KB pages which can cause alignment issues with GPU memory mappings.
		"-machine", "virt,gic-version=3,highmem=on",
		"-accel", "hvf,ipa-granule-size=0x1000",
		"-cpu", "host",
		"-smp", fmt.Sprintf("%d", vm.config.CPUs),
		"-m", fmt.Sprintf("%d", vm.config.MemoryMB),
		// Disable QEMU default devices — we specify everything explicitly
		"-nodefaults",
		"-vga", "none",

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
		"-device", "virtio-net-pci,netdev=net0,romfile=",
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22,hostfwd=tcp:%s:%d-:8080",
			vm.config.SSHPort, vm.bindAddr(), vm.config.APIPort),

		// GPU: virtio-gpu-gl with virgl3d + Venus Vulkan passthrough
		// Matches UTM's config for full GPU acceleration inside the VM.
		// blob=true enables zero-copy memory sharing via host mappable blob resources.
		// venus=true enables Vulkan passthrough via Venus protocol.
		// hostmem=256M allocates host-side memory for GPU resources.
		// EDID enabled with 5K preferred resolution so 5120x2880 is available as a DRM mode.
		"-device", fmt.Sprintf("virtio-gpu-gl-pci,id=gpu0,hostmem=256M,blob=true,venus=true,edid=on,xres=5120,yres=2880,helix-port=%d", vm.config.FrameExportPort),

		// SPICE display with GL/ES context (via ANGLE) — matches UTM's approach.
		// Provides the EGL context needed by virglrenderer and the Helix frame export patches.
		"-spice", fmt.Sprintf("unix=on,addr=%s,disable-ticketing=on,gl=es", vm.getSpiceSocketPath()),

		// Serial console — captured and shown in the app UI
		"-serial", "mon:stdio",

		// QMP for VM control (pause, resume, etc.)
		"-qmp", fmt.Sprintf("tcp:localhost:%d,server,nowait", vm.config.QMPPort),
	)

	// Find QEMU binary: bundled in app > system PATH
	qemuPath := vm.findQEMUBinary()
	if qemuPath == "" {
		vm.setError(fmt.Errorf("QEMU not found. Install via 'brew install qemu' or use the bundled app"))
		return
	}

	vm.cmd = exec.CommandContext(ctx, qemuPath, args...)
	vm.cmd.Env = vm.buildQEMUEnv()

	// Pipe serial console (QEMU stdio = guest /dev/ttyAMA0) through ring buffer
	stdoutPipe, err := vm.cmd.StdoutPipe()
	if err != nil {
		vm.setError(fmt.Errorf("failed to create stdout pipe: %w", err))
		return
	}
	vm.cmd.Stderr = os.Stderr // QEMU errors still go to app stderr
	stdinPipe, err := vm.cmd.StdinPipe()
	if err != nil {
		vm.setError(fmt.Errorf("failed to create stdin pipe: %w", err))
		return
	}
	vm.consoleStdin = stdinPipe

	if err := vm.cmd.Start(); err != nil {
		vm.setError(fmt.Errorf("failed to start VM: %w", err))
		return
	}

	vm.startTime = time.Now()
	vm.statusMu.Lock()
	vm.status.State = VMStateRunning
	vm.statusMu.Unlock()
	vm.emitStatus()

	// Capture serial console output into ring buffer and emit to frontend
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := stdoutPipe.Read(buf)
			if n > 0 {
				vm.appendConsole(buf[:n])
			}
			if err != nil {
				return
			}
		}
	}()

	// Wait for VM services to be ready
	go vm.waitForReady(ctx)

	// Wait for process to exit
	err = vm.cmd.Wait()

	vm.stackStarted = false
	vm.statusMu.Lock()
	vm.status.APIReady = false
	vm.status.BootStage = ""
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

// waitForReady waits for the VM's services to be ready.
// Times out after 10 minutes total to avoid hanging forever.
func (vm *VMManager) waitForReady(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	bootStart := time.Now()
	const bootTimeout = 10 * time.Minute
	const apiTimeout = 3 * time.Minute // Max time to wait for API after stack starts

	sshReady := false
	zfsInitialized := false
	secretInjected := false
	vm.stackStarted = false
	vm.envUpdated = false
	stackStartedAt := time.Time{}
	apiReady := false
	apiCheckCount := 0

	setBootStage := func(stage string) {
		vm.statusMu.Lock()
		vm.status.BootStage = stage
		vm.statusMu.Unlock()
		vm.emitStatus()
	}

	setBootStage("Booting VM...")

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check overall boot timeout
			if time.Since(bootStart) > bootTimeout {
				log.Printf("Boot timed out after %v", bootTimeout)
				vm.statusMu.Lock()
				vm.status.BootStage = ""
				vm.statusMu.Unlock()
				vm.setError(fmt.Errorf("boot timed out after %d minutes — check VM console for errors", int(bootTimeout.Minutes())))
				return
			}

			// Wait for SSH first (needed for all subsequent steps).
			// We test with an actual SSH command rather than a TCP port check
			// because QEMU opens the host-side port forwarding immediately,
			// long before sshd is running inside the guest.
			if !sshReady {
				cmd := exec.Command("ssh",
					"-o", "StrictHostKeyChecking=no",
					"-o", "UserKnownHostsFile=/dev/null",
					"-o", "ConnectTimeout=2",
					"-p", fmt.Sprintf("%d", vm.config.SSHPort),
					"ubuntu@localhost",
					"echo ready",
				)
				if out, err := cmd.CombinedOutput(); err == nil && strings.Contains(string(out), "ready") {
					sshReady = true
					log.Printf("VM SSH is ready")
				}
			}

			// Initialize ZFS pool and start Docker via SSH.
			// IMPORTANT: Docker must NOT be started before ZFS mounts /var/lib/docker,
			// otherwise Docker runs on the root disk and the zvol mount replaces its
			// working directory mid-operation, causing "Interrupted" image pulls.
			// The ZFS init script handles Docker startup in Step 4, after the mount.
			if sshReady && !zfsInitialized {
				setBootStage("Setting up storage...")
				if err := vm.initZFSPool(); err != nil {
					log.Printf("ZFS init not ready yet: %v", err)
				} else {
					zfsInitialized = true
				}
			}

			// Inject desktop auto-login secret into .env.vm (after ZFS init restores .env.vm)
			if sshReady && zfsInitialized && !secretInjected && vm.desktopSecret != "" {
				setBootStage("Configuring environment...")
				if err := vm.injectDesktopSecret(); err != nil {
					log.Printf("Desktop secret injection: %v", err)
				} else {
					secretInjected = true
				}
			}

			// Start the Helix compose stack after ZFS + Docker + secret are ready.
			// ZFS init ensures Docker is running before returning.
			// Wait for secretInjected so LICENSE_KEY and other env vars are in
			// .env.vm before docker compose reads them.
			if zfsInitialized && secretInjected && !vm.stackStarted {
				setBootStage("Starting Helix services...")
				if err := vm.startHelixStack(); err != nil {
					log.Printf("Helix stack start: %v", err)
				} else {
					vm.stackStarted = true
					stackStartedAt = time.Now()
				}
			}

			// Check if API is responding (HTTP health check, not just TCP)
			// TCP port checks give false positives because QEMU opens the
			// host-side port forwarding before anything is listening in the guest.
			if vm.stackStarted && !apiReady {
				apiCheckCount++
				elapsed := time.Since(stackStartedAt)
				if elapsed > apiTimeout {
					// API didn't come up — check what's wrong with docker compose
					log.Printf("API not ready after %v — checking container status", apiTimeout)
					errMsg := vm.diagnoseAPIFailure()
					vm.statusMu.Lock()
					vm.status.BootStage = ""
					vm.statusMu.Unlock()
					vm.setError(fmt.Errorf("API failed to start: %s", errMsg))
					return
				}

				setBootStage("Starting app...")
				if apiCheckCount%5 == 0 {
					log.Printf("API health check attempt %d (%.0fs since stack start)", apiCheckCount, elapsed.Seconds())
				}
				if vm.checkAPIHealth() {
					vm.statusMu.Lock()
					vm.status.APIReady = true
					vm.status.BootStage = ""
					vm.statusMu.Unlock()
					vm.emitStatus()
					apiReady = true
					if vm.onAPIReady != nil {
						vm.onAPIReady()
					}
				}
			}

			if apiReady {
				return
			}
		}
	}
}

// diagnoseAPIFailure checks docker compose inside the VM to determine why the API isn't starting
func (vm *VMManager) diagnoseAPIFailure() string {
	composeFile := vm.composeFile
	if composeFile == "" {
		composeFile = "docker-compose.dev.yaml"
	}
	cmd := vm.sshCommand(fmt.Sprintf(`cd ~/helix 2>/dev/null && docker compose -f %s ps --format '{{.Service}}: {{.Status}}' 2>/dev/null | head -20`, composeFile))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Sprintf("could not check container status: %v", err)
	}
	status := strings.TrimSpace(string(out))
	if status == "" {
		return "no containers running — docker compose may have failed to start"
	}
	log.Printf("Container status:\n%s", status)

	// Also grab recent API logs if available
	logCmd := vm.sshCommand(fmt.Sprintf(`cd ~/helix 2>/dev/null && docker compose -f %s logs api --tail 10 2>/dev/null`, composeFile))
	logOut, _ := logCmd.CombinedOutput()
	if len(logOut) > 0 {
		log.Printf("API container logs:\n%s", string(logOut))
	}

	return fmt.Sprintf("containers: %s", status)
}

// checkAPIHealth verifies the Helix API is actually responding to HTTP requests.
// A TCP port check is insufficient because QEMU opens host-side port forwarding
// before anything is listening inside the guest.
func (vm *VMManager) checkAPIHealth() bool {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/api/v1/status", vm.config.APIPort))
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode < 500
}

// sshCommand creates an SSH exec.Cmd to the VM with standard options.
// Uses generous keepalive settings to avoid killing long-running operations
// (e.g., docker compose image pulls) under I/O pressure.
func (vm *VMManager) sshCommand(script string) *exec.Cmd {
	return exec.Command("ssh",
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=/dev/null",
		"-o", "ConnectTimeout=5",
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=6",
		"-p", fmt.Sprintf("%d", vm.config.SSHPort),
		"ubuntu@localhost",
		script,
	)
}

// ensureDockerRunning starts Docker if it isn't already running.
// Docker is disabled on boot (storage-init starts it after ZFS mount),
// but we start it here as a safety net regardless of ZFS state.
func (vm *VMManager) ensureDockerRunning() error {
	cmd := vm.sshCommand("if ! systemctl is-active docker >/dev/null 2>&1; then sudo systemctl start docker && echo STARTED; else echo RUNNING; fi")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Docker: %w (output: %s)", err, string(out))
	}
	log.Printf("Docker: %s", strings.TrimSpace(string(out)))
	return nil
}

// startHelixStack starts the Helix docker-compose services inside the VM.
// Auto-detects compose file: docker-compose.dev.yaml (dev/build-from-source)
// or docker-compose.yaml (prod/install.sh with pre-built images).
// In prod mode, also starts sandbox.sh separately (sandbox is not a compose service).
func (vm *VMManager) startHelixStack() error {
	script := `
cd ~/helix 2>/dev/null || exit 0
if [ ! -f .env ] && [ ! -f .env.vm ]; then
    echo 'NO_ENV_FILE'
    exit 0
fi
# Docker Compose always reads .env by default; symlink so it picks up our config
if [ ! -e .env ]; then
    ln -s .env.vm .env
fi
# Detect compose file: dev (build-from-source) vs prod (install.sh)
COMPOSE_FILE=""
if [ -f docker-compose.dev.yaml ]; then
    COMPOSE_FILE="docker-compose.dev.yaml"
elif [ -f docker-compose.yaml ]; then
    COMPOSE_FILE="docker-compose.yaml"
else
    echo 'NO_COMPOSE_FILE'
    exit 0
fi
echo "COMPOSE_FILE=$COMPOSE_FILE"
# Check if stack is already running
if docker compose -f "$COMPOSE_FILE" ps --format '{{.Service}}' 2>/dev/null | grep -q api; then
    echo 'ALREADY_RUNNING'
else
    echo 'Starting Helix stack...'
    docker compose -f "$COMPOSE_FILE" up -d 2>&1
    # In prod mode (install.sh), start sandbox separately via sandbox.sh
    if [ "$COMPOSE_FILE" = "docker-compose.yaml" ] && [ -f sandbox.sh ]; then
        docker stop helix-sandbox 2>/dev/null || true
        docker rm helix-sandbox 2>/dev/null || true
        nohup bash sandbox.sh > /tmp/sandbox.log 2>&1 &
        echo 'SANDBOX_STARTED'
    fi
    echo 'STARTED'
fi
`
	cmd := vm.sshCommand(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to start Helix stack: %w (output: %s)", err, string(out))
	}
	outStr := strings.TrimSpace(string(out))
	log.Printf("Helix stack: %s", outStr)

	// Extract detected compose file from output
	for _, line := range strings.Split(outStr, "\n") {
		if strings.HasPrefix(line, "COMPOSE_FILE=") {
			vm.composeFile = strings.TrimPrefix(line, "COMPOSE_FILE=")
		}
	}

	// If containers were auto-started by Docker (non-clean shutdown) and
	// injectDesktopSecret() updated the env, restart the API to pick up
	// the new values. We do a full `up -d` instead of just restarting API
	// to ensure all containers read the updated .env.
	if strings.Contains(outStr, "ALREADY_RUNNING") && vm.envUpdated {
		composeFile := vm.composeFile
		if composeFile == "" {
			composeFile = "docker-compose.dev.yaml"
		}
		log.Printf("Containers already running with stale env — restarting stack to apply updated settings")
		restart := vm.sshCommand(fmt.Sprintf("cd ~/helix && docker compose -f %s up -d --force-recreate 2>&1", composeFile))
		restartOut, _ := restart.CombinedOutput()
		log.Printf("Stack restart: %s", strings.TrimSpace(string(restartOut)))
		vm.envUpdated = false
	}

	return nil
}

// initZFSPool initializes the ZFS pool on the data disk via SSH.
// Creates the ZFS layout: workspaces dataset and config dataset for
// persistent state across root disk upgrades.
//
// Host Docker runs on the root disk (NOT on a ZFS zvol). This means
// Docker images pre-pulled during VM provisioning are preserved across
// boots. Only sandbox inner Docker storage and workspace data use ZFS
// for dedup benefits.
//
// All steps are idempotent — safe to run on every boot.
func (vm *VMManager) initZFSPool() error {
	script := `
set -e

# =========================================================================
# Step 1: Import or create pool
# =========================================================================
if sudo zpool list helix 2>/dev/null; then
    echo 'ZFS pool helix already exists'
else
    # Find the data disk (second virtio disk, typically /dev/vdb or /dev/vdc)
    DATA_DISK=""
    for disk in /dev/vdb /dev/vdc /dev/vdd; do
        if [ -b "$disk" ] && ! mount | grep -q "$disk"; then
            # Try importing first (upgrade scenario: pool exists on disk but not imported)
            if sudo zpool import -f -d "$disk" helix 2>/dev/null; then
                echo "Imported existing ZFS pool from $disk"
                DATA_DISK="imported"
                break
            fi
            DATA_DISK="$disk"
            break
        fi
    done
    if [ -z "$DATA_DISK" ]; then
        echo 'ERROR: No unmounted data disk found'
        exit 1
    fi
    if [ "$DATA_DISK" != "imported" ]; then
        echo "Creating ZFS pool on $DATA_DISK..."
        sudo zpool create -f helix "$DATA_DISK"
    fi
fi

# Expand pool if disk was resized (no-op if already at full size)
sudo zpool online -e helix $(sudo zpool list -vHP helix 2>/dev/null | awk '/dev/{print $1}' | head -1) 2>/dev/null || true

# =========================================================================
# Step 2: Create datasets
# =========================================================================

# Workspaces dataset (dedup + compression for user workspace data)
if ! sudo zfs list helix/workspaces 2>/dev/null; then
    echo 'Creating helix/workspaces dataset...'
    sudo zfs create -o dedup=on -o compression=lz4 -o atime=off -o mountpoint=/helix/workspaces helix/workspaces
fi

# Docker volumes dataset — persists user data (postgres, keycloak, etc.)
# across root disk upgrades. Mounted at /var/lib/docker/volumes/ so Docker
# named volumes survive while images stay on root disk (pre-baked).
if ! sudo zfs list helix/docker-volumes 2>/dev/null; then
    echo 'Creating helix/docker-volumes dataset...'
    sudo zfs create -o compression=lz4 -o atime=off -o mountpoint=/var/lib/docker/volumes helix/docker-volumes
fi
# Ensure mount exists even if dataset was already created (e.g., after reboot)
if ! mountpoint -q /var/lib/docker/volumes 2>/dev/null; then
    sudo mkdir -p /var/lib/docker/volumes
    sudo zfs mount helix/docker-volumes 2>/dev/null || true
fi

# Container Docker zvol — stores per-session inner dockerd data and BuildKit state.
# The sandbox's own Docker storage stays on the root disk (default named volume)
# so desktop images baked during provisioning persist without transfer.
# This zvol is for data that benefits from ZFS dedup+compression:
#   - Per-session inner dockerd (/helix/container-docker/sessions/{id}/docker/)
#   - BuildKit state (/helix/container-docker/buildkit/)
# Hydra bind-mounts these paths into desktop containers and the BuildKit container.
ZVOL_SIZE=200G
ZVOL_DEV=/dev/zvol/helix/container-docker
if ! sudo zfs list helix/container-docker 2>/dev/null; then
    # Migrate from old name if it exists
    if sudo zfs list helix/sandbox-docker 2>/dev/null; then
        echo "Renaming helix/sandbox-docker zvol to helix/container-docker..."
        sudo umount /helix/sandbox-docker 2>/dev/null || true
        sudo zfs rename helix/sandbox-docker helix/container-docker
    else
        echo "Creating helix/container-docker zvol (${ZVOL_SIZE}, dedup + compression)..."
        sudo zfs create -V "$ZVOL_SIZE" -s -o dedup=on -o compression=lz4 helix/container-docker
        # Wait for device node
        for i in $(seq 1 10); do [ -e "$ZVOL_DEV" ] && break; sleep 1; done
        echo 'Formatting container-docker zvol as ext4...'
        sudo mkfs.ext4 -q -L container-docker "$ZVOL_DEV"
    fi
fi
# Mount the zvol
if ! mountpoint -q /helix/container-docker 2>/dev/null; then
    sudo mkdir -p /helix/container-docker
    if [ -e "$ZVOL_DEV" ]; then
        sudo mount "$ZVOL_DEV" /helix/container-docker
    fi
fi
# Create subdirectories for Hydra
sudo mkdir -p /helix/container-docker/sessions
sudo mkdir -p /helix/container-docker/buildkit

# Config dataset (persistent state surviving root disk swaps)
if ! sudo zfs list helix/config 2>/dev/null; then
    echo 'Creating helix/config dataset...'
    sudo zfs create -o compression=lz4 -o mountpoint=/helix/config helix/config
fi

# =========================================================================
# Step 3: Persist / restore config (SSH keys, machine-id, authorized_keys)
# =========================================================================

# SSH host keys
if [ ! -d /helix/config/ssh ]; then
    # First boot: copy keys TO config
    echo 'Persisting SSH host keys to /helix/config/ssh/...'
    sudo mkdir -p /helix/config/ssh
    sudo cp /etc/ssh/ssh_host_* /helix/config/ssh/
    # Also persist authorized_keys if they exist
    if [ -f /home/ubuntu/.ssh/authorized_keys ]; then
        sudo cp /home/ubuntu/.ssh/authorized_keys /helix/config/ssh/authorized_keys
    fi
else
    # Upgrade boot: restore keys FROM config
    echo 'Restoring SSH host keys from /helix/config/ssh/...'
    sudo cp /helix/config/ssh/ssh_host_* /etc/ssh/
    sudo chmod 600 /etc/ssh/ssh_host_*_key
    sudo chmod 644 /etc/ssh/ssh_host_*_key.pub
    sudo systemctl restart sshd 2>/dev/null || true
    # Restore authorized_keys
    if [ -f /helix/config/ssh/authorized_keys ]; then
        mkdir -p /home/ubuntu/.ssh
        sudo cp /helix/config/ssh/authorized_keys /home/ubuntu/.ssh/authorized_keys
        sudo chmod 600 /home/ubuntu/.ssh/authorized_keys
        sudo chown ubuntu:ubuntu /home/ubuntu/.ssh/authorized_keys
    fi
fi

# Machine ID
if [ ! -f /helix/config/machine-id ]; then
    sudo cp /etc/machine-id /helix/config/machine-id
else
    sudo cp /helix/config/machine-id /etc/machine-id
    sudo systemd-machine-id-commit 2>/dev/null || true
fi

# Helix .env.vm
if [ -f /home/ubuntu/helix/.env.vm ] && [ ! -f /helix/config/env.vm ]; then
    sudo cp /home/ubuntu/helix/.env.vm /helix/config/env.vm
elif [ -f /helix/config/env.vm ] && [ ! -f /home/ubuntu/helix/.env.vm ]; then
    sudo mkdir -p /home/ubuntu/helix
    sudo cp /helix/config/env.vm /home/ubuntu/helix/.env.vm
    sudo chown ubuntu:ubuntu /home/ubuntu/helix/.env.vm
fi

# =========================================================================
# Step 4: Ensure Docker is running
# =========================================================================
# Host Docker runs on root disk (images pre-baked during provisioning).
# Only sandbox inner Docker and workspace data use ZFS.
if ! systemctl is-active docker >/dev/null 2>&1; then
    echo 'Starting Docker...'
    sudo systemctl start docker
else
    echo 'Docker already running'
fi

echo 'ZFS storage ready'
`
	cmd := vm.sshCommand(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SSH command failed: %w (output: %s)", err, string(out))
	}
	log.Printf("ZFS init: %s", string(out))
	return nil
}

// injectDesktopSecret ensures the desktop auto-login secret, admin config,
// and console password are applied to the VM. Runs via SSH after initZFSPool
// restores .env.vm. Idempotent — only modifies files if values changed.
func (vm *VMManager) injectDesktopSecret() error {
	script := fmt.Sprintf(`
ENV_FILE=/home/ubuntu/helix/.env
if [ ! -f "$ENV_FILE" ]; then
    ENV_FILE=/home/ubuntu/helix/.env.vm
fi
if [ ! -f "$ENV_FILE" ]; then
    # Create .env.vm if neither file exists (e.g., after factory reset re-download)
    touch "$ENV_FILE"
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

# Admin user config
ADMIN_VAL="%s"
if grep -q '^ADMIN_USER_IDS=' "$ENV_FILE" 2>/dev/null; then
    CURRENT=$(grep '^ADMIN_USER_IDS=' "$ENV_FILE" | cut -d= -f2-)
    if [ "$CURRENT" != "$ADMIN_VAL" ]; then
        sed -i "s|^ADMIN_USER_IDS=.*|ADMIN_USER_IDS=$ADMIN_VAL|" "$ENV_FILE"
        CHANGED=1
    fi
else
    echo "ADMIN_USER_IDS=$ADMIN_VAL" >> "$ENV_FILE"
    CHANGED=1
fi

# Registration
REG_VAL="%s"
if grep -q '^AUTH_REGISTRATION_ENABLED=' "$ENV_FILE" 2>/dev/null; then
    CURRENT=$(grep '^AUTH_REGISTRATION_ENABLED=' "$ENV_FILE" | cut -d= -f2-)
    if [ "$CURRENT" != "$REG_VAL" ]; then
        sed -i "s|^AUTH_REGISTRATION_ENABLED=.*|AUTH_REGISTRATION_ENABLED=$REG_VAL|" "$ENV_FILE"
        CHANGED=1
    fi
else
    echo "AUTH_REGISTRATION_ENABLED=$REG_VAL" >> "$ENV_FILE"
    CHANGED=1
fi

# Enable user-configured inference providers (OpenAI, Anthropic, etc.)
if ! grep -q '^ENABLE_CUSTOM_USER_PROVIDERS=' "$ENV_FILE" 2>/dev/null; then
    echo 'ENABLE_CUSTOM_USER_PROVIDERS=true' >> "$ENV_FILE"
    CHANGED=1
fi

# Max concurrent desktops (hard limit: 15 QEMU video outputs)
if ! grep -q '^PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=' "$ENV_FILE" 2>/dev/null; then
    echo 'PROJECTS_FREE_MAX_CONCURRENT_DESKTOPS=15' >> "$ENV_FILE"
    CHANGED=1
fi

# Identify this as the Mac Desktop edition for Launchpad telemetry
if ! grep -q '^HELIX_EDITION=' "$ENV_FILE" 2>/dev/null; then
    echo 'HELIX_EDITION=mac-desktop' >> "$ENV_FILE"
    CHANGED=1
fi

# Container Docker storage — Hydra bind-mounts this into desktop containers for their
# inner dockerd and BuildKit state. The sandbox's own Docker uses a named volume on
# the root disk so desktop images from provisioning persist without transfer.
if ! grep -q '^CONTAINER_DOCKER_PATH=' "$ENV_FILE" 2>/dev/null; then
    echo 'CONTAINER_DOCKER_PATH=/helix/container-docker' >> "$ENV_FILE"
    CHANGED=1
fi
# Remove old SANDBOX_DOCKER_STORAGE if present (migrated to CONTAINER_DOCKER_PATH)
if grep -q '^SANDBOX_DOCKER_STORAGE=' "$ENV_FILE" 2>/dev/null; then
    sed -i '/^SANDBOX_DOCKER_STORAGE=/d' "$ENV_FILE"
    CHANGED=1
fi

# QEMU frame export port — tells sandbox/hydra which port helix-frame-export
# listens on, so desktop containers connect to the right QEMU instance.
FRAME_PORT="%d"
if grep -q '^HELIX_FRAME_EXPORT_PORT=' "$ENV_FILE" 2>/dev/null; then
    CURRENT=$(grep '^HELIX_FRAME_EXPORT_PORT=' "$ENV_FILE" | cut -d= -f2-)
    if [ "$CURRENT" != "$FRAME_PORT" ]; then
        sed -i "s|^HELIX_FRAME_EXPORT_PORT=.*|HELIX_FRAME_EXPORT_PORT=$FRAME_PORT|" "$ENV_FILE"
        CHANGED=1
    fi
else
    echo "HELIX_FRAME_EXPORT_PORT=$FRAME_PORT" >> "$ENV_FILE"
    CHANGED=1
fi

# Enable code-macos compose profile so sandbox-macos starts with docker compose up -d
if grep -q '^COMPOSE_PROFILES=' "$ENV_FILE" 2>/dev/null; then
    CURRENT=$(grep '^COMPOSE_PROFILES=' "$ENV_FILE" | cut -d= -f2-)
    if [ "$CURRENT" != "code-macos" ]; then
        sed -i "s|^COMPOSE_PROFILES=.*|COMPOSE_PROFILES=code-macos|" "$ENV_FILE"
        CHANGED=1
    fi
else
    echo 'COMPOSE_PROFILES=code-macos' >> "$ENV_FILE"
    CHANGED=1
fi

# License key
LICENSE_KEY="%s"
if [ -n "$LICENSE_KEY" ]; then
    if grep -q '^LICENSE_KEY=' "$ENV_FILE" 2>/dev/null; then
        CURRENT=$(grep '^LICENSE_KEY=' "$ENV_FILE" | cut -d= -f2-)
        if [ "$CURRENT" != "$LICENSE_KEY" ]; then
            sed -i "s|^LICENSE_KEY=.*|LICENSE_KEY=$LICENSE_KEY|" "$ENV_FILE"
            CHANGED=1
        fi
    else
        echo "LICENSE_KEY=$LICENSE_KEY" >> "$ENV_FILE"
        CHANGED=1
    fi
fi

# Persist env to ZFS
if [ $CHANGED -eq 1 ]; then
    sudo cp "$ENV_FILE" /helix/config/env.vm
    echo 'ENV_UPDATED'
fi

# Set ubuntu user password and persist to ZFS
PASS_FILE=/helix/config/console_password
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
`, vm.desktopSecret, vm.desktopSecret, vm.desktopSecret,
		vm.adminUserIDs(), vm.registrationEnabled(),
		vm.config.FrameExportPort,
		vm.licenseKey,
		vm.consolePassword, vm.consolePassword, vm.consolePassword)

	cmd := vm.sshCommand(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inject desktop secret failed: %w (output: %s)", err, string(out))
	}
	outStr := string(out)
	if strings.Contains(outStr, "ENV_UPDATED") {
		if vm.stackStarted {
			// Stack is already running (runtime settings change, e.g. user
			// activated a license key). Restart API to pick up new env.
			composeFile := vm.composeFile
			if composeFile == "" {
				composeFile = "docker-compose.dev.yaml"
			}
			log.Printf("Desktop secret injected into .env — restarting API container")
			restart := vm.sshCommand(fmt.Sprintf("cd ~/helix && docker compose -f %s down api && docker compose -f %s up -d api 2>&1 || true", composeFile, composeFile))
			restartOut, _ := restart.CombinedOutput()
			log.Printf("API restart: %s", string(restartOut))
		} else {
			// During boot — mark env as updated so startHelixStack() can
			// restart containers if they were auto-started by Docker with
			// stale env from a previous non-clean shutdown.
			vm.envUpdated = true
			log.Printf("Desktop secret injected into .env.vm — flagged for restart if containers already running")
		}
	}
	if strings.Contains(outStr, "PASS_UPDATED") {
		log.Printf("Console password updated for ubuntu user")
	}
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

// GetVsockCID returns the vsock CID for the VM
func (vm *VMManager) GetVsockCID() uint32 {
	return vm.config.VsockCID
}

// GetSSHCommand returns the SSH command to connect to the VM
func (vm *VMManager) GetSSHCommand() string {
	return fmt.Sprintf("ssh -p %d ubuntu@localhost", vm.config.SSHPort)
}

// getAppBundlePath returns the path to the running .app bundle, if any.
// Returns empty string if not running from an app bundle.
func (vm *VMManager) getAppBundlePath() string {
	return getAppBundlePath()
}

// findQEMUBinary locates the QEMU binary. Search order:
//  1. HELIX_QEMU_PATH environment variable (explicit override)
//  2. Standalone dev QEMU: build/dev-qemu/qemu-system-aarch64
//     (signed independently — immune to wails dev breaking the app bundle seal)
//  3. Bundled in app: Contents/MacOS/qemu-system-aarch64 (production mode)
//  4. System PATH: qemu-system-aarch64
//
// QEMU is built as a dylib + thin wrapper. The wrapper (75KB) has main() and
// loads libqemu-aarch64-softmmu.dylib via @executable_path. You cannot execute
// a .dylib directly — the wrapper executable is required.
func (vm *VMManager) findQEMUBinary() string {
	// Explicit override via environment variable
	if envPath := os.Getenv("HELIX_QEMU_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	// Check standalone dev QEMU (signed independently of app bundle — works
	// even when wails dev has broken the bundle's CodeResources seal)
	devQemu := filepath.Join("build", "dev-qemu", "qemu-system-aarch64")
	if _, err := os.Stat(devQemu); err == nil {
		if abs, err := filepath.Abs(devQemu); err == nil {
			log.Printf("Using dev QEMU: %s", abs)
			return abs
		}
		return devQemu
	}

	// Check app bundle (production mode — only reached when dev-qemu doesn't exist)
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
//  2. Build output: build/bin/Helix.app/Contents/Resources/firmware/<name> (dev mode)
//  3. Homebrew: /opt/homebrew/share/qemu/<name>
func (vm *VMManager) findFirmware(name string) string {
	// Check app bundle first
	appPath := vm.getAppBundlePath()
	if appPath != "" {
		bundled := filepath.Join(appPath, "Contents", "Resources", "firmware", name)
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	// Check build output directory (dev mode)
	devBuild := filepath.Join("build", "bin", "Helix.app", "Contents", "Resources", "firmware", name)
	if _, err := os.Stat(devBuild); err == nil {
		if abs, err := filepath.Abs(devBuild); err == nil {
			return abs
		}
		return devBuild
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
//  2. Build output: build/bin/Helix.app/Contents/Resources/vulkan/icd.d/... (dev mode)
//  3. UTM.app: /Applications/UTM.app/Contents/Resources/vulkan/icd.d/kosmickrisp_mesa_icd.json
func (vm *VMManager) findVulkanICD() string {
	icdRel := filepath.Join("vulkan", "icd.d", "kosmickrisp_mesa_icd.json")

	// Check app bundle first
	appPath := vm.getAppBundlePath()
	if appPath != "" {
		bundled := filepath.Join(appPath, "Contents", "Resources", icdRel)
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	// Check build output directory (dev mode)
	devBuild := filepath.Join("build", "bin", "Helix.app", "Contents", "Resources", icdRel)
	if _, err := os.Stat(devBuild); err == nil {
		if abs, err := filepath.Abs(devBuild); err == nil {
			return abs
		}
		return devBuild
	}

	// Fall back to UTM.app
	utmPath := filepath.Join("/Applications/UTM.app/Contents/Resources", icdRel)
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

	// Tell ANGLE to use the Metal backend for EGL/GLES on macOS.
	// Without this, ANGLE can't initialize and QEMU fails with
	// "No provider of eglCreateImageKHR found". UTM sets this in
	// UTMQemuSystem.m's setRendererBackend().
	env = append(env, "ANGLE_DEFAULT_PLATFORM=metal")

	// Use KosmicKrisp Vulkan driver — check bundled location first, then UTM.app
	icdPath := vm.findVulkanICD()
	if icdPath != "" {
		env = append(env, "VK_DRIVER_FILES="+icdPath)
	}

	return env
}

const maxConsoleSize = 256 * 1024 // 256KB ring buffer

// appendConsole appends data to the serial console ring buffer and emits to frontend
func (vm *VMManager) appendConsole(data []byte) {
	vm.consoleMu.Lock()
	vm.consoleBuf = append(vm.consoleBuf, data...)
	// Trim to ring buffer size
	if len(vm.consoleBuf) > maxConsoleSize {
		vm.consoleBuf = vm.consoleBuf[len(vm.consoleBuf)-maxConsoleSize:]
	}
	vm.consoleMu.Unlock()
	// Emit to frontend for xterm.js
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

// SendConsoleInput sends input to the serial console (guest /dev/ttyAMA0).
// Filters out terminal responses (CPR, etc.) that xterm.js generates
// automatically — these would echo back as garbage text in the guest.
func (vm *VMManager) SendConsoleInput(input string) error {
	if vm.consoleStdin == nil {
		return fmt.Errorf("console not connected")
	}
	// Strip Cursor Position Report responses (\e[row;colR) generated by xterm.js
	// in response to DSR queries (\e[6n) from the guest's getty/shell.
	filtered := cprPattern.ReplaceAllString(input, "")
	if filtered == "" {
		return nil
	}
	_, err := vm.consoleStdin.Write([]byte(filtered))
	return err
}
