.PHONY: install uninstall test check
ENT = hack/vz.entitlements

bin/kubesilicond: $(shell find cmd/kubesilicond internal -name '*.go')
	go build -o $@ ./cmd/kubesilicond
	codesign --force --sign - --entitlements $(ENT) $@ 2>/dev/null || codesign --force --sign - --entitlements $(ENT) $@

test:
	go test ./...

# make check: what CI runs, before every commit. Tracked and new Go files only, so the agent
# worktrees under .claude/ are not formatted.
GOFMT = gofmt -l $(shell git ls-files -co --exclude-standard '*.go')

check:
	@test -z "$$($(GOFMT))" || { $(GOFMT); exit 1; }
	go vet ./...
	go test -race ./...

bin/kubesilicon: $(shell find cmd/kubesilicon -name '*.go')
	go build -o $@ ./cmd/kubesilicon

# make install CTX=kind-kubesilicon [KCFG=kubeconfig]: build, install the CRD, run the daemon at login
install:
	@test -n "$(CTX)" || { echo "usage: make install CTX=<kube-context>"; exit 2; }
	@echo "1/3 building the daemon and the client"
	@$(MAKE) -s bin/kubesilicond bin/kubesilicon
	@echo "2/3 adding the SiliconVM and SiliconJob types to the cluster $(CTX)"
	@if [ -n "$(KCFG)" ]; then echo "(skipped: KCFG is set, hack/daemon-kubeconfig.sh applied them)"; else kubectl --context $(CTX) apply -f config/crd/; fi
	@echo "3/3 starting the daemon in the background"
	@hack/agent.sh install $(CTX) $(KCFG)

uninstall:
	@hack/agent.sh uninstall
