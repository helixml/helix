# macOS DMG builder cloud-init OOM

Date: 2026-07-16
Status: root fix validated for provisioning; Resolute signed DMG validation pending

## Drone 3011 evidence

Drone build 3011 was not a release-wide failure:

- Core build and deployment completed successfully.
- The macOS DMG pipeline failed while provisioning its Linux guest.
- The preserved runner `qemu-boot.log` showed `apt-get` reaching 32.27 GB RSS
  and being OOM-killed inside the 32 GB guest.
- The provisioning script waited only for its cloud-init ready marker. Its
  eventual timeout reported that the marker never appeared but hid the actual
  package-install failure.
- The guest image was Ubuntu 25.10, which reached end of life on 2026-07-09.

The failure was therefore in the DMG builder's disposable VM provisioning, not
in the already-successful release or deployment steps.

## Root fix

Switch `for-mac/scripts/provision-vm-light.sh` from the Ubuntu 25.10 current
image to the Ubuntu 26.04 LTS Resolute release image.

The image URL is release-specific and its SHA-256 checksum is pinned. Provision
must verify the downloaded image before creating the guest disk, delete a
mismatched download, and fail immediately. This avoids both an EOL package
source and silent changes to a floating cloud image.

## Diagnostic hardening

When the cloud-init ready marker times out, print the evidence needed to
identify the failed stage:

- `cloud-init status --long`
- `systemctl status cloud-final.service`
- Recent `journalctl` output for `cloud-final.service`
- The tail of `/var/log/cloud-init-output.log`
- The tail of the host-side `qemu-boot.log`

These diagnostics do not replace the timeout. They make the timeout report the
underlying cloud-init, package, service, or guest-kernel failure instead of only
the missing marker.

## Validation results

### Fresh Ubuntu 26.04 provision

A fresh isolated full provision ran from 10:32:16 to 10:35:38 and passed:

- The pinned Ubuntu 26.04 image checksum passed.
- Cloud-init completed in 31 seconds without the prior OOM.
- ZFS 2.4.3 loaded on kernel 7.0.
- Helix API health passed.
- Sandbox Docker daemon readiness passed.
- Guest shutdown, image trim, and compaction passed.
- The full provisioning script completed successfully.

The `helix-ubuntu` inner image pull failed after sandbox dockerd was ready with
a host Docker socket error. The existing script treats that pull as a nonfatal
warning, and the full provision still completed. This was not caused by the
Ubuntu migration. Track it separately only if it reproduces as a user-visible
failure.

### Release pipeline

The original Drone 3012 Questing retry passed completely, including the signed
DMG, and unblocked release 2.11.53. That validates the existing DMG pipeline but
not the new Resolute guest image.

The next tagged pipeline must validate that the signed, notarized, stapled DMG
also completes with the Ubuntu 26.04 Resolute provisioned artifact.
