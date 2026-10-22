# Contributing

Issues and pull requests are welcome. This is an early project, so open an issue before a large change.

## Setup

- Apple Silicon Mac, Go (see `go.mod`), `qemu-img` (`brew install qemu`).
- `make bin/kubesilicond` builds and ad-hoc signs the daemon. The `com.apple.security.virtualization` entitlement is required, and an unsigned binary fails at VM start.

## Before sending a PR

- `gofmt -l .` prints nothing, `go vet ./...` and `go test ./...` pass.
- Logic that is not trivial comes with a small test (see `internal/lease`).
- Changes to the reconcile loop, VM lifecycle or networking: run `hack/e2e.sh` and say so in the PR. CI cannot run it, because GitHub-hosted macOS runners do not provide nested virtualization.
- Keep the diff small. No new dependency for something the standard library or client-go already does.
- Mark deliberate shortcuts with a `ponytail:` comment that names the limit and the upgrade path.

## Commits

[Conventional Commits](https://www.conventionalcommits.org): `feat:`, `fix:`, `docs:`, `test:`, `chore:`.
