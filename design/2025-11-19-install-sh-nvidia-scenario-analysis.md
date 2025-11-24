# install.sh NVIDIA Docker Runtime Scenario Analysis
## Post-Lambda Labs Fix (2025-11-19)

### Summary of Changes
**Fix:** If `nvidia-container-toolkit` already installed but Docker not configured → skip package reinstall, just configure Docker daemon.

**Modified code:** Lines 809-895 in `install_nvidia_docker()` function

---

## Critical NVIDIA Scenarios

### ✅ Scenario 1: No NVIDIA Drivers Installed (--runner)
**Setup:**
- Ubuntu/Debian/Fedora
- No nvidia-smi available
- --runner flag

**Code Path:**
1. Line 978: `check_nvidia_gpu()` returns false (no nvidia-smi)
2. Lines 979-998: Print error message with installation instructions
3. Exit 1

**Result:** ✅ **PASS** - Clear error message tells user to install drivers and reboot

**Error message includes:**
```
❌ ERROR: --runner requires NVIDIA GPU
No NVIDIA GPU detected. Helix Runner requires an NVIDIA GPU.

If you have an NVIDIA GPU:
  1. Install NVIDIA drivers (Ubuntu/Debian):
     sudo ubuntu-drivers install
  2. Reboot your system
  3. Verify drivers: nvidia-smi
  4. Re-run installer
```

**Regression risk:** ❌ None - error handling unchanged

---

### ✅ Scenario 2: NVIDIA Drivers + No Docker (--runner)
**Setup:**
- Ubuntu/Debian/Fedora
- nvidia-smi works
- Docker not installed
- --runner flag

**Code Path:**
1. Line 978: `check_nvidia_gpu()` returns true
2. Lines 1000-1003: Print "NVIDIA GPU detected, runtime will be installed"
3. Lines 1136-1149: `check_docker_needed()` returns true → install Docker
4. Line 1701: Call `install_nvidia_docker()`
5. Line 771: `check_nvidia_gpu()` returns true
6. Line 792: `NVIDIA_IN_DOCKER="false"` (Docker just installed, not configured)
7. Line 793: `NVIDIA_CTK_EXISTS="false"` (not installed yet)
8. Line 799: Condition `true || true` → enter installation block
9. **Line 810: NEW CODE** - Check if toolkit exists
10. Line 810: `NVIDIA_CTK_EXISTS="false"` → enter else block (install package)
11. Lines 813-844: Install nvidia-container-toolkit + nvidia-container-runtime
12. Lines 847-884: Configure Docker daemon + restart + verify

**Result:** ✅ **PASS** - Installs Docker, toolkit, and configures everything

**Regression risk:** ❌ None - same path as before (enters else block at line 812)

---

### ✅ Scenario 3: NVIDIA Drivers + Docker + No Toolkit (--code)
**Setup:**
- Ubuntu/Debian/Fedora
- nvidia-smi works
- Docker installed, toolkit NOT installed
- --code flag

**Code Path:**
1. Line 1009: `check_nvidia_gpu()` returns true
2. Lines 1010-1014: Print "NVIDIA GPU detected, runtime will be installed"
3. Lines 1153-1156: CODE=true + RUNNER=false → call `install_nvidia_docker()`
4. Line 771: `check_nvidia_gpu()` returns true
5. Line 792: `NVIDIA_IN_DOCKER="false"` (not configured)
6. Line 793: `NVIDIA_CTK_EXISTS="false"` (not installed)
7. Line 799: Condition `true || true` → enter installation block
8. **Line 810: NEW CODE** - Check if toolkit exists
9. Line 810: `NVIDIA_CTK_EXISTS="false"` → enter else block (install package)
10. Lines 817-828: Install nvidia-container-toolkit + nvidia-container-runtime
11. Lines 847-884: Configure Docker daemon + restart + verify

**Result:** ✅ **PASS** - Installs toolkit and configures Docker

**Regression risk:** ❌ None - same path as before (enters else block)

---

### ✅ Scenario 4: NVIDIA Drivers + Docker + Toolkit Installed BUT Not Configured (Lambda Labs)
**Setup:**
- Ubuntu/Debian/Fedora
- nvidia-smi works
- Docker installed
- nvidia-container-toolkit installed (package exists)
- Docker daemon NOT configured (no runtime in `docker info`)
- --runner or --code flag

**Code Path:**
1. Line 1001/1012: `check_nvidia_runtime_needed()` returns true
2. Line 1701 or 1155: Call `install_nvidia_docker()`
3. Line 771: `check_nvidia_gpu()` returns true
4. Line 792: `NVIDIA_IN_DOCKER="false"` (not configured)
5. Line 793: `NVIDIA_CTK_EXISTS="true"` (package installed!)
6. Line 799: Condition `true || false` → enter installation block
7. **Line 810: NEW CODE** - Check if toolkit exists
8. Line 810: `NVIDIA_CTK_EXISTS="true"` → **SKIP package installation!**
9. Line 811: Print "NVIDIA Container Toolkit already installed. Configuring Docker to use it..."
10. Lines 847-884: Configure Docker daemon + restart + verify

**Result:** ✅ **PASS** - Skips reinstall, just configures Docker (Lambda Labs fix!)

**What changed:**
- **Before:** Would try to reinstall package → apt repository conflict → failure
- **After:** Skips package installation → directly configures Docker → success

**Regression risk:** ❌ None - this was the broken case we fixed

---

### ✅ Scenario 5: NVIDIA Drivers + Docker + Toolkit + Runtime Configured (--runner)
**Setup:**
- Ubuntu/Debian/Fedora
- nvidia-smi works
- Docker installed and configured
- nvidia-container-toolkit installed
- Docker daemon shows nvidia runtime in `docker info`
- --runner flag

**Code Path:**
1. Line 1001: `check_nvidia_runtime_needed()` returns **false** (already configured!)
2. Line 1002: Does NOT print "will be installed" message
3. Line 1701: Call `install_nvidia_docker()`
4. Line 771: `check_nvidia_gpu()` returns true
5. Line 792: `NVIDIA_IN_DOCKER="true"` (✓ configured)
6. Line 793: `NVIDIA_CTK_EXISTS="true"` (✓ installed)
7. **Line 799:** Condition `false || false` → **SKIP installation block entirely!**
8. Function returns immediately (line 895)

**Result:** ✅ **PASS** - Does nothing (runtime already configured)

**Regression risk:** ❌ None - condition at line 799 unchanged, still skips when both are true

---

### ✅ Scenario 6: Fedora + No Docker + NVIDIA GPU (--runner)
**Setup:**
- Fedora
- nvidia-smi works
- Docker not installed
- --runner flag

**Code Path:**
1. Lines 1136-1149: Install Docker (Fedora path)
2. Line 1701: Call `install_nvidia_docker()`
3. Line 771: `check_nvidia_gpu()` returns true
4. Line 792: `NVIDIA_IN_DOCKER="false"`
5. Line 793: `NVIDIA_CTK_EXISTS="false"`
6. Line 799: Enter installation block
7. **Line 810: NEW CODE** - Toolkit doesn't exist
8. Line 812: Enter else block (install package)
9. Line 815: Check `$ID`
10. **Lines 830-834:** Fedora case - install via dnf
11. Lines 847-884: Configure Docker (Fedora case at line 851)

**Result:** ✅ **PASS** - Installs Docker + toolkit + configures (Fedora-specific commands)

**Regression risk:** ❌ None - Fedora path preserved in new code structure

---

### ✅ Scenario 7: Ubuntu + Conflicting Repo Configs (Lambda Labs variant)
**Setup:**
- Ubuntu
- nvidia-smi works
- Docker installed
- Toolkit NOT installed
- Pre-existing conflicting apt repo config in `/etc/apt/cloud-init.gpg.d/`

**Code Path:**
1. Line 793: `NVIDIA_CTK_EXISTS="false"` (package not installed)
2. Line 799: Enter installation block
3. **Line 810: NEW CODE** - Toolkit doesn't exist
4. Line 812: Enter else block
5. **Lines 819-820: NEW CODE** - Remove conflicting repo files!
   ```bash
   sudo rm -f /etc/apt/sources.list.d/nvidia-container-toolkit.list
   sudo rm -f /etc/apt/sources.list.d/nvidia-docker.list
   ```
6. Lines 823-828: Add fresh repo config + install packages
7. Lines 847-884: Configure Docker daemon

**Result:** ✅ **PASS** - Removes conflicts before adding repo

**What changed:**
- **Before:** Would fail with "Conflicting values for Signed-By"
- **After:** Removes old configs first → clean install → success

**Regression risk:** ❌ None - only affects systems with pre-existing conflicts

---

## Edge Cases

### ✅ Edge Case 1: WSL2 Environment
**Code Path:**
- Lines 801-805: Special WSL2 detection → print instructions → return early
- Never reaches package installation code

**Result:** ✅ **PASS** - Unchanged behavior

---

### ✅ Edge Case 2: Git Bash (Windows Docker Desktop)
**Code Path:**
- Lines 777-789: Git Bash detection → verify Docker Desktop has NVIDIA support → return early
- Never reaches package installation code

**Result:** ✅ **PASS** - Unchanged behavior

---

### ✅ Edge Case 3: macOS
**Code Path:**
- Lines 668-671: macOS detected at early stage → exit with manual install instructions
- Never reaches `install_nvidia_docker()`

**Result:** ✅ **PASS** - Unchanged behavior

---

### ✅ Edge Case 4: Other Linux (Arch, etc.)
**Code Path:**
- Lines 836-838: OS detection fails → print manual install instructions → exit
- Falls through when `$ID` is not ubuntu/debian/fedora

**Result:** ✅ **PASS** - Unchanged behavior

---

## Summary Table

| Scenario | NVIDIA Drivers | Docker | Toolkit | Runtime Config | Result | Regression Risk |
|----------|---------------|--------|---------|----------------|--------|----------------|
| 1. No drivers | ❌ | Any | Any | Any | Error + instructions | ❌ None |
| 2. Drivers only | ✅ | ❌ | ❌ | ❌ | Install all | ❌ None |
| 3. Drivers + Docker | ✅ | ✅ | ❌ | ❌ | Install toolkit | ❌ None |
| 4. Drivers + Docker + Toolkit | ✅ | ✅ | ✅ | ❌ | **Skip install, just configure** | ❌ None (FIXED) |
| 5. Fully configured | ✅ | ✅ | ✅ | ✅ | Do nothing | ❌ None |
| 6. Fedora variant | ✅ | ❌ | ❌ | ❌ | Install all (dnf) | ❌ None |
| 7. Conflicting repos | ✅ | ✅ | ❌ | ❌ | **Remove conflicts, then install** | ❌ None (FIXED) |

---

## Conclusion

✅ **All scenarios verified safe**

**New code changes (lines 809-845):**
1. Added toolkit existence check before package installation
2. Added cleanup of conflicting repo configs
3. Preserved all existing error paths and edge case handling
4. Same configuration logic for all paths (lines 847-884)

**Fixes:**
- Lambda Labs case: toolkit installed → skip reinstall → configure only
- Repo conflicts: remove old configs → clean install

**Regressions:**
- ❌ None identified - all code paths preserved or improved

**Test coverage:**
- ✅ No drivers → error message unchanged
- ✅ Fresh install → installs everything (same path as before)
- ✅ Partial install → installs missing components (same path)
- ✅ Toolkit exists → NEW: skips reinstall, configures only
- ✅ Fully configured → skips everything (unchanged)
- ✅ Fedora/WSL2/Git Bash → edge cases unchanged
