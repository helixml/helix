# Helix Desktop build targets
#
# Usage:
#   make build      - Quick rebuild (Wails + re-sign)
#   make build-full - Full rebuild with QEMU + frameworks packaging
#   make sign       - Re-sign existing app bundle
#   make dev        - Start Wails dev mode (hot reload)

APP_BUNDLE := build/bin/Helix.app
MACOS_DIR := $(APP_BUNDLE)/Contents/MacOS
FRAMEWORKS_DIR := $(APP_BUNDLE)/Contents/Frameworks
APP_ENTITLEMENTS := build/darwin/entitlements-app.plist
QEMU_ENTITLEMENTS := build/darwin/entitlements.plist
SIGNING_ENV := .env.signing
SIGN_SCRIPT := scripts/sign-app.sh

.PHONY: build build-full sign dev

# Quick rebuild: wails build + re-sign
build:
	wails build
	@$(MAKE) sign

# Full rebuild: QEMU + frameworks + everything
build-full:
	./scripts/build-helix-app.sh

# Re-sign the app bundle (frameworks + main app + QEMU with entitlements)
sign:
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

# Wails dev mode with hot reload
dev:
	wails dev
