# install.sh Test Matrix Verification (Post-Changes)
## Date: 2025-11-19

## Changes Made
1. NVIDIA Docker runtime: Skip package reinstall if toolkit already installed (lines 809-895)
2. Auto-enable controlplane for --code/--haystack (lines 968-974)
3. Runner validation: Require --runner-token OR --controlplane OR controlplane features (lines 983-1012)
4. Improved messaging for NVIDIA runtime (configure vs install)

---

## Test Case 1: Ubuntu + No Docker + No GPU + --cli
**Command:** `./install.sh --cli`

**Initial State:**
- AUTO=true, CLI=false, CONTROLPLANE=false, RUNNER=false
- CODE="", HAYSTACK=""

**Code Path:**
1. Line 337: `--cli` → CLI=true, AUTO=false
2. Line 968-974: CODE/HAYSTACK empty → skip auto-enable controlplane
3. Line 983: RUNNER=false → skip runner validation
4. Line 659-662: CLI=true, CONTROLPLANE=false, RUNNER=false → NEED_SUDO="false"
5. Line 1178-1190: CLI=true → Install CLI binary
6. Line 1218: CONTROLPLANE=false → skip controlplane installation
7. Line 1706: RUNNER=false → skip runner installation

**Result:** ✅ **PASS** - Installs CLI only, no Docker needed

**Regression Risk:** ❌ None - CLI-only path unchanged

---

## Test Case 2: Ubuntu + No Docker + NVIDIA GPU + --controlplane --runner
**Command:** `./install.sh --controlplane --runner`

**Initial State:**
- nvidia-smi works, Docker not installed
- AUTO=true, CLI=false, CONTROLPLANE=false, RUNNER=false

**Code Path:**
1. Line 341: `--controlplane` → CONTROLPLANE=true, AUTO=false
2. Line 346: `--runner` → RUNNER=true, AUTO=false
3. Line 968-974: CODE/HAYSTACK empty → skip auto-enable
4. Line 983: RUNNER=true, CONTROLPLANE=true → skip runner validation (CONTROLPLANE already true)
5. Line 1019-1020: `check_nvidia_gpu()` returns true (nvidia-smi works)
6. Line 1023: `check_nvidia_runtime_needed()` returns true (no Docker)
7. Line 1025-1029: toolkit doesn't exist → "will be installed and configured"
8. Line 1142-1155: Docker needed → `install_docker()` called
9. Line 1707: `install_nvidia_docker()` called
10. Line 793: NVIDIA_CTK_EXISTS="false" (fresh install)
11. Line 799: Enter installation block
12. Line 810: NVIDIA_CTK_EXISTS="false" → enter else block (install package)
13. Lines 817-828: Install nvidia-container-toolkit + configure Docker
14. Line 1718: Create runner.sh with generated RUNNER_TOKEN from .env

**Result:** ✅ **PASS** - Installs Docker + NVIDIA runtime + creates runner

**Regression Risk:** ❌ None - Fresh install path unchanged (enters else block)

---

## Test Case 3: Ubuntu + Docker + NVIDIA GPU + No NVIDIA runtime + --code
**Command:** `./install.sh --code`

**Initial State:**
- nvidia-smi works, Docker installed, toolkit NOT installed
- AUTO=true, CLI=false, CONTROLPLANE=false, CODE=""

**Code Path:**
1. Line 367: `--code` → CODE=true, AUTO=false
2. **Line 969: CODE="true" → enters auto-enable block**
3. **Line 972-973: CONTROLPLANE=true, CLI=true**
4. Line 983: RUNNER=false → skip runner validation
5. Line 1025: `check_nvidia_gpu()` returns true
6. Line 1027: `check_nvidia_runtime_needed()` returns true (Docker exists but not configured)
7. Line 1029: toolkit doesn't exist → "will be installed and configured"
8. Line 1159-1162: CODE=true, RUNNER=false → `install_nvidia_docker()` called
9. Line 793: NVIDIA_CTK_EXISTS="false"
10. Line 799: Enter installation block
11. Line 810: Enter else block (install package)
12. Lines 817-828: Install toolkit + configure Docker
13. Line 1344: CODE=true → COMPOSE_PROFILES includes "code"

**Result:** ✅ **PASS** - Auto-enables controlplane, installs NVIDIA runtime, enables code profile

**Regression Risk:** ⚠️ **BEHAVIOR CHANGE** - Now auto-enables controlplane (was documented as needing it)

**Verification:** Test case 3 description says "Enable code profile" - this implies controlplane must be enabled. Our change makes this automatic, which is correct.

---

## Test Case 4: Ubuntu + No Docker + NVIDIA GPU without drivers + --runner
**Command:** `./install.sh --runner`

**Initial State:**
- No nvidia-smi, NVIDIA hardware present
- AUTO=true, CONTROLPLANE=false, RUNNER=false

**Code Path:**
1. Line 346: `--runner` → RUNNER=true, AUTO=false
2. Line 968-974: CODE/HAYSTACK empty → skip auto-enable
3. **Line 983: RUNNER=true, CONTROLPLANE=false → enters validation block**
4. **Line 989: RUNNER_TOKEN empty → skip remote runner check**
5. **Line 1001-1010: ERROR - "Error: --runner requires either: ..."**

**WAIT - THIS IS WRONG!** Test case 4 expects GPU validation to happen BEFORE runner token validation.

Let me check the GPU validation order:

