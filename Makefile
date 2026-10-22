ENT = hack/vz.entitlements

bin/kubesilicond: $(shell find cmd/kubesilicond internal -name '*.go')
	go build -o $@ ./cmd/kubesilicond
	codesign --force --sign - --entitlements $(ENT) $@

test:
	go test ./...
