# Running KubeSilicon for real

One Mac as a VM and GPU host for a Kubernetes cluster. It is a single machine: there is no failover, and the cluster must treat it as a pet. Tested on one Mac against kind; the steps for a real cluster are the same but have not been run on one.

## What survives a restart

| Event | Result |
|---|---|
| `kubesilicond` crashes or restarts | VMs and jobs keep running; the new daemon adopts them |
| API server restarts | informers reconnect on their own |
| Mac reboots during a job | the job is restarted, up to `spec.backoffLimit` times (default 2), then `Failed`. A script that exits non-zero is **not** retried |
| Mac reboots with VMs | VMs with `spec.running: true` boot again when the daemon is back, with the same MAC and IP. Tested with `kill -9` of daemon and launcher. The guest loses writes it had not flushed yet (about 30 s), like any hard power-off |
| Mac is off | jobs wait as `Pending`; the Kubeflow component gives up after `pending_timeout_seconds` (default 900) and deletes the job |
| Job already `Succeeded`/`Failed` in the cluster | never run again |

## 1. The Mac

The daemon is a LaunchAgent, so it only starts once a user is logged in.

```sh
sudo pmset -a sleep 0 disksleep 0 autorestart 1 powernap 0    # never sleep, power back on after an outage
```

System Settings > Users & Groups > "Automatically log in as". **This option does not exist while FileVault is on**; with FileVault on, someone must type the password after every reboot. Pick one. Running the daemon as a LaunchDaemon (no login needed) is untested: `Virtualization.framework` may need a user session.

## 2. Least-privilege credentials

`make install` normally uses your own kubeconfig context (usually cluster-admin). Use a dedicated identity instead:

```sh
hack/daemon-kubeconfig.sh <admin-context>                       # ServiceAccount + ClusterRole + token, writes ~/.kubesilicon/kubeconfig-<ctx>
make install CTX=kubesilicond KCFG=$HOME/.kubesilicon/kubeconfig-<ctx>
```

The role (`config/rbac/daemon.yaml`) covers the two CRDs, their `/status` and `/finalizers`, and `services` and `endpointslices` (VM DNS). No pods, secrets or nodes. The token does not expire; revoke it by deleting the `kubesilicond-token` Secret. The kubeconfig file is `0600`: protect it like a password.

## 3. Who may create jobs

A `SiliconJob` runs code on the Mac as the user of the daemon. Anyone who can create one in the cluster can run code there. Grant `create` on `siliconjobs` only to people you would let log in to that Mac. For Kubeflow, that is the `pipeline-runner` ServiceAccount (`examples/kubeflow/rbac.yaml`), which any pipeline author can use.

## 4. Who may create VMs

`spec.disk.source` is read on the Mac by the daemon. It accepts an http(s) URL or a file under `~/.kubesilicon/images` (symlinks are resolved), and only self-contained raw or qcow2 images: a qcow2 that names a backing file or an external data file is rejected, because converting it would copy that file of the Mac into the guest disk. Set `spec.disk.sha256` to pin a downloaded image. A VM still uses the Mac's CPU, memory, disk and network, so grant `create` on `siliconvms` with the same care as jobs.

## Limits

- Isolation is `sandbox-exec` (deprecated by Apple) plus a work directory. It stops a job reading your home directory or writing outside its directory. It does not stop network access, and it is not a boundary against hostile code. Not safe for untrusted tenants; a dedicated macOS user would be the next step.
- No per-job memory limit; the GPU and its unified memory are shared with the desktop. One GPU job runs at a time.
- One Mac per cluster state directory (`~/.kubesilicon`); two daemons on different clusters need different `-root` values.
- GPU for the VMs is not possible.

## Tested after a real reboot (kind on Docker Desktop, FileVault session logged in)

The LaunchAgent started the daemon at login and it kept retrying until Docker and the kind nodes were up (Docker was started by hand: autostart was off). A job that had already `Succeeded` was not rerun, and `hack/e2e-job.sh` passed afterwards. VMs were not running during this reboot, and a LaunchDaemon is still untested.
