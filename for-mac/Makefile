# Helix Desktop build targets
#
# Usage:
#   make build       - Quick rebuild (Wails + re-sign)
#   make build-full  - Full rebuild with QEMU + frameworks packaging
#   make rebuild-qemu - Rebuild QEMU from source + install into dev-qemu (no Wails rebuild)
#   make sign        - Re-sign existing app bundle + dev QEMU
#   make dev         - Start Wails dev mode (hot reload)

APP_BUNDLE := build/bin/Helix.app
MACOS_DIR := $(APP_BUNDLE)/Contents/MacOS
FRAMEWORKS_DIR := $(APP_BUNDLE)/Contents/Frameworks
DEV_QEMU_DIR := build/dev-qemu
APP_ENTITLEMENTS := build/darwin/entitlements-app.plist
QEMU_ENTITLEMENTS := build/darwin/entitlements.plist
SIGNING_ENV := .env.signing
SIGN_SCRIPT := scripts/sign-app.sh

QEMU_SRC ?= $(HOME)/pm/qemu-utm
SYSROOT ?= $(HOME)/pm/UTM/sysroot-macOS-arm64

.PHONY: build build-full rebuild-qemu sign sign-dev-qemu dev

# Quick rebuild: wails build + re-sign
build:
	wails build
	@$(MAKE) sign

# Full rebuild: QEMU + frameworks + everything
build-full:
	./scripts/build-helix-app.sh
	@$(MAKE) sign-dev-qemu

# Rebuild QEMU from source and install into dev-qemu.
# Use this after modifying QEMU source (~/pm/qemu-utm).
# Requires: VM stopped (kill QEMU process first).
rebuild-qemu:
	@echo "[rebuild-qemu] Building QEMU from source..."
	./qemu-helix/build-qemu-standalone.sh
	@echo "[rebuild-qemu] Copying QEMU to app bundle..."
	@mkdir -p $(MACOS_DIR)
	@cp -f $(SYSROOT)/bin/qemu-system-aarch64 $(MACOS_DIR)/qemu-system-aarch64
	@cp -f $(SYSROOT)/lib/libqemu-aarch64-softmmu.dylib $(MACOS_DIR)/libqemu-aarch64-softmmu.dylib
	@# Fix dylib paths (same as build-helix-app.sh Step 6)
	@install_name_tool -id "@rpath/libqemu-aarch64-softmmu.dylib" \
		$(MACOS_DIR)/libqemu-aarch64-softmmu.dylib 2>/dev/null || true
	@install_name_tool -change "$(SYSROOT)/lib/libqemu-aarch64-softmmu.dylib" \
		"@executable_path/libqemu-aarch64-softmmu.dylib" \
		$(MACOS_DIR)/qemu-system-aarch64 2>/dev/null || true
	@install_name_tool -add_rpath "@executable_path/../Frameworks" \
		$(MACOS_DIR)/qemu-system-aarch64 2>/dev/null || true
	@install_name_tool -add_rpath "@executable_path/../Frameworks" \
		$(MACOS_DIR)/libqemu-aarch64-softmmu.dylib 2>/dev/null || true
	@echo "[rebuild-qemu] Creating dev QEMU..."
	@$(MAKE) sign-dev-qemu
	@echo "[rebuild-qemu] Done. Restart the VM to use the new QEMU."

# Re-sign the app bundle + create standalone dev QEMU
sign: sign-dev-qemu
	@echo "[sign] Signing app bundle..."
	@for fw_dir in $(FRAMEWORKS_DIR)/*.framework; do \
		codesign --force --sign - --timestamp=none "$$fw_dir" 2>/dev/null || true; \
	done
	@codesign --force --sign - --timestamp=none \
		--entitlements $(APP_ENTITLEMENTS) \
		--deep $(APP_BUNDLE) 2>/dev/null || true
	@codesign --force --sign - --timestamp=none \
		--entitlements $(QEMU_ENTITLEMENTS) \
		"$(MACOS_DIR)/libqemu-aarch64-softmmu.dylib" 2>/dev/null || true
	@codesign --force --sign - --timestamp=none \
		--entitlements $(QEMU_ENTITLEMENTS) \
		"$(MACOS_DIR)/qemu-system-aarch64" 2>/dev/null || true
	@if [ -f "$(SIGNING_ENV)" ] && [ -f "$(SIGN_SCRIPT)" ]; then \
		echo "[sign] Re-signing with Developer ID..."; \
		bash $(SIGN_SCRIPT); \
	else \
		echo "[sign] Ad-hoc signing complete (no .env.signing found)"; \
	fi
	@codesign --verify --deep --strict $(APP_BUNDLE) 2>&1 && echo "[sign] Signature verified OK"

# Create standalone signed QEMU for dev mode.
# Lives outside the app bundle so wails dev rebuilding the main binary
# doesn't invalidate QEMU's code signature.
sign-dev-qemu:
	@if [ ! -f "$(MACOS_DIR)/qemu-system-aarch64" ]; then \
		echo "[dev-qemu] No QEMU in app bundle — run 'make build-full' first"; \
		exit 0; \
	fi
	@echo "[dev-qemu] Creating standalone dev QEMU..."
	@mkdir -p $(DEV_QEMU_DIR)
	@cp -f $(MACOS_DIR)/qemu-system-aarch64 $(DEV_QEMU_DIR)/
	@cp -f $(MACOS_DIR)/libqemu-aarch64-softmmu.dylib $(DEV_QEMU_DIR)/
	@# Fix dylib load path to point to the dev directory
	@install_name_tool -change "@executable_path/libqemu-aarch64-softmmu.dylib" \
		"@executable_path/libqemu-aarch64-softmmu.dylib" \
		$(DEV_QEMU_DIR)/qemu-system-aarch64 2>/dev/null || true
	@# Add rpath for frameworks (still in the app bundle)
	@install_name_tool -delete_rpath "@executable_path/../Frameworks" \
		$(DEV_QEMU_DIR)/qemu-system-aarch64 2>/dev/null || true
	@install_name_tool -add_rpath "$(CURDIR)/$(FRAMEWORKS_DIR)" \
		$(DEV_QEMU_DIR)/qemu-system-aarch64 2>/dev/null || true
	@install_name_tool -delete_rpath "@executable_path/../Frameworks" \
		$(DEV_QEMU_DIR)/libqemu-aarch64-softmmu.dylib 2>/dev/null || true
	@install_name_tool -add_rpath "$(CURDIR)/$(FRAMEWORKS_DIR)" \
		$(DEV_QEMU_DIR)/libqemu-aarch64-softmmu.dylib 2>/dev/null || true
	@# Sign with entitlements + hardened runtime (Developer ID if available, ad-hoc otherwise)
	@if [ -f "$(SIGNING_ENV)" ]; then \
		. $(SIGNING_ENV) && \
		codesign --force --sign "$$APPLE_SIGNING_IDENTITY" --timestamp --options runtime \
			--entitlements $(QEMU_ENTITLEMENTS) \
			$(DEV_QEMU_DIR)/libqemu-aarch64-softmmu.dylib && \
		codesign --force --sign "$$APPLE_SIGNING_IDENTITY" --timestamp --options runtime \
			--entitlements $(QEMU_ENTITLEMENTS) \
			$(DEV_QEMU_DIR)/qemu-system-aarch64 && \
		echo "[dev-qemu] Signed with Developer ID (hardened runtime)"; \
	else \
		codesign --force --sign - --timestamp=none --options runtime \
			--entitlements $(QEMU_ENTITLEMENTS) \
			$(DEV_QEMU_DIR)/libqemu-aarch64-softmmu.dylib && \
		codesign --force --sign - --timestamp=none --options runtime \
			--entitlements $(QEMU_ENTITLEMENTS) \
			$(DEV_QEMU_DIR)/qemu-system-aarch64 && \
		echo "[dev-qemu] Ad-hoc signed (hardened runtime)"; \
	fi
	@codesign -vvv $(DEV_QEMU_DIR)/qemu-system-aarch64 2>&1 && echo "[dev-qemu] Signature verified OK"

# Wails dev mode with hot reload
dev:
	@mkdir -p build/bin/Helix.app/Contents/Resources/vm
	@cp -f vm-manifest.json build/bin/Helix.app/Contents/Resources/vm/vm-manifest.json 2>/dev/null || true
	wails dev
