# Validation

Results from running the project on an Apple M5 Pro, macOS 27, Go 1.27, Docker Desktop, kind v1.36. Date: 2026-10-22.

## Phase 0: can it work at all

| Question | Result |
|---|---|
| `vz` boots Linux ARM64 through EFI with a cloud image | Yes. Ubuntu 24.04 reached the login prompt in seconds |
| Disk format | `Virtualization.framework` needs raw. The Ubuntu `.img` is qcow2, so convert with `qemu-img convert -O raw` |
| Signing | Ad-hoc `codesign -s -` with `com.apple.security.virtualization` is enough |
| cloud-init | `hdiutil makehybrid -iso -joliet -default-volume-name cidata` applied user, hostname and SSH key |
| NAT network | VM got `192.168.64.x` by DHCP and SSH from the Mac worked |
| Pod in kind reaches the VM | **Yes.** Ping at 1 to 2 ms and port 22 open from a busybox pod |
| Nested virtualization (plan B: KubeVirt inside a Linux VM) | `vz.IsNestedVirtualizationSupported()` is `true` on M5. KubeVirt inside Lima was **not** tested |

## Phase 1: lifecycle

`hack/e2e.sh` runs this against a real cluster and passes:

1. Create a `SiliconVM`: it reaches `Running` with an IP. SSH from the Mac prints the hostname, `aarch64`, and a root disk of 8.7 GB (10Gi requested, resize applied).
2. A pod in the cluster pings the VM and finds port 22 open.
3. `running: false`: phase `Stopped`, IP cleared, disk kept.
4. `running: true`: the same disk boots again, phase `Running`.
5. Stop `kubesilicond`: the VM goes down and the object says `Stopped`. Start it again: the VM comes back.
6. Delete: the object is gone, the VM is stopped and its directory is removed.

Unit tests cover the DHCP lease parser.

## Phase 2: launcher process and DNS

`hack/e2e.sh` now also checks:

- From a pod: `nslookup e2e.kubesilicon.svc.cluster.local` returns the VM IP, and `nc` to that name on port 22 connects.
- `SIGTERM` to the daemon: SSH to the VM still works, the object stays `Running`, and a new daemon reports the same IP.
- `kill -9` of the daemon: SSH still works, and the new daemon adopts the launcher.
- `running: false` through the adopted launcher: object `Stopped`, IP cleared, EndpointSlice emptied, launcher process gone, disk kept.
- Delete: Service and EndpointSlice are garbage-collected and no launcher is left.

## Bugs the e2e test found

- **Stale DHCP lease.** `/var/db/dhcpd_leases` keeps leases of old VMs. With a fixed hostname, a new VM was reported at the IP of the previous one, and the pod ping failed. A restarted VM also reuses its lease without renewing the expiry, so "lease newer than before boot" does not work either. Fix: a hostname unique per object (`<name>-<uid8>`). The same change removes the collision between VMs with the same name in different namespaces.
- **Check that could not fail.** A first version of the "launcher exited" assertions were written as `cmd && echo`, which never fails the script, and `pgrep -f` also matched the shell running the test. They are now hard checks with a pattern that does not match itself (`[k]ubesilicond`).
- **Cluster context.** The default kubectl context was a different cluster. The daemon now requires `-context`, and every test command passes the context explicitly.

## Not covered

- KubeVirt inside Lima with nested virtualization.
- Bridged networking.
- Colima and OrbStack as the container runtime.
- Daemon crash (not a clean stop): the VMs die with it and the object keeps its last status until the daemon is back.
