# Design

KubeSilicon gives a local Kubernetes cluster on a Mac the KubeVirt workflow (declare a VM, get a VM) without KVM. The VM runs on the macOS host through `Virtualization.framework`. The cluster only stores the desired state.

## Decisions

| Topic | Decision |
|---|---|
| Guests | Linux ARM64. macOS guests are out of scope for now |
| Cluster | A local cluster such as kind, one Mac. A fleet would need a `nodeName` field, not implemented |
| API | Own group `kubesilicon.io/v1`, shape similar to KubeVirt. Not a drop-in for `kubevirt.io` |
| Language | Go. [`Code-Hex/vz`](https://github.com/Code-Hex/vz) binds `Virtualization.framework` (cgo) |
| Out of scope for v1 | live migration, hotplug, GPU passthrough, snapshots, macOS guests |

## Mapping to KubeVirt

| KubeVirt | KubeSilicon | Note |
|---|---|---|
| virt-api (webhooks) | not needed | Validation is in the CRD schema |
| virt-controller (creates the launcher pod) | not needed | There is no pod: the VM runs on the host |
| virt-handler (reconciles on the node) | `kubesilicond` | One daemon on macOS, informer plus work queue, `client-go` dynamic client |
| virt-launcher (QEMU in a pod) | `kubesilicond launch` | One detached process per VM (own session), spawned by the daemon. It owns the VM through `internal/vm` |
| VirtualMachine / VMI | `SiliconVM` | One kind. `spec.running` starts and stops. `status.phase`, `ip`, `message` |
| DataVolume / CDI | `spec.disk.source` | URL (optional `sha256`) or a file under `~/.kubesilicon/images`. Downloaded, converted to raw, resized by the daemon. Images that reference another file (qcow2 backing or data file) are rejected |
| cloud-init | `spec.cloudInit.userData` | NoCloud ISO with volume label `cidata` |
| `virtctl console` / `ssh` | `kubesilicon console` / `ssh` / `ip` | Console talks to a unix socket the launcher serves in the VM directory. `ssh` and `ip` read `status.ip` through `kubectl` |
| Service for the VM | headless Service + EndpointSlice | Same name as the SiliconVM, owned by it. Gives `<name>.<namespace>.svc` |

There is one binary and no separate operator. The CRD is applied with `kubectl`. An operator can be added when there is something for it to do.

## Lifecycle

```
create -> finalizer -> Provisioning -> Starting -> Running (status.ip)
running=false -> ACPI shutdown (20 s) then forced -> Stopped (disk kept, ip cleared)
running=true  -> same disk, boots again
delete -> stop VM -> remove ~/.kubesilicon/vms/<ns>_<name> -> drop finalizer (Service and EndpointSlice are garbage-collected)
```

### Launcher processes

The daemon starts `kubesilicond launch ...` with `Setsid`, writes its pid to `<vm dir>/pid` and sends `SIGTERM` to stop it (the launcher asks the guest to shut down, waits up to 20 s, then forces). Stopping, restarting or killing the daemon leaves the launchers, and so the VMs, running. A new daemon reads the pid file and adopts a launcher only if the process is alive and its command line is `... launch ...` with the VM directory, which guards against pid reuse. If a launcher dies, the object goes to `Stopped` with a pointer to `launcher.log` and `console.log`.

### Orphans and the LaunchAgent

Each VM directory has a `uid` file with the UID of its object. On start, after the informer has synced, the daemon removes every directory whose object is gone or has another UID (deleted and recreated while the daemon was down), shutting its launcher down first. That can take up to ~25 s per VM, because the guest gets the chance to shut down cleanly.

`hack/agent.sh` installs a LaunchAgent (`RunAtLoad`, `KeepAlive`, `AbandonProcessGroup`) that runs a copy of the daemon from `~/.kubesilicon/bin`, so launchd does not need access to the checkout (macOS protects `~/Documents`). `AbandonProcessGroup` plus the launchers' own session keep VMs alive when the agent is restarted or removed.

### DNS

The daemon applies (server-side apply) a headless Service without selector and an EndpointSlice with the VM IP. The IP is added when the VM gets an address and removed when it stops. Both objects carry an ownerReference to the SiliconVM.

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
- **Phase 2, done:** one process per VM, DNS per VM, the `kubesilicon` client, the LaunchAgent and orphan cleanup.
- **Phase 3:** list what virtifoundry-style workloads use from KubeVirt and cover the gaps.
- **Later:** several Macs as nodes (Virtual Kubelet), nested virtualization, snapshots, macOS guests.

## Plan: native GPU jobs (`SiliconJob`)

Goal: Kubeflow steps and training jobs in the local kind cluster use the Mac GPU. A Linux guest under `Virtualization.framework` cannot reach Metal compute (no CUDA, no MPS), so the VM is the wrong place. The job runs as a native macOS process (PyTorch MPS, MLX) and Kubeflow only orchestrates it.

### Decisions

| Topic | Decision |
|---|---|
| Source | Kubeflow in the local kind cluster. The homelab cluster (over Tailscale) comes later |
| API | New kind `SiliconJob` in `kubesilicon.io/v1`, handled by the same `kubesilicond` |
| Execution | Native process in a per-job Python environment (venv/uv). Dependencies declared in the object. No container, because a Linux container loses the GPU |
| Isolation | Per-job work directory plus `sandbox-exec` limiting writes to it. Runs as the user. Dedicated macOS user is the fallback |
| Data | The daemon downloads inputs from the Kubeflow object store (MinIO) before the job and uploads outputs after. User code only sees local files and never gets credentials |

### Mapping to the current code

| Piece | Today | Change |
|---|---|---|
| CRD | `config/crd/kubesilicon.io_siliconvms.yaml` | Add `siliconjobs.yaml`; `make install` applies both |
| Informer and queue | One `gvr`, one queue, one worker in `main.go` | Second informer and queue for `siliconjobs`. Do not share the VM worker: a job runs for hours |
| Reconcile | `reconcile` for VMs | New `reconcileJob`: phases `Pending`, `Running`, `Succeeded`, `Failed` plus `message` |
| Process | `proc.go` `spawn`/`adopt`, subcommand `launch` | Parametrize the subcommand (`adopt` matches `" launch "`) and add `runjob`. Same survive-the-daemon model, state in `~/.kubesilicon/jobs/<ns>_<name>/` |
| Launcher | `launch.go` owns one VM | New `runjob.go`: sets up the work dir, runs the command, writes `exit` and `job.log` |
| Orphans | `cleanup.go` scans `vms/` | Same pass for `jobs/`, with the UID file check |
| Client | `kubesilicon ip/ssh/console` | Add `kubesilicon logs <job>` that reads `job.log`; `cancel` is `spec.running: false` |
| Python env | none | `internal/pyenv`: create venv with `uv`, shared wheel cache |
| Sandbox | none | `internal/sandbox`: builds the `sandbox-exec` profile behind a small interface |
| Artifacts | none | `internal/artifact`: MinIO download and upload using the cluster Secret |

### Status

Phases 1 to 3 are implemented and tested on an M5 Pro Mac: `hack/e2e-job.sh` (sandbox, timeout, cancel, queue, parallel jobs and memory limit, artifacts, path escape), a daemon-restart check, and a Kubeflow Pipelines 2.18.0 run (`examples/kubeflow/pipeline.py`) whose step created a `SiliconJob` that trained a small model on `mps`. Metal/MPS works from a detached process and inside the `sandbox-exec` profile.

How it fits together: the KFP step is a pod in kind; it creates a `SiliconJob` through the Kubernetes API (RBAC in `examples/kubeflow/rbac.yaml`) and polls `status`. `kubesilicond` runs the script on macOS and mirrors the phase and the last 4 KiB of the log into `status.logTail`, so the step log shows the Mac output. `hack/kind-kubeflow.sh` builds the whole cluster (k8s 1.34, cert-manager, KFP, CRDs).

Known gaps: `kfp-metadata-writer` has no arm64 image, so it is scaled to 0 (no lineage metadata in the UI, pipelines run); artifacts use presigned URLs, not the SeaweedFS Secret (KFP 2.18 ships SeaweedFS, not MinIO); the per-job memory limit is a watchdog that polls, not a kernel limit (macOS has no cgroups); `status.logTail` is visible to anyone who can read the object; the pipeline step cannot return files from the job yet.

### Phases

1. **Minimal `SiliconJob`.** Runs a command, writes `status` and `job.log`. Proves PyTorch MPS works end to end. Validate here that the Metal device is reachable from a detached process.
2. **Environment and isolation.** `uv` venv per job, wheel cache, `sandbox-exec` profile, timeout (default set) and cancel. Validate that the profile still allows Metal.
3. **Artifacts and Kubeflow.** MinIO in and out, a Kubeflow pipeline component that creates the `SiliconJob`, `make kind-kubeflow` with a `kindest/node` image the Kubeflow release supports.
4. **Hardening.** Done: `nice`, a queue in creation order, jobs in parallel when their `spec.memory` fits in the memory available, and a per-job limit (`internal/mem` reads the physical footprint, which includes Metal allocations; RSS does not). Not done: pause the queue, optional `runtime: torch-mps | mlx`, then a Virtual Kubelet node or the homelab cluster.

### Risks and mitigations

| Risk | Mitigation | Phase |
|---|---|---|
| `sandbox-exec` is deprecated | Keep it behind `internal/sandbox`; fall back to a dedicated macOS user. Re-test on each macOS update | 2 |
| Ops without MPS fall back to CPU | `PYTORCH_ENABLE_MPS_FALLBACK=1` by default, log when it happens, offer MLX | 1-2 |
| Docker Desktop memory is small (3.8 GB seen) | `make` checks the Docker memory and warns below 12 GB | 1 |
| Kubeflow needs an older k8s than latest | `make kind-kubeflow` pins the node image; `-context` stays required | 3 |
| Job competes with the desktop | GPU queue of 1, low `nice`, memory limit, pause | 4 |
| Hung or orphan job | Mandatory default timeout, `cancel`, orphan cleanup at daemon start | 2-4 |
| MinIO credentials | Daemon reads the Secret; user code never sees it | 3 |
| Slow per-job environments | Shared `uv` wheel cache | 2 |
| Daemon dies with a job running | One process per job, adopted again at start (same as VMs) | 1-2 |
| Pipelines written for x86 or CUDA | Document that jobs use macOS arm64 wheels | 3 |

### Open points

- Metal inside a `sandbox-exec` profile: tested, works (see Status). Re-test on each macOS update.
- The sandbox allows `stat` on the home directory (needed to `exec` anything under it), so file names can be probed but not read or listed. Network stays open, so a job can still send what it can read (its own work dir) out.
- How the Kubeflow component reaches the daemon is only the CRD for now (it creates the object). Reading logs back into the pipeline UI is not designed.
- Several Macs and a fleet are out of scope, as for VMs.

## Known limits

- One state directory per cluster. `kubesilicond` treats every directory under `~/.kubesilicon/vms` and `jobs` that has no object in *its* cluster as an orphan and removes it. Two daemons on different clusters must use different `-root` values.

- One worker: provisioning a VM (a large download) delays the others.
- If the launcher crashes the VM is gone and the object says `Stopped`; there is no automatic restart. - The Service has no ports (headless, name resolution only).
- Changing `cpu`, `memory` or `cloudInit` does not affect a running VM. Stop and start provisions it again from the spec, so the change applies on the next start (the disk is kept).
- No `conditions` yet, only `phase` and `message`.
- Without `userData` the VM has no user you can log in with.
- The console is a text serial line: no window, no VNC. One client at a time; a new one replaces the old.
