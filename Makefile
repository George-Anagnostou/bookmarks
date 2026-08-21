GO ?= go
DIST_DIR ?= dist
SERVER_GOOS ?= linux
SERVER_GOARCH ?= amd64
CLI_GOOS ?= $(shell $(GO) env GOOS)
CLI_GOARCH ?= $(shell $(GO) env GOARCH)
CLI_INSTALL_DIR ?= $(if $(GOBIN),$(GOBIN),$(shell bin="$$($(GO) env GOBIN)"; if [ -n "$$bin" ]; then printf '%s' "$$bin"; else printf '%s/bin' "$$($(GO) env GOPATH | cut -d: -f1)"; fi))
SERVER_BIN := $(DIST_DIR)/bookmarkd-$(SERVER_GOOS)-$(SERVER_GOARCH)
CLI_BIN := $(DIST_DIR)/bookmarkctl-$(CLI_GOOS)-$(CLI_GOARCH)

-include .env.deploy

export BOOKMARKS_DEPLOY_HOST
export BOOKMARKS_DEPLOY_SERVICE
export BOOKMARKS_DEPLOY_TARGET
export BOOKMARKS_DEPLOY_USER
export BOOKMARKS_DOMAIN
export BOOKMARKS_URL

.PHONY: test test-race vet fmt-check script-check check build-server build-cli install-cli verify update rollback install-backups clean

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

vet:
	$(GO) vet ./...

fmt-check:
	test -z "$$(gofmt -l $$(git ls-files '*.go'))"

script-check:
	sh -n scripts/*.sh

check: fmt-check script-check test vet test-race

build-server:
	mkdir -p "$(DIST_DIR)"
	GOOS=$(SERVER_GOOS) GOARCH=$(SERVER_GOARCH) $(GO) build -trimpath -o "$(SERVER_BIN)" ./cmd/bookmarkd

build-cli:
	mkdir -p "$(DIST_DIR)"
	GOOS=$(CLI_GOOS) GOARCH=$(CLI_GOARCH) $(GO) build -trimpath -o "$(CLI_BIN)" ./cmd/bookmarkctl

install-cli: build-cli
	mkdir -p "$(CLI_INSTALL_DIR)"
	install -m 0755 "$(CLI_BIN)" "$(CLI_INSTALL_DIR)/bookmarkctl"

verify:
	test -n "$(BOOKMARKS_URL)"
	curl -fsS "$${BOOKMARKS_URL%/}/healthz" >/dev/null

update: check
	$(MAKE) build-server install-cli
	./scripts/deploy-bookmarkd.sh "$(SERVER_BIN)"

rollback:
	./scripts/rollback-bookmarkd.sh

install-backups:
	./scripts/remote-install-backups.sh

clean:
	rm -rf "$(DIST_DIR)"
