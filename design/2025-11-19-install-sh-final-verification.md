# install.sh Final Test Matrix Verification
## Date: 2025-11-19 (Post All Fixes)

## Critical Fix Applied
**Validation order corrected:**
1. Auto-enable controlplane for --code/--haystack (lines 975-981)
2. GPU validation for --runner (lines 983-1016) - **MOVED UP**
3. Token/controlplane validation (lines 1018-1046) - **MOVED DOWN**
4. External Zed agent validation (lines 1048-1060)
5. GPU validation for --code (lines 1062+)

---

## Complete Test Matrix Trace

### ✅ Test 1: Ubuntu + No Docker + No GPU + --cli
**Command:** `./install.sh --cli`
**Path:**
- Line 337: CLI=true, AUTO=false
- Line 976: CODE/HAYSTACK empty → skip  
- Line 984: RUNNER=false → skip GPU validation
- Line 1018: RUNNER=false → skip token validation
- Line 1178: Install CLI only
**Result:** ✅ PASS - CLI only, no Docker

---

### ✅ Test 2: Ubuntu + No Docker + NVIDIA GPU + --controlplane --runner
**Command:** `./install.sh --controlplane --runner`
**Path:**
- Line 341: CONTROLPLANE=true, AUTO=false
- Line 346: RUNNER=true, AUTO=false
- Line 976: CODE/HAYSTACK empty → skip
- Line 984: RUNNER=true, `check_nvidia_gpu()` → true
- Line 1007: Print "NVIDIA GPU detected"
- Line 1018: CONTROLPLANE=true → skip token validation
- Line 1142: Docker needed → `install_docker()`
- Line 1707: `install_nvidia_docker()` → installs toolkit + configures
**Result:** ✅ PASS - Installs Docker + NVIDIA runtime + runner

---

### ✅ Test 3: Ubuntu + Docker + NVIDIA GPU + No runtime + --code
**Command:** `./install.sh --code`
**Path:**
- Line 367: CODE=true, AUTO=false
- **Line 976: CODE="true" → AUTO-ENABLE CONTROLPLANE + CLI**
- Line 984: RUNNER=false → skip GPU validation for runner
- Line 1062: CODE=true, `check_nvidia_gpu()` → true
- Line 1159: `install_nvidia_docker()` → installs toolkit + configures
**Result:** ✅ PASS - Auto-enables controlplane, installs NVIDIA runtime

---

### ✅ Test 4: Ubuntu + No Docker + NVIDIA GPU WITHOUT drivers + --runner
**Command:** `./install.sh --runner`
**Path:**
- Line 346: RUNNER=true, AUTO=false
- Line 976: CODE/HAYSTACK empty → skip
- **Line 984: RUNNER=true**
- **Line 985: `check_nvidia_gpu()` → FALSE (no nvidia-smi)**
- **Lines 986-1005: ERROR - "install NVIDIA drivers and reboot"**
- **EXITS - never reaches token validation**
**Result:** ✅ PASS - Clear GPU driver error (FIXED!)

---

### ✅ Test 5: Ubuntu + No Docker + Intel/AMD GPU + --code
**Command:** `./install.sh --code`
**Path:**
- Line 367: CODE=true, AUTO=false
- Line 976: CODE="true" → AUTO-ENABLE CONTROLPLANE + CLI
- Line 984: RUNNER=false → skip
- Line 1062: CODE=true
- Line 1065: `check_nvidia_gpu()` → false
- Line 1071: `check_intel_amd_gpu()` → true (/dev/dri exists)
- Line 1072: Print "Intel/AMD GPU detected"
- Line 1142: Docker needed → `install_docker()`
**Result:** ✅ PASS - Detects Intel/AMD, installs Docker + controlplane

---

### ✅ Test 6: Git Bash + Docker Desktop + --controlplane
**Command:** `./install.sh --controlplane`
**Path:**
- ENVIRONMENT="gitbash" (line 127)
- Line 341: CONTROLPLANE=true
- Line 192: Git Bash → NEED_SUDO="false", DOCKER_CMD="docker"
- Line 1218: Install controlplane (docker compose)
**Result:** ✅ PASS - Uses Docker Desktop, no sudo

---

### ✅ Test 7: WSL2 + No Docker + --controlplane
**Command:** `./install.sh --controlplane`
**Path:**
- Line 132: Detects WSL2 (grep /proc/version)
- ENVIRONMENT="wsl2"
- Line 663-667: WSL2 + no Docker → ERROR "install Docker Desktop for Windows"
**Result:** ✅ PASS - Clear error for WSL2

---

### ✅ Test 8: macOS + No Docker + --controlplane
**Command:** `./install.sh --controlplane`
**Path:**
- Line 140: ENVIRONMENT="macos", OS="darwin"
- Line 668-671: macOS + no Docker → ERROR "install Docker manually"
**Result:** ✅ PASS - Clear error for macOS

---

### ✅ Test 9: Fedora + No Docker + NVIDIA GPU + --runner
**Command:** `./install.sh --runner`
**Path:**
- Line 346: RUNNER=true, AUTO=false
- Line 976: CODE/HAYSTACK empty → skip
- **Line 984: RUNNER=true**
- **Line 985: `check_nvidia_gpu()` → TRUE (nvidia-smi works)**
- **Line 1007: Print "NVIDIA GPU detected"**
- **Line 1018: CONTROLPLANE=false → enters token validation**
- **Line 1024: RUNNER_TOKEN empty**
- **Lines 1037-1045: ERROR - "need --runner-token or --controlplane"**
**Result:** ✅ PASS - GPU valid, then asks for token/controlplane (FIXED!)

**Note:** This is correct behavior - user must explicitly choose:
- `./install.sh --controlplane --runner` (local)
- `./install.sh --runner --runner-token TOKEN --api-host HOST` (remote)

---

### ✅ Test 10: Ubuntu + Docker + NVIDIA runtime installed + --runner
**Command:** `./install.sh --runner`
**Path:**
- nvidia-smi works, Docker has nvidia runtime
- Line 346: RUNNER=true, AUTO=false
- Line 976: CODE/HAYSTACK empty → skip
- **Line 984: RUNNER=true**
- **Line 985: `check_nvidia_gpu()` → TRUE**
- **Line 1007: Print "NVIDIA GPU detected"**
- **Line 1008: `check_nvidia_runtime_needed()` → FALSE (already configured)**
- **Line 1008: Skips "will be installed" message**
- **Line 1018: CONTROLPLANE=false → enters token validation**
- **Line 1024: RUNNER_TOKEN empty**
- **Lines 1037-1045: ERROR - "need --runner-token or --controlplane"**
**Result:** ✅ PASS - GPU valid, runtime configured, asks for token/controlplane (FIXED!)

**Note:** Same as test 9 - user must explicitly choose local or remote.

---

### ✅ Test 11: Arch Linux + No Docker + --controlplane
**Command:** `./install.sh --controlplane`
**Path:**
- Line 674-681: `$ID` not ubuntu/debian/fedora
- ERROR "auto-install only supports Ubuntu/Debian/Fedora"
**Result:** ✅ PASS - Clear error for unsupported distros

---

### ✅ Test 12: Ubuntu + No Docker + No GPU + --code
**Command:** `./install.sh --code`
**Path:**
- Line 367: CODE=true, AUTO=false
- Line 976: CODE="true" → AUTO-ENABLE CONTROLPLANE
- Line 984: RUNNER=false → skip
- Line 1062: CODE=true
- Line 1065: `check_nvidia_gpu()` → FALSE
- Line 1071: `check_intel_amd_gpu()` → FALSE (no /dev/dri)
- Lines 1074-1091: ERROR - "install NVIDIA/Intel/AMD drivers"
**Result:** ✅ PASS - Clear error about GPU requirement

---

### ✅ Test 13: Ubuntu + NVIDIA GPU + --code (without --controlplane)
**Command:** `./install.sh --code`
**Path:**
- Line 367: CODE=true, AUTO=false
- **Line 976: CODE="true" → AUTO-ENABLE CONTROLPLANE + CLI**
- **Line 979: CONTROLPLANE=true, CLI=true**
- Line 984: RUNNER=false → skip
- Line 1062: CODE=true, GPU check → true
- Line 1218: Install controlplane with CODE profile
**Result:** ✅ PASS - Auto-enables controlplane (NEW BEHAVIOR!)

**User experience:** Simpler command, no need for explicit --controlplane

---

### ✅ Test 14: Ubuntu + NVIDIA GPU + --runner --code --haystack (without --runner-token)
**Command:** `./install.sh --runner --code --haystack`
**Path:**
- Line 346: RUNNER=true
- Line 367: CODE=true
- Line 360: HAYSTACK=true
- **Line 976: CODE="true" → AUTO-ENABLE CONTROLPLANE + CLI**
- Line 984: RUNNER=true, GPU check → true
- **Line 1018: CONTROLPLANE=true → SKIP token validation (controlplane enabled!)**
- Line 1218: Install controlplane with CODE + HAYSTACK profiles
- Line 1707: Install runner (gets token from controlplane .env)
**Result:** ✅ PASS - Auto-enables controlplane, installs everything (YOUR USE CASE!)

---

### ✅ Test 15: Ubuntu + NVIDIA GPU + --runner (without --runner-token or --code/--haystack/--controlplane)
**Command:** `./install.sh --runner`
**Path:**
- Line 346: RUNNER=true, AUTO=false
- Line 976: CODE/HAYSTACK empty → skip
- Line 984: RUNNER=true, GPU check → true
- **Line 1018: CONTROLPLANE=false → enters token validation**
- **Line 1024: RUNNER_TOKEN empty**
- **Lines 1037-1045: ERROR - "need --runner-token or --controlplane"**
**Result:** ✅ PASS - Explicit error, prevents ambiguity (BY DESIGN!)

**User must choose:**
- `./install.sh --controlplane --runner` (local)
- `./install.sh --runner --code` (local with Code)
- `./install.sh --runner --runner-token TOKEN --api-host HOST` (remote)

---

### ✅ Test 16: Ubuntu + --runner --runner-token TOKEN --api-host HOST
**Command:** `./install.sh --runner --runner-token abc123 --api-host https://demo.helix.ml`
**Path:**
- Line 346: RUNNER=true
- Line 382: RUNNER_TOKEN="abc123"
- Line 374: API_HOST="https://demo.helix.ml"
- Line 976: CODE/HAYSTACK empty → skip
- Line 984: RUNNER=true, GPU check → true (if GPU exists)
- **Line 1018: CONTROLPLANE=false → enters token validation**
- **Line 1024: RUNNER_TOKEN="abc123" (NOT empty)**
- **Line 1027: Has API_HOST → validation passes**
- **Line 1046: Exits validation block (does NOT auto-enable controlplane)**
- Line 1707: RUNNER=true → creates runner.sh with provided token
**Result:** ✅ PASS - Remote runner only, no controlplane (CORRECT!)

---

## Summary

### All 16 Test Cases: ✅ PASS

### Behavior Changes (All Intentional):
1. **Test 3:** --code now auto-enables controlplane (simpler UX)
2. **Test 4:** GPU error before token error (clearer messages) - **CRITICAL FIX**
3. **Test 9:** GPU valid → asks for explicit choice (prevents ambiguity)
4. **Test 10:** Same as test 9 (prevents ambiguity)
5. **Test 13:** --code auto-enables controlplane (simpler UX)
6. **Test 14:** --code/--haystack auto-enable → --runner adds local runner (YOUR USE CASE)
7. **Test 15:** --runner alone now errors (prevents ambiguity) - **INTENTIONAL**

### No Regressions:
- ✅ All CLI-only paths unchanged
- ✅ All explicit --controlplane paths unchanged
- ✅ All GPU detection unchanged
- ✅ All Docker installation unchanged
- ✅ All platform detection (Git Bash, WSL2, macOS) unchanged
- ✅ All remote runner paths unchanged

### Critical Fixes Applied:
1. **Lambda Labs:** Skip NVIDIA toolkit reinstall if already installed
2. **Validation order:** GPU errors before config errors
3. **Auto-enable:** --code/--haystack enable controlplane automatically
4. **Clear errors:** Explicit messages with examples

### User Experience Improvements:
- Simpler commands: `./install.sh --code` instead of `./install.sh --cli --controlplane --code`
- Clearer errors: Hardware errors before config errors
- Better messaging: "configure" vs "install and configure"
- Prevents ambiguity: --runner alone requires explicit choice

