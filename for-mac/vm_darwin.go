//go:build darwin

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
	"strings"
	"time"
)

// getSpiceSocketPath returns the path for the SPICE Unix socket
func (vm *VMManager) getSpiceSocketPath() string {
	return filepath.Join(os.TempDir(), "helix-spice.sock")
}

// bindAddr returns the address to bind forwarded ports to.
func (vm *VMManager) bindAddr() string {
	if vm.config.ExposeOnNetwork {
		return "0.0.0.0"
	}
	return ""
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

	if _, err := os.Stat(rootDisk); err != nil {
		log.Printf("VM root disk not found at %s — download required", vmDir)

		// Copy EFI vars from bundle if available
		bundlePath := getAppBundlePath()
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

	// ZFS data disk is created locally on first boot
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

// createEmptyQcow2 creates an empty thin-provisioned qcow2 image
func (vm *VMManager) createEmptyQcow2(path, size string) error {
	qemuImg := vm.findQEMUImg()
	if qemuImg == "" {
		return fmt.Errorf("qemu-img not found")
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
		return fmt.Errorf("qemu-img not found")
	}
	cmd := exec.Command(qemuImg, "resize", path, size)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// findQEMUImg locates the qemu-img binary
func (vm *VMManager) findQEMUImg() string {
	qemuPath := vm.findQEMUBinary()
	if qemuPath != "" {
		qemuImg := filepath.Join(filepath.Dir(qemuPath), "qemu-img")
		if _, err := os.Stat(qemuImg); err == nil {
			return qemuImg
		}
	}
	if _, err := os.Stat("/opt/homebrew/bin/qemu-img"); err == nil {
		return "/opt/homebrew/bin/qemu-img"
	}
	path, err := exec.LookPath("qemu-img")
	if err == nil {
		return path
	}
	return ""
}

// Start starts the QEMU VM
func (vm *VMManager) Start() error {
	vm.statusMu.Lock()
	if vm.status.State != VMStateStopped && vm.status.State != VMStateError {
		vm.statusMu.Unlock()
		return fmt.Errorf("VM is not stopped (current state: %s)", vm.status.State)
	}
	vm.status.State = VMStateStarting
	vm.status.ErrorMsg = ""
	vm.statusMu.Unlock()
	vm.emitStatus()

	vm.killStaleQEMU()

	spiceSock := vm.getSpiceSocketPath()
	if _, err := os.Stat(spiceSock); err == nil {
		os.Remove(spiceSock)
	}

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
func (vm *VMManager) killStaleQEMU() {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vm.config.QMPPort), 500*time.Millisecond)
	if err != nil {
		return
	}
	conn.Close()
	log.Printf("QMP port %d is in use — looking for stale QEMU process", vm.config.QMPPort)

	cmd := exec.Command("lsof", "-ti", fmt.Sprintf("tcp:%d", vm.config.QMPPort))
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		log.Printf("Could not find PID holding port %d", vm.config.QMPPort)
		return
	}

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

	for i := 0; i < 10; i++ {
		time.Sleep(200 * time.Millisecond)
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("localhost:%d", vm.config.QMPPort), 200*time.Millisecond)
		if err != nil {
			log.Printf("QMP port %d is now free", vm.config.QMPPort)
			return
		}
		conn.Close()
	}
	log.Printf("Warning: QMP port %d still in use after killing stale process", vm.config.QMPPort)
}

// runVM runs the QEMU process
func (vm *VMManager) runVM(ctx context.Context) {
	vmDir := vm.getVMDir()
	imagePath := vm.getVMImagePath()
	zfsDiskPath := vm.getZFSDiskPath()

	efiCode := vm.findFirmware("edk2-aarch64-code.fd")
	if efiCode == "" {
		vm.setError(fmt.Errorf("EFI firmware not found. Install QEMU via 'brew install qemu' or use the bundled app"))
		return
	}

	efiVars := filepath.Join(vmDir, "efi_vars.fd")
	if _, err := os.Stat(efiVars); os.IsNotExist(err) {
		efiVarsTemplate := vm.findFirmware("edk2-arm-vars.fd")
		if efiVarsTemplate != "" {
			if data, readErr := os.ReadFile(efiVarsTemplate); readErr == nil {
				os.MkdirAll(vmDir, 0755)
				os.WriteFile(efiVars, data, 0644)
			}
		}
		if _, checkErr := os.Stat(efiVars); os.IsNotExist(checkErr) {
			if f, createErr := os.Create(efiVars); createErr == nil {
				f.Truncate(64 * 1024 * 1024)
				f.Close()
			}
		}
	}

	args := []string{
		"-machine", "virt,accel=hvf,highmem=on",
		"-cpu", "host",
		"-smp", fmt.Sprintf("%d", vm.config.CPUs),
		"-m", fmt.Sprintf("%d", vm.config.MemoryMB),
		"-drive", fmt.Sprintf("if=pflash,format=raw,readonly=on,file=%s", efiCode),
		"-drive", fmt.Sprintf("if=pflash,format=raw,file=%s", efiVars),
		"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio,cache=writeback", imagePath),
	}

	if _, err := os.Stat(zfsDiskPath); err == nil {
		args = append(args,
			"-drive", fmt.Sprintf("file=%s,format=qcow2,if=virtio", zfsDiskPath),
		)
	}

	args = append(args,
		"-device", "virtio-net-pci,netdev=net0,romfile=",
		"-netdev", fmt.Sprintf("user,id=net0,hostfwd=tcp::%d-:22,hostfwd=tcp:%s:%d-:8080",
			vm.config.SSHPort, vm.bindAddr(), vm.config.APIPort),
		"-device", fmt.Sprintf("virtio-gpu-gl-pci,id=gpu0,hostmem=256M,blob=true,venus=true,edid=on,xres=5120,yres=2880,helix-port=%d", vm.config.FrameExportPort),
		"-spice", fmt.Sprintf("unix=on,addr=%s,disable-ticketing=on,gl=es", vm.getSpiceSocketPath()),
		"-serial", "mon:stdio",
		"-qmp", fmt.Sprintf("tcp:localhost:%d,server,nowait", vm.config.QMPPort),
	)

	qemuPath := vm.findQEMUBinary()
	if qemuPath == "" {
		vm.setError(fmt.Errorf("QEMU not found. Install via 'brew install qemu' or use the bundled app"))
		return
	}

	vm.cmd = exec.CommandContext(ctx, qemuPath, args...)
	vm.cmd.Env = vm.buildQEMUEnv()

	stdoutPipe, err := vm.cmd.StdoutPipe()
	if err != nil {
		vm.setError(fmt.Errorf("failed to create stdout pipe: %w", err))
		return
	}
	vm.cmd.Stderr = os.Stderr
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

	// Capture serial console output
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

	go vm.waitForReady(ctx)

	err = vm.cmd.Wait()

	vm.statusMu.Lock()
	if ctx.Err() != nil {
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

// waitForReady waits for the macOS QEMU VM services to be ready.
func (vm *VMManager) waitForReady(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	bootStart := time.Now()
	const bootTimeout = 10 * time.Minute
	const apiTimeout = 3 * time.Minute

	sshReady := false
	zfsInitialized := false
	secretInjected := false
	stackStarted := false
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
			if time.Since(bootStart) > bootTimeout {
				log.Printf("Boot timed out after %v", bootTimeout)
				vm.statusMu.Lock()
				vm.status.BootStage = ""
				vm.statusMu.Unlock()
				vm.setError(fmt.Errorf("boot timed out after %d minutes — check VM console for errors", int(bootTimeout.Minutes())))
				return
			}

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

			if sshReady && !zfsInitialized {
				setBootStage("Setting up storage...")
				if err := vm.initZFSPool(); err != nil {
					log.Printf("ZFS init not ready yet: %v", err)
				} else {
					zfsInitialized = true
				}
			}

			if sshReady && zfsInitialized && !secretInjected && vm.desktopSecret != "" {
				setBootStage("Configuring environment...")
				if err := vm.injectDesktopSecret(); err != nil {
					log.Printf("Desktop secret injection: %v", err)
				} else {
					secretInjected = true
				}
			}

			if zfsInitialized && !stackStarted {
				setBootStage("Starting Helix services...")
				if err := vm.startHelixStack(); err != nil {
					log.Printf("Helix stack start: %v", err)
				} else {
					stackStarted = true
					stackStartedAt = time.Now()
				}
			}

			if stackStarted && !apiReady {
				apiCheckCount++
				elapsed := time.Since(stackStartedAt)
				if elapsed > apiTimeout {
					log.Printf("API not ready after %v — checking container status", apiTimeout)
					errMsg := vm.diagnoseAPIFailure()
					vm.statusMu.Lock()
					vm.status.BootStage = ""
					vm.statusMu.Unlock()
					vm.setError(fmt.Errorf("API failed to start: %s", errMsg))
					return
				}

				setBootStage("Waiting for API...")
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
				}
			}

			if apiReady {
				return
			}
		}
	}
}

// runInVM creates an SSH command to execute a script inside the QEMU VM.
func (vm *VMManager) runInVM(script string) *exec.Cmd {
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

// Stop stops the QEMU VM gracefully
func (vm *VMManager) Stop() error {
	vm.statusMu.Lock()
	if vm.status.State != VMStateRunning && vm.status.State != VMStateStarting {
		vm.statusMu.Unlock()
		return fmt.Errorf("VM is not running (current state: %s)", vm.status.State)
	}
	vm.status.State = VMStateStopping
	vm.statusMu.Unlock()
	vm.emitStatus()

	vm.sendQMPCommand("system_powerdown")
	time.Sleep(5 * time.Second)

	if vm.cancelFunc != nil {
		vm.cancelFunc()
	}
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

	buf := make([]byte, 1024)
	conn.Read(buf)
	conn.Write([]byte(`{"execute": "qmp_capabilities"}`))
	conn.Read(buf)
	conn.Write([]byte(fmt.Sprintf(`{"execute": "%s"}`, command)))
	conn.Read(buf)
	return nil
}

// SendConsoleInput sends input to the QEMU serial console
func (vm *VMManager) SendConsoleInput(input string) error {
	if vm.consoleStdin == nil {
		return fmt.Errorf("console not connected")
	}
	_, err := vm.consoleStdin.Write([]byte(input))
	return err
}

// GetVsockCID returns the vsock CID for the VM
func (vm *VMManager) GetVsockCID() uint32 {
	return vm.config.VsockCID
}

// GetSSHCommand returns the SSH command to connect to the VM
func (vm *VMManager) GetSSHCommand() string {
	return fmt.Sprintf("ssh -p %d ubuntu@localhost", vm.config.SSHPort)
}

// findQEMUBinary locates the QEMU binary on macOS
func (vm *VMManager) findQEMUBinary() string {
	if envPath := os.Getenv("HELIX_QEMU_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath
		}
	}

	devQemu := filepath.Join("build", "dev-qemu", "qemu-system-aarch64")
	if _, err := os.Stat(devQemu); err == nil {
		if abs, err := filepath.Abs(devQemu); err == nil {
			log.Printf("Using dev QEMU: %s", abs)
			return abs
		}
		return devQemu
	}

	appPath := getAppBundlePath()
	if appPath != "" {
		bundled := filepath.Join(appPath, "Contents", "MacOS", "qemu-system-aarch64")
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	path, err := exec.LookPath("qemu-system-aarch64")
	if err == nil {
		return path
	}
	return ""
}

// findFirmware locates an EFI firmware file on macOS
func (vm *VMManager) findFirmware(name string) string {
	appPath := getAppBundlePath()
	if appPath != "" {
		bundled := filepath.Join(appPath, "Contents", "Resources", "firmware", name)
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	devBuild := filepath.Join("build", "bin", "Helix.app", "Contents", "Resources", "firmware", name)
	if _, err := os.Stat(devBuild); err == nil {
		if abs, err := filepath.Abs(devBuild); err == nil {
			return abs
		}
		return devBuild
	}

	brewPath := filepath.Join("/opt/homebrew/share/qemu", name)
	if _, err := os.Stat(brewPath); err == nil {
		return brewPath
	}
	return ""
}

// findVulkanICD locates the KosmicKrisp Vulkan ICD JSON
func (vm *VMManager) findVulkanICD() string {
	icdRel := filepath.Join("vulkan", "icd.d", "kosmickrisp_mesa_icd.json")

	appPath := getAppBundlePath()
	if appPath != "" {
		bundled := filepath.Join(appPath, "Contents", "Resources", icdRel)
		if _, err := os.Stat(bundled); err == nil {
			return bundled
		}
	}

	devBuild := filepath.Join("build", "bin", "Helix.app", "Contents", "Resources", icdRel)
	if _, err := os.Stat(devBuild); err == nil {
		if abs, err := filepath.Abs(devBuild); err == nil {
			return abs
		}
		return devBuild
	}

	utmPath := filepath.Join("/Applications/UTM.app/Contents/Resources", icdRel)
	if _, err := os.Stat(utmPath); err == nil {
		return utmPath
	}
	return ""
}

// buildQEMUEnv returns the environment variables for the QEMU process.
func (vm *VMManager) buildQEMUEnv() []string {
	env := os.Environ()
	icdPath := vm.findVulkanICD()
	if icdPath != "" {
		env = append(env, "VK_DRIVER_FILES="+icdPath)
	}
	return env
}

// initZFSPool initializes the ZFS pool on the data disk via SSH.
func (vm *VMManager) initZFSPool() error {
	script := `
set -e

if sudo zpool list helix 2>/dev/null; then
    echo 'ZFS pool helix already exists'
else
    DATA_DISK=""
    for disk in /dev/vdb /dev/vdc /dev/vdd; do
        if [ -b "$disk" ] && ! mount | grep -q "$disk"; then
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

sudo zpool online -e helix $(sudo zpool list -vHP helix 2>/dev/null | awk '/dev/{print $1}' | head -1) 2>/dev/null || true

if ! sudo zfs list helix/workspaces 2>/dev/null; then
    echo 'Creating helix/workspaces dataset...'
    sudo zfs create -o dedup=on -o compression=lz4 -o atime=off -o mountpoint=/helix/workspaces helix/workspaces
fi

if ! sudo zfs list helix/docker 2>/dev/null; then
    echo 'Creating helix/docker zvol (200GB)...'
    sudo zfs create -V 200G -o dedup=on -o compression=lz4 -o volblocksize=64k helix/docker
    sleep 2
    ZVOL_DEV=$(ls /dev/zvol/helix/docker 2>/dev/null || echo "/dev/zd0")
    echo "Formatting $ZVOL_DEV as ext4..."
    sudo mkfs.ext4 -L helix-docker "$ZVOL_DEV"
fi

if ! mountpoint -q /var/lib/docker 2>/dev/null; then
    ZVOL_DEV=$(ls /dev/zvol/helix/docker 2>/dev/null || echo "/dev/zd0")
    if [ -b "$ZVOL_DEV" ]; then
        echo "Mounting Docker zvol at /var/lib/docker..."
        sudo mkdir -p /var/lib/docker
        sudo mount "$ZVOL_DEV" /var/lib/docker 2>/dev/null || {
            if mountpoint -q /var/lib/docker 2>/dev/null; then
                echo "Docker zvol already mounted (fstab race)"
            else
                echo "WARNING: Failed to mount Docker zvol"
            fi
        }
        if ! grep -q 'helix-docker' /etc/fstab 2>/dev/null; then
            echo "LABEL=helix-docker /var/lib/docker ext4 defaults,nofail 0 2" | sudo tee -a /etc/fstab
        fi
    fi
else
    echo "Docker zvol already mounted at /var/lib/docker"
fi

if ! sudo zfs list helix/config 2>/dev/null; then
    echo 'Creating helix/config dataset...'
    sudo zfs create -o compression=lz4 -o mountpoint=/helix/config helix/config
fi

if [ ! -d /helix/config/ssh ]; then
    echo 'Persisting SSH host keys to /helix/config/ssh/...'
    sudo mkdir -p /helix/config/ssh
    sudo cp /etc/ssh/ssh_host_* /helix/config/ssh/
    if [ -f /home/ubuntu/.ssh/authorized_keys ]; then
        sudo cp /home/ubuntu/.ssh/authorized_keys /helix/config/ssh/authorized_keys
    fi
else
    echo 'Restoring SSH host keys from /helix/config/ssh/...'
    sudo cp /helix/config/ssh/ssh_host_* /etc/ssh/
    sudo chmod 600 /etc/ssh/ssh_host_*_key
    sudo chmod 644 /etc/ssh/ssh_host_*_key.pub
    sudo systemctl restart sshd 2>/dev/null || true
    if [ -f /helix/config/ssh/authorized_keys ]; then
        mkdir -p /home/ubuntu/.ssh
        sudo cp /helix/config/ssh/authorized_keys /home/ubuntu/.ssh/authorized_keys
        sudo chmod 600 /home/ubuntu/.ssh/authorized_keys
        sudo chown ubuntu:ubuntu /home/ubuntu/.ssh/authorized_keys
    fi
fi

if [ ! -f /helix/config/machine-id ]; then
    sudo cp /etc/machine-id /helix/config/machine-id
else
    sudo cp /helix/config/machine-id /etc/machine-id
    sudo systemd-machine-id-commit 2>/dev/null || true
fi

if [ -f /home/ubuntu/helix/.env.vm ] && [ ! -f /helix/config/env.vm ]; then
    sudo cp /home/ubuntu/helix/.env.vm /helix/config/env.vm
elif [ -f /helix/config/env.vm ] && [ ! -f /home/ubuntu/helix/.env.vm ]; then
    sudo mkdir -p /home/ubuntu/helix
    sudo cp /helix/config/env.vm /home/ubuntu/helix/.env.vm
    sudo chown ubuntu:ubuntu /home/ubuntu/helix/.env.vm
fi

# Ensure Docker is running on the correct /var/lib/docker
if systemctl is-active docker >/dev/null 2>&1; then
    if mountpoint -q /var/lib/docker 2>/dev/null; then
        echo 'Docker already running on zvol'
    else
        echo 'Docker running but not on zvol — restarting...'
        sudo systemctl restart docker
    fi
else
    echo 'Starting Docker...'
    sudo systemctl start docker
fi

echo 'ZFS storage ready'
`
	cmd := vm.runInVM(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("SSH command failed: %w (output: %s)", err, string(out))
	}
	log.Printf("ZFS init: %s", string(out))
	return nil
}

// copyFile copies a file from src to dst
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
		os.Remove(dst)
		return err
	}
	return dstFile.Close()
}
