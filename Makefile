.PHONY: install uninstall test
ENT = hack/vz.entitlements

bin/kubesilicond: $(shell find cmd/kubesilicond internal -name '*.go')
	go build -o $@ ./cmd/kubesilicond
	codesign --force --sign - --entitlements $(ENT) $@ 2>/dev/null || codesign --force --sign - --entitlements $(ENT) $@

test:
	go test ./...

bin/kubesilicon: $(shell find cmd/kubesilicon -name '*.go')
	go build -o $@ ./cmd/kubesilicon

# make install CTX=kind-kubesilicon: build, install the CRD, run the daemon at login
install:
	@test -n "$(CTX)" || { echo "usage: make install CTX=<kube-context>"; exit 2; }
	@echo "1/3 building the daemon and the client"
	@$(MAKE) -s bin/kubesilicond bin/kubesilicon
	@echo "2/3 adding the SiliconVM type to the cluster $(CTX)"
	@kubectl --context $(CTX) apply -f config/crd/kubesilicon.io_siliconvms.yaml
	@echo "3/3 starting the daemon in the background"
	@hack/agent.sh install $(CTX)

uninstall:
	@hack/agent.sh uninstall
