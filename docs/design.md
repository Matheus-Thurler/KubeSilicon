# Design

kubesilicon gives a local Kubernetes cluster on a Mac the KubeVirt workflow (declare a VM, get a VM) without KVM. The VM runs on the macOS host through `Virtualization.framework`. The cluster only stores the desired state.

## Decisions

| Topic | Decision |
|---|---|
| Guests | Linux ARM64. macOS guests are out of scope for now |
| Cluster | A local cluster such as kind, one Mac. The API already carries `nodeName` room for a fleet later |
| API | Own group `kubesilicon.io/v1`, shape similar to KubeVirt. Not a drop-in for `kubevirt.io` |
| Language | Go. [`Code-Hex/vz`](https://github.com/Code-Hex/vz) binds `Virtualization.framework` (cgo) |
| Out of scope for v1 | live migration, hotplug, GPU passthrough, snapshots, macOS guests |

## Mapping to KubeVirt

| KubeVirt | kubesilicon | Note |
|---|---|---|
| virt-api (webhooks) | not needed | Validation is in the CRD schema |
| virt-controller (creates the launcher pod) | not needed | There is no pod: the VM runs on the host |
| virt-handler (reconciles on the node) | `kubesilicond` | One daemon on macOS, informer plus work queue, `client-go` dynamic client |
| virt-launcher (QEMU in a pod) | `internal/vm` | One VM per goroutine inside the daemon. One process per VM is the next step |
| VirtualMachine / VMI | `SiliconVM` | One kind. `spec.running` starts and stops. `status.phase`, `ip`, `message` |
| DataVolume / CDI | `spec.disk.source` | URL or path. Downloaded, converted to raw, resized by the daemon |
| cloud-init | `spec.cloudInit.userData` | NoCloud ISO with volume label `cidata` |

There is one binary and no separate operator. The CRD is applied with `kubectl`. An operator can be added when there is something for it to do.

## Lifecycle

```
create -> finalizer -> Provisioning -> Starting -> Running (status.ip)
running=false -> ACPI shutdown (20 s) then forced -> Stopped (disk kept, ip cleared)
running=true  -> same disk, boots again
delete -> stop VM -> remove ~/.kubesilicon/vms/<ns>_<name> -> drop finalizer
```

Guest hostname is `<name>-<first 8 chars of the UID>`. The VM IP comes from the macOS NAT DHCP leases (`/var/db/dhcpd_leases`), matched by hostname. A unique hostname is what keeps stale leases of other VMs out (see [validation](validation.md)).

## Constraints of the platform

1. `Virtualization.framework` reads raw disks, not qcow2. The daemon converts with `qemu-img`.
2. The binary must be signed with `com.apple.security.virtualization`. An ad-hoc `codesign -s -` is enough locally. Bridged networking would also need `com.apple.vm.networking`, which needs Apple's approval to distribute.
3. NAT gives the VM an address in `192.168.64.0/24`. With kind on Docker Desktop, pods reach it directly (tested). Other runtimes (Colima, OrbStack) are untested.
4. No graphical console without a window, so the console is serial only.
5. No live migration.
6. The daemon must use an explicit kubeconfig context (`-context` is required) so it never acts on whatever the current context happens to be.

## Roadmap

- **Phase 0, done:** prove boot, signing, cloud-init, NAT and pod-to-VM reachability. See [validation](validation.md).
- **Phase 1, done:** CRD, reconciler, full lifecycle, end-to-end test.
- **Phase 2:** one process per VM (daemon crash must not kill VMs), Service and DNS per VM, `kubesilicon` CLI (`console`, `ssh`, `ip`), LaunchAgent, orphan cleanup.
- **Phase 3:** list what virtifoundry-style workloads use from KubeVirt and cover the gaps.
- **Later:** several Macs as nodes (Virtual Kubelet), nested virtualization, snapshots, macOS guests.

## Known limits

- One worker: provisioning a VM (a large download) delays the others.
- VMs live inside the daemon. If it crashes, they stop.
- Changing `cpu`, `memory` or `cloudInit` does not affect a VM that already exists. Stop and start does not re-read the CPU or memory.
- No `conditions` yet, only `phase` and `message`.
- Without `userData` the VM has no user you can log in with.
- The console is a log file with no input.
