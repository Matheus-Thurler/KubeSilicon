# Working on KubeSilicon

One Mac runs the VMs (Virtualization.framework) and the GPU jobs; the cluster only stores the
objects. Read [README.md](README.md) for the user's view, and [docs/design.md](docs/design.md) for
the mapping to KubeVirt, before changing the API.

This file is the shared contract for the agents that edit this repo (Claude Code, DSH); `AGENTS.md`
is a symlink to it. Keep it short and current: update "Recently changed" at the end of a task.

## Commands

| Command | |
|---|---|
| `make check` | gofmt, `go vet`, `go test -race`. Run it before every commit |
| `make bin/kubesilicond bin/kubesilicon` | build; the daemon is ad-hoc signed with `hack/vz.entitlements` |
| `hack/e2e.sh` | full VM cycle; needs a real Mac, a `kubesilicon` context and `qemu-img` |
| `hack/e2e-job.sh <ctx>` | SiliconJob cycle; needs a `kubesilicond` already running |
| `make install CTX=<ctx>` | copies the daemon to `~/.kubesilicon/bin` and loads a LaunchAgent |

Do not run `make install`, `hack/agent.sh install` or the e2e scripts unless the task asks for it:
they start VMs on the user's Mac, load a login item and create `~/.kubesilicon/e2e-key`.

## Commits and releases

- Conventional Commits, lowercase, imperative, no period: `fix(vm): a crashed VM stays stopped after
  a daemon restart`. Scopes in use: `vm`, `job`, `cli`, `e2e`, `docs`, `ci`, `kfp`.
- The subject is user-facing: release-please turns it into CHANGELOG.md. Keep it under ~72 chars.
- A body only when the why is not obvious, in two or three sentences. No list of what changed; the
  diff says that.
- No trailers: no `Co-Authored-By`, no `Signed-off-by`, no "generated with". Commit as the author.
- Never edit CHANGELOG.md, `.release-please-manifest.json` or tags by hand.
- Branches `fix/<slug>`, `feat/<slug>`, `docs/<slug>`. PRs are merged with a merge commit, so every
  commit has to stand on its own.

## Code

- Standard library and client-go first; a new dependency needs a reason in the PR.
- Comments explain the decision and the failure mode, in English, and are rarer than the code. Match
  the density of the file you are in.
- A deliberate shortcut gets a `ponytail:` comment naming the limit and the upgrade path.
- Tests live next to the thing they test, table-driven where it fits. The reconcilers are tested
  against `k8s.io/client-go/dynamic/fake` (see `cmd/kubesilicond/reconcile_test.go`).
- macOS only: `internal/vm` (vz), `internal/mem` (libproc) and `internal/lease`
  (`/var/db/dhcpd_leases`). A `GOOS=linux` build is expected to fail.

## State on disk

Everything the daemon keeps is under `~/.kubesilicon`, so a bug can be reproduced without the
cluster:

| Path | |
|---|---|
| `vms/<ns>_<name>/` | `pid`, `vmexit` (`shutdown`/`error`), `dead`, `restarts`, `mac`, `uid`, `disk.raw`, `seed.iso`, `console.log`, `launcher.log`, `console.sock` |
| `jobs/<ns>_<name>/` | `spec.json`, `pid`, `pgid`, `mem`, `exit`, `job.log`, `uid`; `venv/` and `work/` while it runs, `profile.sb` for the sandbox |
| `images/`, `cache/` | local disk sources, downloaded images |

## Two agents, one repo

- One branch per task, never commit to `main`.
- If another agent may be active, work in your own worktree (`/worktrees/<slug>`, ignored). Never
  switch branches or `stash` in a checkout you do not own.
- `git add` explicit paths, never `-A` or `.`. Run `git status` before staging.
- Do not touch files outside the task: README.md, docs/, examples/ and CHANGELOG.md unless the task
  is exactly that.
- Say what you verified and what you could not. A green `make check` is not an e2e run, and only the
  person at the Mac can run one.

## Recently changed

- 2026-11-27: the daemon takes an `flock` on `~/.kubesilicon/lock`, so a second one exits instead of
  spawning a launcher for the same VM; a VM whose launcher died goes through `exited()` after a
  daemon restart, so `restartPolicy` is not bypassed; a pass keeps the status it wrote, so
  `Provisioning` no longer overwrites `Failed`; the Service and EndpointSlice are republished on
  every pass instead of only when the IP moves; the KFP component says when the log tail stops at
  4 KiB; CI runs the tests with `-race`.
