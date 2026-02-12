//go:build windows

package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"unsafe"
)

// getHelixDataDir returns the Windows-conventional data directory for Helix.
// Uses %APPDATA%\Helix (e.g., C:\Users\<user>\AppData\Roaming\Helix).
func getHelixDataDir() string {
	appdata := os.Getenv("APPDATA")
	if appdata == "" {
		home, _ := os.UserHomeDir()
		appdata = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(appdata, "Helix")
}

// getAppBundlePath returns empty string on Windows (no .app bundles).
func getAppBundlePath() string {
	return ""
}

// getExeDir returns the directory containing the running executable.
// Used to find bundled resources on Windows.
func getExeDir() string {
	execPath, err := os.Executable()
	if err != nil {
		return "."
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return "."
	}
	return filepath.Dir(execPath)
}

// openBrowser opens a URL in the default browser on Windows
func openBrowser(url string) error {
	cmd := exec.Command("cmd", "/c", "start", "", url)
	return cmd.Start()
}

// preferredNIC returns empty string on Windows — no preferred interface name.
func preferredNIC() string {
	return ""
}

// getSystemMemoryMB returns the total physical memory in MB using Windows API.
func getSystemMemoryMB() int {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	// MEMORYSTATUSEX structure
	type memoryStatusEx struct {
		dwLength                uint32
		dwMemoryLoad            uint32
		ullTotalPhys            uint64
		ullAvailPhys            uint64
		ullTotalPageFile        uint64
		ullAvailPageFile        uint64
		ullTotalVirtual         uint64
		ullAvailVirtual         uint64
		ullAvailExtendedVirtual uint64
	}

	var memStatus memoryStatusEx
	memStatus.dwLength = uint32(unsafe.Sizeof(memStatus))

	ret, _, _ := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&memStatus)))
	if ret == 0 {
		return 0
	}
	return int(memStatus.ullTotalPhys / (1024 * 1024))
}

// generateMachineID creates a unique machine identifier for license binding.
// On Windows, uses the MachineGuid from the registry.
func generateMachineID() string {
	cmd := exec.Command("reg", "query",
		`HKLM\SOFTWARE\Microsoft\Cryptography`,
		"/v", "MachineGuid")
	out, err := cmd.Output()
	if err == nil && len(out) > 0 {
		return fmt.Sprintf("win-%s", string(out))
	}
	// Fallback: random ID
	b := make([]byte, 16)
	rand.Read(b)
	return fmt.Sprintf("win-%s", hex.EncodeToString(b)[:16])
}
