# Implementation Tasks

- [x] Revert `ensure_go()` changes from `helix/stack` script

- [~] Add Go installation to `Dockerfile.ubuntu-helix`
  - Copy `go.mod` to extract version
  - Download and install Go tarball to `/usr/local/go`
  - Add `/usr/local/go/bin` to PATH via ENV

- [ ] Add Go installation to `Dockerfile.sway-helix`
  - Same as ubuntu-helix

- [ ] Test by rebuilding desktop image and verifying `go version` works inside container