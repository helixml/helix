# Implementation Tasks

- [x] Revert `ensure_go()` changes from `helix/stack` script

- [x] Add Go installation to `Dockerfile.ubuntu-helix`
  - Copy `go.mod` to extract version
  - Download and install Go tarball to `/usr/local/go`
  - Add `/usr/local/go/bin` to PATH via ENV

- [x] Add Go installation to `Dockerfile.sway-helix`
  - Same as ubuntu-helix

- [ ] Test by rebuilding desktop image and verifying `go version` works inside container
  - Run `./stack build-ubuntu` or `./stack build-sway`
  - Start a session and run `go version` inside the container